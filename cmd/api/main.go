package main

import (
	"log"
	"net/http"

	"github.com/Geetur/Notery/internal/database"
	"github.com/gin-gonic/gin"
)

func main() {

	// setting up the database connection
	log.Println("initializing database...")
	database, err := database.Init()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Println("database initialized. Connection pool established.")

	// setting up the Gin router with middleware attached
	router := gin.Default()
	// remove the proxy warning (safe dev default)
	_ = router.SetTrustedProxies([]string{"127.0.0.1"})

	// simple health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "OK",
			"message": "Notery API is alive",
		})
	})

	_ = database // to avoid unused variable error for now

	// starting the API server
	log.Println("Server starting on port 8080...")
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	log.Println("Server stopped.")
}