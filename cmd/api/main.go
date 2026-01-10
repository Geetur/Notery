package main

import (
	"log"
	"net/http"

	"github.com/Geetur/Notery/internal/database"
	"github.com/Geetur/Notery/internal/handlers"
	"github.com/gin-gonic/gin"
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
	cartHandler := handlers.CreateCartHandler(redisClient)

	// health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "OK",
			"message": "Notery API is alive",
		})
	})

	// note endpoints
	router.GET("/notes/:id", noteHandler.GetNoteByID)
	router.POST("/notes", noteHandler.CreateNote)
	router.GET("/notes/pending", noteHandler.GetPendingNotes)
	router.GET("/notes/approved", noteHandler.GetApprovedNotes)
	router.PATCH("/notes/:id/approve", noteHandler.ApproveNote)
	router.PATCH("/notes/:id/reject", noteHandler.RejectNote)
	router.DELETE("/notes/:id", noteHandler.DeleteNote)

	// cart endpoints
	router.GET("/cart/:user_id", cartHandler.GetCart)
	router.POST("/cart", cartHandler.AddToCart)
	router.DELETE("/cart/:user_id/:item_id", cartHandler.RemoveFromCart)

	// starting the API server
	log.Println("Server starting on port 8080...")
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	log.Println("Server stopped.")
}
