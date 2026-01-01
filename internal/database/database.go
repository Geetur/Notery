package database

import (
	"fmt"
	"log"
	"os"

	"github.com/Geetur/Notery/internal/models"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

)
// database is a global variable that stores our connection pool
var database *gorm.DB

func Init() {
	connect()
	migrate()
}

func connect() {

	// get our data source name (DSN), with credentials matching that within
	// the docker compose file. Load environment variables from .env file if it exists
	_ = godotenv.Load()

	// format the DSN string, fetch local environment variables or use defaults
	DSN := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
						getenv("DB_HOST", "localhost"),
						getenv("DB_USER", "admin"),
						getenv("DB_PASSWORD", ""),
						getenv("DB_NAME", "notery_db"),
						getenv("DB_PORT", "5432"),
						getenv("DB_SSLMODE", "disable"),
						getenv("DB_TIMEZONE", "UTC"),
					)		
	var err error
	database, err = gorm.Open(postgres.Open(DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	log.Println("Connected to Postgres.")

}

func migrate() {
	// auto-migrate our models
	err := database.AutoMigrate(&models.Note{})
	if err != nil {
		log.Fatalf("Failed to auto-migrate database: %v", err)
	}
	log.Println("Database migrated successfully.")
}

func getenv(key string, def string) string {
	// get environment variables, or use default
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
