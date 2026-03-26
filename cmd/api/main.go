// main.go — Application entrypoint: dependency init, route wiring, graceful shutdown.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/Geetur/Notery/internal/config"
	"github.com/Geetur/Notery/internal/database"
	"github.com/Geetur/Notery/internal/email"
	"github.com/Geetur/Notery/internal/handlers"
	"github.com/Geetur/Notery/internal/middleware"
	"github.com/Geetur/Notery/internal/payment"
)

func main() {
	// ── Configuration ──────────────────────────────────────────────────────
	log.Println("loading configuration...")
	cfg := config.Load()
	middleware.LoadRateLimitOverrides()

	// ── Dependencies ───────────────────────────────────────────────────────
	db, err := database.InitDatabase()
	if err != nil {
		log.Fatalf("database init failed: %v", err)
	}

	redisClient, err := database.InitRedis()
	if err != nil {
		log.Fatalf("redis init failed: %v", err)
	}

	meiliClient, meiliIndex, err := database.InitMeilisearch()
	if err != nil {
		log.Fatalf("meilisearch init failed: %v", err)
	}

	r2Client, err := database.InitR2()
	if err != nil {
		log.Printf("R2 not configured — file storage disabled (development mode): %v", err)
	}

	var paymentService payment.Service
	if cfg.StripeSecretKey != "" {
		paymentService = payment.NewStripeService(cfg.StripeSecretKey, cfg.StripeWebhookSecret)
		log.Println("stripe payment service active")
	} else {
		log.Println("stripe not configured — dev mode (auto-fulfil purchases)")
	}

	mailer := email.NewMailer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom)

	// ── Router & Global Middleware ─────────────────────────────────────────
	router := gin.Default()
	trustedProxies := []string{"127.0.0.1"}
	if tp := os.Getenv("TRUSTED_PROXIES"); tp != "" {
		trustedProxies = strings.Split(tp, ",")
	}
	_ = router.SetTrustedProxies(trustedProxies)
	router.Use(middleware.SecurityHeaders())
	router.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// ── App Handler (all dependencies injected) ────────────────────────────
	app := handlers.NewApp(handlers.AppConfig{
		DB:                 db,
		Redis:              redisClient,
		R2:                 r2Client,
		Meilisearch:        meiliClient,
		SearchIndex:        meiliIndex,
		JWTSecret:          cfg.JWTSecret,
		Payment:            paymentService,
		Mailer:             mailer,
		BaseURL:            cfg.BaseURL,
		FrontendURL:        cfg.FrontendURL,
		GoogleClientID:     cfg.GoogleClientID,
		GoogleClientSecret: cfg.GoogleClientSecret,
		GitHubClientID:     cfg.GitHubClientID,
		GitHubClientSecret: cfg.GitHubClientSecret,
	})

	// ── Health ─────────────────────────────────────────────────────────────
	router.GET("/health", func(c *gin.Context) {
		status := http.StatusOK
		deps := gin.H{}

		// Postgres
		if sqlDB, err := db.DB(); err != nil {
			deps["postgres"] = "error: " + err.Error()
			status = http.StatusServiceUnavailable
		} else if err := sqlDB.Ping(); err != nil {
			deps["postgres"] = "unreachable: " + err.Error()
			status = http.StatusServiceUnavailable
		} else {
			deps["postgres"] = "ok"
		}

		// Redis
		if redisClient != nil {
			if err := redisClient.Ping(c.Request.Context()).Err(); err != nil {
				deps["redis"] = "unreachable: " + err.Error()
				status = http.StatusServiceUnavailable
			} else {
				deps["redis"] = "ok"
			}
		} else {
			deps["redis"] = "not configured"
		}

		// Meilisearch
		if meiliClient != nil {
			if health, err := meiliClient.Health(); err != nil {
				deps["meilisearch"] = "unreachable: " + err.Error()
				// Meilisearch is optional — don't downgrade status
			} else if health.Status != "available" {
				deps["meilisearch"] = "unhealthy: " + health.Status
			} else {
				deps["meilisearch"] = "ok"
			}
		} else {
			deps["meilisearch"] = "not configured"
		}

		c.JSON(status, gin.H{"status": status == http.StatusOK, "dependencies": deps})
	})

	api := router.Group("/api/v1")

	// ── Auth (public, rate-limited) ────────────────────────────────────────
	auth := api.Group("/auth")
	if redisClient != nil {
		auth.Use(middleware.RateLimit(redisClient, middleware.DefaultAuthRateLimit, "auth:"))
	}
	auth.POST("/signup", app.Signup)
	auth.POST("/login", app.Login)
	auth.POST("/refresh", app.RefreshAccessToken)
	auth.POST("/logout", app.Logout)
	auth.POST("/forgot-password", app.ForgotPassword)
	auth.POST("/reset-password", app.ResetPassword)
	auth.GET("/verify-email", app.VerifyEmail)

	// OAuth routes (public, separate rate limit — each flow uses 2 requests)
	oauth := api.Group("/auth")
	if redisClient != nil {
		oauth.Use(middleware.RateLimit(redisClient, middleware.DefaultOAuthRateLimit, "oauth:"))
	}
	oauth.GET("/oauth/providers", app.OAuthProviders)
	oauth.GET("/oauth/google", app.OAuthGoogle)
	oauth.GET("/oauth/google/callback", app.OAuthGoogleCallback)
	oauth.GET("/oauth/github", app.OAuthGitHub)
	oauth.GET("/oauth/github/callback", app.OAuthGitHubCallback)

	// Auth endpoints requiring login (not verification)
	authProtected := auth.Group("", middleware.RequireAuth(cfg.JWTSecret))
	authProtected.POST("/logout-all", app.LogoutAll)
	authProtected.POST("/resend-verification", app.ResendVerification)

	// Stripe webhook (secured via Stripe signature, not JWT; rate-limited by IP)
	webhook := api.Group("")
	if redisClient != nil {
		webhook.Use(middleware.RateLimit(redisClient, middleware.RateLimitConfig{
			MaxRequests: 100,
			Window:      1 * time.Minute,
		}, "webhook:"))
	}
	webhook.POST("/webhooks/stripe", app.HandleStripeWebhook)

	// ── Public Endpoints (optional auth, rate-limited reads) ──────────────
	optAuth := middleware.OptionalAuth(cfg.JWTSecret)
	readPublic := api.Group("", optAuth)
	if redisClient != nil {
		readPublic.Use(middleware.RateLimit(redisClient, middleware.DefaultReadRateLimit, "read:"))
	}
	readPublic.GET("/feed/hot", app.GetHotFeed)
	readPublic.GET("/notes/:id/comments", app.GetNoteComments)
	readPublic.GET("/comments/:comment_id", app.GetComment)
	readPublic.GET("/search", app.SearchAll)
	readPublic.GET("/users/:id/profile", app.GetUserProfile)
	readPublic.GET("/users/:id/avatar", app.GetAvatar)
	readPublic.GET("/users/:id/banner", app.GetUserBanner)
	readPublic.GET("/users/:id/notes", app.GetUserNotes)
	readPublic.GET("/users/:id/comments", app.GetUserComments)
	readPublic.GET("/notes/:id/thumbnail", app.GetThumbnail)

	// Approved notes browsing (public, supports all sort modes)
	readPublic.GET("/notes/approved", app.GetApprovedNotes)
	readPublic.GET("/notes/:id", app.GetNoteByID)

	// Subnotery browsing (public, rate-limited)
	readPublic.GET("/subnoteries", app.ListSubnoteries)
	readPublic.GET("/subnoteries/:subnotery_id", app.GetSubnoteryDetail)
	readPublic.GET("/subnoteries/:subnotery_id/notes", app.GetSubnoteryNotes)
	readPublic.GET("/subnoteries/:subnotery_id/banner", app.GetSubnoteryBanner)
	readPublic.GET("/subnoteries/:subnotery_id/members", app.GetSubnoteryMembers)

	// ── Authenticated Read-Only (login required, no verification) ──────────
	// Unverified users can browse, view profile, check purchases — read-only.
	readOnly := api.Group("", middleware.RequireAuth(cfg.JWTSecret))
	if redisClient != nil {
		readOnly.Use(middleware.RateLimit(redisClient, middleware.DefaultReadRateLimit, "read:"))
	}
	readOnly.GET("/notes/:id/content", app.GetNotePDFContent)
	readOnly.GET("/notes/:id/preview", app.GetNotePreview)
	readOnly.GET("/cart", app.GetCart)
	readOnly.GET("/notes/:id/purchased", app.CheckPurchaseStatus)
	readOnly.GET("/me/purchases", app.GetMyPurchases)
	readOnly.GET("/me/purchases/history", app.GetPurchaseHistory)
	readOnly.GET("/me/profile", app.GetMyProfile)
	readOnly.GET("/orders/:order_id", app.GetOrderStatus)
	readOnly.GET("/me/notes", app.GetMyNotes)
	readOnly.GET("/me/comments", app.GetMyComments)
	readOnly.GET("/bookmarks", app.GetBookmarks)
	readOnly.GET("/bookmarks/:note_id", app.CheckBookmark)

	// Stripe Connect (payout) — read-only status check
	readOnly.GET("/me/stripe/status", app.StripeStatus)

	// Notifications (read-only)
	readOnly.GET("/notifications", app.GetNotifications)
	readOnly.GET("/notifications/unread-count", app.GetUnreadCount)

	// ── Verified Write Endpoints (login + email verification) ──────────────
	// All mutating operations require a verified email. Unverified → 403.
	write := api.Group("",
		middleware.RequireAuth(cfg.JWTSecret),
		middleware.RequireVerified(db),
		middleware.RateLimit(redisClient, middleware.DefaultWriteRateLimit, "write:"),
	)

	// Notes
	write.POST("/notes", app.CreateNote)
	write.POST("/notes/:id/content", app.UploadNotePDF)
	write.POST("/notes/:id/thumbnail", app.UploadThumbnail)
	write.DELETE("/notes/:id/thumbnail", app.DeleteThumbnail)
	write.POST("/notes/:id/upvote", app.Upvote)
	write.POST("/notes/:id/downvote", app.Downvote)

	// Cart & Checkout
	write.POST("/cart", app.AddToCart)
	write.DELETE("/cart/:item_id", app.RemoveFromCart)
	write.POST("/checkout", app.CheckoutCart)
	write.POST("/checkout/selected", app.CheckoutSelected)
	write.POST("/notes/:id/purchase", app.PurchaseSingleNote)

	// Profile & Avatar
	write.PATCH("/me/profile", app.UpdateMyProfile)
	write.POST("/me/avatar", app.UploadAvatar)
	write.DELETE("/me/avatar", app.DeleteAvatar)
	write.POST("/me/banner", app.UploadUserBanner)
	write.DELETE("/me/banner", app.DeleteUserBanner)

	// Comments
	write.POST("/notes/:id/comments", app.CreateComment)
	write.PUT("/comments/:comment_id", app.EditComment)
	write.DELETE("/comments/:comment_id", app.DeleteComment)
	write.POST("/comments/:comment_id/vote", app.VoteComment)
	write.DELETE("/comments/:comment_id/vote", app.RemoveCommentVote)
	write.POST("/comments/:comment_id/pin", app.PinComment)
	write.DELETE("/comments/:comment_id/pin", app.UnpinComment)

	// Orders & Subnoteries
	write.POST("/orders/:order_id/confirm", app.ConfirmOrder)
	write.POST("/subnoteries/:subnotery_id/join", app.JoinSubnotery)
	write.POST("/subnoteries/:subnotery_id/leave", app.LeaveSubnotery)
	write.PATCH("/subnoteries/:subnotery_id/settings", app.UpdateSubnoterySettings)
	write.POST("/subnoteries/:subnotery_id/banner", app.UploadSubnoteryBanner)
	write.DELETE("/subnoteries/:subnotery_id/banner", app.DeleteSubnoteryBanner)
	write.DELETE("/subnoteries/:subnotery_id/admins/:uid", app.RemoveAdminFromSubnotery)

	// Bookmarks
	write.POST("/bookmarks/:note_id", app.AddBookmark)
	write.DELETE("/bookmarks/:note_id", app.RemoveBookmark)

	// Bug reports
	write.POST("/reports/bug", app.SubmitBugReport)

	// Notifications (actions)
	write.PATCH("/notifications/:id/read", app.MarkNotificationRead)
	write.POST("/notifications/read-all", app.MarkAllNotificationsRead)
	write.POST("/notifications/:id/accept", app.AcceptNotification)
	write.POST("/notifications/:id/deny", app.DenyNotification)

	// Subnotery admin invites
	write.POST("/subnoteries/:subnotery_id/invite-admin", app.InviteAdmin)

	// Subnotery member management (admin action)
	write.DELETE("/subnoteries/:subnotery_id/members/:uid", app.RemoveMemberFromSubnotery)

	// Stripe Connect (payout)
	write.POST("/me/stripe/connect", app.StripeConnect)
	write.POST("/me/stripe/refresh-link", app.StripeRefreshLink)

	// Bans (subnotery-scoped)
	write.POST("/subnoteries/:subnotery_id/bans", app.BanUser)
	write.DELETE("/subnoteries/:subnotery_id/bans/:uid", app.UnbanUser)
	write.GET("/subnoteries/:subnotery_id/bans", app.ListBans)

	// ── Admin Endpoints (verified + admin role) ────────────────────────────
	admin := write.Group("", middleware.RequireAdmin(db))
	admin.GET("/notes/pending", app.GetPendingNotes)
	admin.PATCH("/notes/:id/approve", app.ApproveNote)
	admin.PATCH("/notes/:id/reject", app.RejectNote)
	admin.DELETE("/notes/:id", app.DeleteNote)
	admin.PATCH("/notes/:id/lock", app.LockNote)
	admin.PATCH("/notes/:id/unlock", app.UnlockNote)
	admin.GET("/admin/notes/:id/preview", app.AdminPreviewPDF)
	admin.DELETE("/admin/notes/:id/content", app.DeleteNotePDF)
	admin.POST("/subnoteries/:subnotery_id/admins", app.AddAdminToSubnotery)

	// Site-wide bans (global admin only)
	admin.POST("/admin/bans", app.SiteWideBan)
	admin.DELETE("/admin/bans/:uid", app.RemoveSiteWideBan)

	// ── Server Start & Graceful Shutdown ───────────────────────────────────
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("server starting on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}

	// Close database connection pool.
	if sqlDB, err := db.DB(); err == nil {
		if err := sqlDB.Close(); err != nil {
			log.Printf("error closing database: %v", err)
		} else {
			log.Println("database connection closed.")
		}
	}

	// Close Redis connection.
	if redisClient != nil {
		if err := redisClient.Close(); err != nil {
			log.Printf("error closing redis: %v", err)
		} else {
			log.Println("redis connection closed.")
		}
	}

	log.Println("server stopped.")
}
