package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Geetur/Notery/internal/database"
	"github.com/Geetur/Notery/internal/handlers"
	"github.com/Geetur/Notery/internal/middleware"
)

func main() {
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

	// setting up the Gin router with middleware attached
	router := gin.Default()
	_ = router.SetTrustedProxies([]string{"127.0.0.1"})

	// ----- Handler Initialization -----
	// initializing the note handler with the database connection
	noteHandler := handlers.CreateNoteHandler(db, meiliClient, meiliIndex)

	// cart handler for shopping cart operations (backed by Redis)
	cartHandler := handlers.CreateCartHandler(redisClient, db)

	// auth handler for login/signup
	authHandler := handlers.CreateAuthHandler(db)

	// subnotery handler for community management
	subnoteryHandler := handlers.CreateSubnoteryHandler(db)

	// feed handler for hot/trending notes (backed by Redis sorted sets)
	feedHandler := handlers.CreateFeedHandler(redisClient, db)

	// content handler for PDF operations (backed by R2)
	// This handles PDF uploads, viewing, and access control
	contentHandler := handlers.CreateContentHandler(db, r2Client)

	// purchase handler for checkout and purchase history
	purchaseHandler := handlers.CreatePurchaseHandler(db, redisClient)

	// Wire up feed handler to note handler for hot feed updates
	noteHandler.SetFeedHandler(feedHandler)
	// Wire up R2 client to note handler for PDF cleanup on delete/reject
	noteHandler.SetR2Client(r2Client)

	// health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "OK",
			"message": "Notery API is alive",
		})
	})

	api := router.Group("/api/v1")
	// auth endpoints (public)
	api.POST("/signup", authHandler.Signup)
	api.POST("/login", authHandler.Login)

	// feed endpoint (public with optional auth for personalization)
	api.GET("/feed/hot", middleware.OptionalAuth, feedHandler.GetHotFeed)

	// applying the RequireAuth middleware to all protected routes in this group
	protected := api.Group("")
	protected.Use(middleware.RequireAuth)
	{
		// ----- Note Endpoints -----
		protected.GET("/notes/:id", noteHandler.GetNoteByID)
		protected.POST("/notes", noteHandler.CreateNote)
		protected.GET("/notes/approved", noteHandler.GetApprovedNotes)

		// ----- PDF Content Endpoints -----
		// Upload PDF for a note (note creator uploads after creating note metadata)
		protected.POST("/notes/:id/content", contentHandler.UploadNotePDF)
		// View/stream PDF content (requires purchase or admin access)
		protected.GET("/notes/:id/content", contentHandler.GetNotePDFContent)

		// ----- Voting Endpoints -----
		protected.POST("/notes/:id/upvote", feedHandler.Upvote)
		protected.POST("/notes/:id/downvote", feedHandler.Downvote)

		// ----- Cart Endpoints -----
		protected.GET("/cart", cartHandler.GetCart)
		protected.POST("/cart", cartHandler.AddToCart)
		protected.DELETE("/cart/:item_id", cartHandler.RemoveFromCart)

		// ----- Purchase Endpoints -----
		// Checkout entire cart
		protected.POST("/checkout", purchaseHandler.CheckoutCart)
		// Direct purchase of single note (bypass cart)
		protected.POST("/notes/:id/purchase", purchaseHandler.PurchaseSingleNote)
		// Check if user purchased a specific note
		protected.GET("/notes/:id/purchased", purchaseHandler.CheckPurchaseStatus)

		// ----- User Account Endpoints -----
		// Get all purchased notes (for "My Purchases" page)
		protected.GET("/me/purchases", contentHandler.GetMyPurchases)
		// Get detailed purchase history with pagination
		protected.GET("/me/purchases/history", purchaseHandler.GetPurchaseHistory)

		// ----- Subnotery Endpoints -----
		protected.POST("/subnoteries/:subnotery_id/join", subnoteryHandler.JoinSubnotery)
	}

	// applying the RequireAdmin middleware to admin-only routes
	adminProtected := protected.Group("")
	adminProtected.Use(middleware.RequireAdmin(db))
	{
		// ----- Note Admin Endpoints -----
		// Get pending notes for review (scoped to admin's subnoteries)
		adminProtected.GET("/notes/pending", noteHandler.GetPendingNotes)
		// Approve a note (requires PDF to be uploaded)
		adminProtected.PATCH("/notes/:id/approve", noteHandler.ApproveNote)
		// Reject a note (deletes note and PDF)
		adminProtected.PATCH("/notes/:id/reject", noteHandler.RejectNote)
		// Delete a note (removes from search, feed, and R2)
		adminProtected.DELETE("/notes/:id", noteHandler.DeleteNote)

		// ----- PDF Admin Endpoints -----
		// Preview PDF during approval (same as user view but uses admin access)
		adminProtected.GET("/admin/notes/:id/preview", contentHandler.AdminPreviewPDF)
		// Delete PDF content only (without deleting note)
		adminProtected.DELETE("/admin/notes/:id/content", contentHandler.DeleteNotePDF)

		// ----- Subnotery Admin Endpoints -----
		adminProtected.POST("/subnoteries/:subnotery_id/admins", subnoteryHandler.AddAdminToSubnotery)
	}

	// starting the API server
	log.Println("Server starting on port 8080...")
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	log.Println("Server stopped.")
}
