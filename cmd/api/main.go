package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Geetur/Notery/internal/config"
	"github.com/Geetur/Notery/internal/database"
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

	// setting up the Gin router with middleware attached
	router := gin.Default()
	_ = router.SetTrustedProxies([]string{"127.0.0.1"})

	// Initialize the unified App handler with all dependencies
	app := handlers.NewApp(handlers.AppConfig{
		DB:          db,
		Redis:       redisClient,
		R2:          r2Client,
		Meilisearch: meiliClient,
		SearchIndex: meiliIndex,
		JWTSecret:   cfg.JWTSecret,
		Payment:     paymentService,
	})

	// health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "OK",
			"message": "Notery API is alive",
		})
	})

	api := router.Group("/api/v1")
	// auth endpoints (public)
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
