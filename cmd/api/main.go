package main

import (
	"log"
	"net/http"

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

	// applying the RequireAuth middleware to all protected routes in this group
	protected := api.Group("")
	protected.Use(middleware.RequireAuth(cfg.JWTSecret))
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

	// starting the API server
	log.Println("Server starting on port 8080...")
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	log.Println("Server stopped.")
}
