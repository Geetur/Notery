package main

import (
	"log"
	"net/http"

	"github.com/Geetur/Notery/internal/database"
	"github.com/Geetur/Notery/internal/handlers"
	"github.com/gin-gonic/gin"
)

func main() {

	// setting up the database connection
	log.Println("initializing database...")
	db, err := database.InitDatabase()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Println("database initialized. Connection pool established.")

	// initializing Redis connection
	log.Println("initializing Redis...")
	database.InitRedis()
	if err := database.TestRedisConnection(); err != nil {
		log.Fatalf("Failed to initialize Redis: %v", err)
	}
	log.Println("Redis initialized.")

	// setting up the Gin router with middleware attached
	router := gin.Default()
	_ = router.SetTrustedProxies([]string{"127.0.0.1"})

	// initializing the note handler with the database connection
	noteHandler := handlers.CreateNoteHandler(db)
	// this needs to be changed so it dosent rely on database package directly
	cartHandler := handlers.CreateCartHandler(database.RedisClient)

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