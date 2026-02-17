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
	// ----- Load configuration once at startup -----
	log.Println("loading configuration...")
	cfg := config.Load()
	log.Println("configuration loaded.")

	// ----- setting up the database connection ----------------------------------------------
	log.Println("initializing database...")
	db, err := database.InitDatabase()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Println("database initialized. Connection pool established.")
	// ----- setting up the database connection ----------------------------------------------

	// ------ initializing Redis connection ------------------------------------------------
	log.Println("initializing Redis...")
	redisClient, err := database.InitRedis()
	if err != nil {
		log.Fatalf("Failed to initialize Redis: %v", err)
	}
	log.Println("Redis initialized.")
	// ------ initializing Redis connection ------------------------------------------------

	// ------ initializing Meilisearch connection ----------------------------------------------
	log.Println("initializing Meilisearch...")
	meiliClient, meiliIndex, err := database.InitMeilisearch()
	if err != nil {
		log.Fatalf("Failed to initialize Meilisearch: %v", err)
	}
	log.Println("Meilisearch initialized.")
	// ------ initializing Meilisearch connection ----------------------------------------------

	// ------ initializing Cloudflare R2 connection ------------------------------------------
	// R2 stores PDF content for notes. PDFs are served through proxy endpoints,
	// never directly exposed to users. This prevents unauthorized downloads.
	log.Println("initializing Cloudflare R2...")
	r2Client, err := database.InitR2()
	if err != nil {
		log.Fatalf("Failed to initialize Cloudflare R2: %v", err)
	}
	log.Println("Cloudflare R2 initialized.")
	// ------ initializing Cloudflare R2 connection ------------------------------------------

	// ------ initializing payment service ---------------------------------------------------
	var paymentService payment.Service
	if cfg.StripeSecretKey != "" {
		log.Println("initializing Stripe payment service...")
		paymentService = payment.NewStripeService(cfg.StripeSecretKey, cfg.StripeWebhookSecret)
		log.Println("Stripe payment service initialized.")
	} else {
		log.Println("Stripe not configured — purchases will auto-fulfil (development mode).")
	}
	// ------ initializing payment service ---------------------------------------------------

	// ------ initializing email service -----------------------------------------------------
	mailer := email.NewMailer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom)
	// ------ initializing email service -----------------------------------------------------

	// setting up the Gin router with middleware attached
	router := gin.Default()
	_ = router.SetTrustedProxies([]string{"127.0.0.1"})

	// ----- Global middleware -----

	// Security headers (nosniff, DENY frame, strict referrer, etc.)
	router.Use(middleware.SecurityHeaders())

	// CORS — origins loaded from CORS_ORIGINS env var (default: localhost dev ports).
	router.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Initialize the unified App handler with all dependencies
	app := handlers.NewApp(handlers.AppConfig{
		DB:          db,
		Redis:       redisClient,
		R2:          r2Client,
		Meilisearch: meiliClient,
		SearchIndex: meiliIndex,
		JWTSecret:   cfg.JWTSecret,
		Payment:     paymentService,
		Mailer:      mailer,
		BaseURL:     cfg.BaseURL,
	})

	// health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "OK",
			"message": "Notery API is alive",
		})
	})

	api := router.Group("/api/v1")

	// ----- Auth Endpoints (public, rate-limited) -----
	auth := api.Group("/auth")
	if redisClient != nil {
		auth.Use(middleware.RateLimit(redisClient, middleware.DefaultAuthRateLimit, "auth:"))
	}
	{
		auth.POST("/signup", app.Signup)
		auth.POST("/login", app.Login)
		auth.POST("/refresh", app.RefreshAccessToken)
		auth.POST("/logout", app.Logout)
		auth.POST("/forgot-password", app.ForgotPassword)
		auth.POST("/reset-password", app.ResetPassword)
		auth.GET("/verify-email", app.VerifyEmail)
	}

	// Auth endpoints that require authentication
	authProtected := auth.Group("")
	authProtected.Use(middleware.RequireAuth(cfg.JWTSecret))
	{
		authProtected.POST("/logout-all", app.LogoutAll)
		authProtected.POST("/resend-verification", app.ResendVerification)
		authProtected.POST("/change-password", app.ChangePassword)
	}

	// Legacy auth routes (backward compatibility)
	api.POST("/signup", app.Signup)
	api.POST("/login", app.Login)

	// Stripe webhook (public — secured via Stripe signature verification, not JWT)
	api.POST("/webhooks/stripe", app.HandleStripeWebhook)

	// feed endpoint (public with optional auth for personalization)
	api.GET("/feed/hot", middleware.OptionalAuth(cfg.JWTSecret), app.GetHotFeed)

	// ----- Comment Read Endpoints (public, optional auth for user_vote field) -----
	// FIX #6: Comment listing is publicly readable. Auth is optional so logged-in
	// users get their vote state attached to each comment.
	api.GET("/notes/:id/comments",
		middleware.OptionalAuth(cfg.JWTSecret),
		app.GetNoteComments)
	api.GET("/comments/:comment_id",
		middleware.OptionalAuth(cfg.JWTSecret),
		app.GetComment)

	// ----- Public User Profile Read -----
	api.GET("/users/:id/profile", app.GetUserProfile)

	// ----- Public Avatar Proxy (24h cache) -----
	api.GET("/users/:id/avatar", app.GetAvatar)

	// ----- Search (public, optional auth for personalization) -----
	api.GET("/search", middleware.OptionalAuth(cfg.JWTSecret), app.SearchAll)

	// ----- Auth-Only Endpoints (login required, email verification NOT required) -----
	// These endpoints allow unverified users to read data, view their profile,
	// and check statuses. They can use the app as a reader until verified.
	authOnly := api.Group("")
	authOnly.Use(middleware.RequireAuth(cfg.JWTSecret))
	{
		// Read-only note endpoints
		authOnly.GET("/notes/:id", app.GetNoteByID)
		authOnly.GET("/notes/approved", app.GetApprovedNotes)
		// View/stream PDF content (requires purchase or admin access)
		authOnly.GET("/notes/:id/content", app.GetNotePDFContent)

		// Read-only cart endpoint
		authOnly.GET("/cart", app.GetCart)

		// Read-only purchase endpoints
		authOnly.GET("/notes/:id/purchased", app.CheckPurchaseStatus)
		authOnly.GET("/me/purchases", app.GetMyPurchases)
		authOnly.GET("/me/purchases/history", app.GetPurchaseHistory)

		// Self-profile (read own profile, needed for verification banner)
		authOnly.GET("/me/profile", app.GetMyProfile)

		// Read-only order status
		authOnly.GET("/orders/:order_id", app.GetOrderStatus)

		// Own created notes (for "My Notes" tab on profile)
		authOnly.GET("/me/notes", app.GetMyNotes)
	}

	// ----- Verified Endpoints (login + email verification required) -----
	// All write/mutating operations require a verified email address.
	// Unverified users get 403 with code "EMAIL_NOT_VERIFIED".
	protected := api.Group("")
	protected.Use(middleware.RequireAuth(cfg.JWTSecret))
	protected.Use(middleware.RequireVerified(db))
	protected.Use(middleware.RateLimit(redisClient, middleware.DefaultWriteRateLimit, "write:"))
	{
		// note write endpoints
		protected.POST("/notes", app.CreateNote)

		// ----- PDF Content Endpoints -----
		protected.POST("/notes/:id/content", app.UploadNotePDF)

		// voting endpoints
		protected.POST("/notes/:id/upvote", app.Upvote)
		protected.POST("/notes/:id/downvote", app.Downvote)

		// cart write endpoints
		protected.POST("/cart", app.AddToCart)
		protected.DELETE("/cart/:item_id", app.RemoveFromCart)

		// ----- Purchase Endpoints -----
		protected.POST("/checkout", app.CheckoutCart)
		protected.POST("/notes/:id/purchase", app.PurchaseSingleNote)

		// ----- User Profile Write Endpoints -----
		protected.PATCH("/me/profile", app.UpdateMyProfile)

		// ----- Avatar Endpoints -----
		protected.POST("/me/avatar", app.UploadAvatar)
		protected.DELETE("/me/avatar", app.DeleteAvatar)

		// ----- Comment Write Endpoints -----
		protected.POST("/notes/:id/comments", app.CreateComment)
		protected.PUT("/comments/:comment_id", app.EditComment)
		protected.DELETE("/comments/:comment_id", app.DeleteComment)
		protected.POST("/comments/:comment_id/vote", app.VoteComment)
		protected.DELETE("/comments/:comment_id/vote", app.RemoveCommentVote)

		// ----- Order Endpoints -----
		protected.POST("/orders/:order_id/confirm", app.ConfirmOrder)

		// subnotery endpoints
		protected.POST("/subnoteries/:subnotery_id/join", app.JoinSubnotery)
	}

	// applying the RequireAdmin middleware to admin-only routes
	adminProtected := protected.Group("")
	adminProtected.Use(middleware.RequireAdmin(db))
	{
		// note admin endpoints
		adminProtected.GET("/notes/pending", app.GetPendingNotes)
		adminProtected.PATCH("/notes/:id/approve", app.ApproveNote)
		adminProtected.PATCH("/notes/:id/reject", app.RejectNote)
		adminProtected.DELETE("/notes/:id", app.DeleteNote)

		// ----- PDF Admin Endpoints -----
		// Preview PDF during approval (same as user view but uses admin access)
		adminProtected.GET("/admin/notes/:id/preview", app.AdminPreviewPDF)
		// Delete PDF content only (without deleting note)
		adminProtected.DELETE("/admin/notes/:id/content", app.DeleteNotePDF)

		// subnotery admin endpoints
		adminProtected.POST("/subnoteries/:subnotery_id/admins", app.AddAdminToSubnotery)
	}

	// H6/H7: HTTP server with timeouts and graceful shutdown
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine so graceful shutdown can proceed
	go func() {
		log.Println("Server starting on port 8080...")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to trigger graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Give outstanding requests up to 10 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped.")
}
