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

func InitDatabase() (*gorm.DB, error) {
	// logging what is occuring, but not forcing faliure
	// to maintain a resistant service
	log.Println("Attempting to connect to database...")
	db, err := connect()
	if err != nil {
		log.Printf("Failed to connect to database: %v", err)
		return nil, err
	}
	log.Println("Database connection established.")

	log.Println("Attempting to migrate database schema...")
	if err := migrate(db); err != nil {
		log.Printf("Database migration failed: %v", err)
		return nil, err
	}
	log.Println("Database migration completed.")
	return db, nil
}

// create returns the database connection pool

func connect() (*gorm.DB, error) {

	// get our data source name (DSN), with credentials matching that within
	// the docker compose file. Load environment variables from .env file if it exists

	// disallowing silent failure here. ok in prod; helpful in dev

	if err := godotenv.Load(); err != nil {
    	log.Println("No .env file found (ok):", err)
	}

	// format the DSN string, fetch local environment variables or use defaults
	// make sure to replace ssl mode to required in production
	DSN := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
						getenv("DB_HOST", "localhost"),
						getenv("DB_USER", "admin"),
						getenv("DB_PASSWORD", ""),
						getenv("DB_NAME", "notery_db"),
						getenv("DB_PORT", "5432"),
						getenv("DB_SSLMODE", "disable"),
						getenv("DB_TIMEZONE", "UTC"),
					)		
	
	db, err := gorm.Open(postgres.Open(DSN), &gorm.Config{})
	return db, err
}

func migrate(db *gorm.DB) error {
	// auto-migrate our models
	err := db.AutoMigrate(&models.Note{})
	return err
}

func getenv(key string, def string) string {
	// get environment variables, or use default
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
