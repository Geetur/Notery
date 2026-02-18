// main.go — Application entrypoint: dependency init, route wiring, graceful shutdown.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	_ = router.SetTrustedProxies([]string{"127.0.0.1"})
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
		c.JSON(http.StatusOK, gin.H{"status": "OK", "message": "Notery API is alive"})
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

	// Stripe webhook (secured via Stripe signature, not JWT)
	api.POST("/webhooks/stripe", app.HandleStripeWebhook)

	// ── Public Endpoints (optional auth for personalization) ───────────────
	optAuth := middleware.OptionalAuth(cfg.JWTSecret)
	api.GET("/feed/hot", optAuth, app.GetHotFeed)
	api.GET("/notes/:id/comments", optAuth, app.GetNoteComments)
	api.GET("/comments/:comment_id", optAuth, app.GetComment)
	api.GET("/search", optAuth, app.SearchAll)
	api.GET("/users/:id/profile", app.GetUserProfile)
	api.GET("/users/:id/avatar", app.GetAvatar)

	// Subnotery browsing (public)
	api.GET("/subnoteries", app.ListSubnoteries)
	api.GET("/subnoteries/:subnotery_id", app.GetSubnoteryDetail)
	api.GET("/subnoteries/:subnotery_id/notes", app.GetSubnoteryNotes)

	// ── Authenticated Read-Only (login required, no verification) ──────────
	// Unverified users can browse, view profile, check purchases — read-only.
	readOnly := api.Group("", middleware.RequireAuth(cfg.JWTSecret))
	readOnly.GET("/notes/:id", app.GetNoteByID)
	readOnly.GET("/notes/approved", app.GetApprovedNotes)
	readOnly.GET("/notes/:id/content", app.GetNotePDFContent)
	readOnly.GET("/cart", app.GetCart)
	readOnly.GET("/notes/:id/purchased", app.CheckPurchaseStatus)
	readOnly.GET("/me/purchases", app.GetMyPurchases)
	readOnly.GET("/me/purchases/history", app.GetPurchaseHistory)
	readOnly.GET("/me/profile", app.GetMyProfile)
	readOnly.GET("/orders/:order_id", app.GetOrderStatus)
	readOnly.GET("/me/notes", app.GetMyNotes)
	readOnly.GET("/bookmarks", app.GetBookmarks)
	readOnly.GET("/bookmarks/:note_id", app.CheckBookmark)

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
	write.POST("/notes/:id/upvote", app.Upvote)
	write.POST("/notes/:id/downvote", app.Downvote)

	// Cart & Checkout
	write.POST("/cart", app.AddToCart)
	write.DELETE("/cart/:item_id", app.RemoveFromCart)
	write.POST("/checkout", app.CheckoutCart)
	write.POST("/notes/:id/purchase", app.PurchaseSingleNote)

	// Profile & Avatar
	write.PATCH("/me/profile", app.UpdateMyProfile)
	write.POST("/me/avatar", app.UploadAvatar)
	write.DELETE("/me/avatar", app.DeleteAvatar)

	// Comments
	write.POST("/notes/:id/comments", app.CreateComment)
	write.PUT("/comments/:comment_id", app.EditComment)
	write.DELETE("/comments/:comment_id", app.DeleteComment)
	write.POST("/comments/:comment_id/vote", app.VoteComment)
	write.DELETE("/comments/:comment_id/vote", app.RemoveCommentVote)

	// Orders & Subnoteries
	write.POST("/orders/:order_id/confirm", app.ConfirmOrder)
	write.POST("/subnoteries/:subnotery_id/join", app.JoinSubnotery)

	// Bookmarks
	write.POST("/bookmarks/:note_id", app.AddBookmark)
	write.DELETE("/bookmarks/:note_id", app.RemoveBookmark)

	// ── Admin Endpoints (verified + admin role) ────────────────────────────
	admin := write.Group("", middleware.RequireAdmin(db))
	admin.GET("/notes/pending", app.GetPendingNotes)
	admin.PATCH("/notes/:id/approve", app.ApproveNote)
	admin.PATCH("/notes/:id/reject", app.RejectNote)
	admin.DELETE("/notes/:id", app.DeleteNote)
	admin.GET("/admin/notes/:id/preview", app.AdminPreviewPDF)
	admin.DELETE("/admin/notes/:id/content", app.DeleteNotePDF)
	admin.POST("/subnoteries/:subnotery_id/admins", app.AddAdminToSubnotery)

	// ── Server Start & Graceful Shutdown ───────────────────────────────────
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Println("server starting on :8080")
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
	log.Println("server stopped.")
}
