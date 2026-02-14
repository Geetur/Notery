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

	// CORS middleware — allow frontend origins during development and production.
	// Tighten AllowOrigins before production deploy.
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000", "http://localhost:5173"},
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
		Config:      cfg,
	})

	// health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "OK",
			"message": "Notery API is alive",
		})
	})

	api := router.Group("/api/v1")

	// ===== AUTH RATE LIMITING =====
	// Auth endpoints get strict rate limiting (5 req/min per IP) to prevent brute-force.
	authRateLimit := middleware.RateLimitConfig{MaxRequests: 5, Window: 1 * time.Minute}

	// Public read endpoints get moderate rate limiting (120 req/min per IP).
	publicReadRateLimit := middleware.RateLimitConfig{MaxRequests: 120, Window: 1 * time.Minute}

	// auth endpoints (public, rate-limited)
	authGroup := api.Group("")
	if redisClient != nil {
		authGroup.Use(middleware.RateLimit(redisClient, authRateLimit, "auth:"))
	}
	authGroup.POST("/signup", app.Signup)
	authGroup.POST("/login", app.Login)
	authGroup.POST("/auth/refresh", app.RefreshToken)
	authGroup.POST("/auth/logout", app.Logout)
	authGroup.GET("/auth/verify-email", app.VerifyEmail)

	// Stripe webhook (public — secured via Stripe signature verification, not JWT)
	api.POST("/webhooks/stripe", app.HandleStripeWebhook)

	// ===== PUBLIC READ ENDPOINTS (rate-limited) =====
	publicRead := api.Group("")
	if redisClient != nil {
		publicRead.Use(middleware.RateLimit(redisClient, publicReadRateLimit, "pub:"))
	}

	// feed endpoint (public with optional auth for personalization)
	publicRead.GET("/feed/hot", middleware.OptionalAuth(cfg.JWTSecret), app.GetHotFeed)

	// Comment Read Endpoints (public, optional auth for user_vote field)
	publicRead.GET("/notes/:id/comments",
		middleware.OptionalAuth(cfg.JWTSecret),
		app.GetNoteComments)
	publicRead.GET("/comments/:comment_id",
		middleware.OptionalAuth(cfg.JWTSecret),
		app.GetComment)

	// Public User Profile Read
	publicRead.GET("/users/:id/profile", app.GetUserProfile)
	// Public avatar proxy
	publicRead.GET("/avatars/:user_id", app.GetAvatar)

	// applying the RequireAuth middleware to all protected routes in this group
	protected := api.Group("")
	protected.Use(middleware.RequireAuth(cfg.JWTSecret))

	// Apply per-user write rate limiting to all protected (mutating) routes.
	protected.Use(middleware.RateLimit(redisClient, middleware.DefaultWriteRateLimit, "write:"))
	{
		// note endpoints
		protected.GET("/notes/:id", app.GetNoteByID)
		protected.POST("/notes", app.CreateNote)
		protected.GET("/notes/approved", app.GetApprovedNotes)

		// ----- PDF Content Endpoints -----
		// Upload PDF for a note (note creator uploads after creating note metadata)
		protected.POST("/notes/:id/content", app.UploadNotePDF)
		// View/stream PDF content (requires purchase or admin access)
		protected.GET("/notes/:id/content", app.GetNotePDFContent)

		// voting endpoints
		protected.POST("/notes/:id/upvote", app.Upvote)
		protected.POST("/notes/:id/downvote", app.Downvote)

		// cart endpoints
		protected.GET("/cart", app.GetCart)
		protected.POST("/cart", app.AddToCart)
		protected.DELETE("/cart/:item_id", app.RemoveFromCart)

		// ----- Purchase Endpoints -----
		// Checkout entire cart
		protected.POST("/checkout", app.CheckoutCart)
		// Direct purchase of single note (bypass cart)
		protected.POST("/notes/:id/purchase", app.PurchaseSingleNote)
		// Check if user purchased a specific note
		protected.GET("/notes/:id/purchased", app.CheckPurchaseStatus)

		// ----- User Account Endpoints -----
		// Get all purchased notes (for "My Purchases" page)
		protected.GET("/me/purchases", app.GetMyPurchases)
		// Get detailed purchase history with pagination
		protected.GET("/me/purchases/history", app.GetPurchaseHistory)

		// ----- User Profile Endpoints -----
		// Get own full profile (authenticated self)
		protected.GET("/me/profile", app.GetMyProfile)
		// Update own profile (partial updates)
		protected.PATCH("/me/profile", app.UpdateMyProfile)
		// Upload avatar (multipart/form-data, max 5MB, JPEG/PNG/WebP/GIF)
		protected.POST("/me/avatar", app.UploadAvatar)
		// Delete avatar
		protected.DELETE("/me/avatar", app.DeleteAvatar)

		// ----- Session Management Endpoints -----
		// Revoke all refresh tokens (logout everywhere)
		protected.POST("/auth/logout-all", app.LogoutAll)
		// Resend email verification
		protected.POST("/auth/resend-verification", app.ResendVerification)

		// ----- Comment Write Endpoints -----
		// Create a new comment or reply on a note
		protected.POST("/notes/:id/comments", app.CreateComment)
		// Edit own comment
		protected.PUT("/comments/:comment_id", app.EditComment)
		// Soft-delete own comment (also allows subnotery admin delete below)
		protected.DELETE("/comments/:comment_id", app.DeleteComment)
		// Vote on a comment (+1 upvote, -1 downvote, toggle)
		protected.POST("/comments/:comment_id/vote", app.VoteComment)
		// Remove vote from a comment
		protected.DELETE("/comments/:comment_id/vote", app.RemoveCommentVote)
		// ----- Order Endpoints -----
		// Check order status (frontend polls after payment)
		protected.GET("/orders/:order_id", app.GetOrderStatus)
		// Manually confirm/reconcile order (checks Stripe if webhook was delayed)
		protected.POST("/orders/:order_id/confirm", app.ConfirmOrder)

		//subnotery endpoints
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
