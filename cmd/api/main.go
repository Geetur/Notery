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

	// setting up the Gin router with middleware attached
	router := gin.Default()
	_ = router.SetTrustedProxies([]string{"127.0.0.1"})

	// initializing the note handler with the database connection
	noteHandler := handlers.CreateNoteHandler(db, meiliClient, meiliIndex)
	// this needs to be changed so it dosent rely on database package directly
	cartHandler := handlers.CreateCartHandler(redisClient, db)

	authHandler := handlers.CreateAuthHandler(db)

	subnoteryHandler := handlers.CreateSubnoteryHandler(db)

	feedHandler := handlers.CreateFeedHandler(redisClient, db)

	// Wire up feed handler to note handler for hot feed updates
	noteHandler.SetFeedHandler(feedHandler)

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
		// note endpoints
		protected.GET("/notes/:id", noteHandler.GetNoteByID)
		protected.POST("/notes", noteHandler.CreateNote)
		protected.GET("/notes/approved", noteHandler.GetApprovedNotes)

		// voting endpoints
		protected.POST("/notes/:id/upvote", feedHandler.Upvote)
		protected.POST("/notes/:id/downvote", feedHandler.Downvote)

		// cart endpoints
		protected.GET("/cart", cartHandler.GetCart)
		protected.POST("/cart", cartHandler.AddToCart)
		protected.DELETE("/cart/:item_id", cartHandler.RemoveFromCart)

		//subnotery endpoints
		protected.POST("/subnoteries/:subnotery_id/join", subnoteryHandler.JoinSubnotery)
	}

	// applying the RequireAdmin middleware to admin-only routes
	adminProtected := protected.Group("")
	adminProtected.Use(middleware.RequireAdmin(db))
	{
		// note admin endpoints
		adminProtected.GET("/notes/pending", noteHandler.GetPendingNotes)
		adminProtected.PATCH("/notes/:id/approve", noteHandler.ApproveNote)
		adminProtected.PATCH("/notes/:id/reject", noteHandler.RejectNote)
		adminProtected.DELETE("/notes/:id", noteHandler.DeleteNote)

		// subnotery admin endpoints
		adminProtected.POST("/subnoteries/:subnotery_id/admins", subnoteryHandler.AddAdminToSubnotery)
	}

	// starting the API server
	log.Println("Server starting on port 8080...")
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	log.Println("Server stopped.")
}
