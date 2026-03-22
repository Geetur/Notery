// Package database provides initialization and connection management for all
// external data stores: PostgreSQL (GORM), Redis, Meilisearch, and Cloudflare R2.
package database

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/config"
	"github.com/Geetur/Notery/internal/models"
)

// InitDatabase initializes the PostgreSQL connection and runs migrations.
// It returns the database connection pool or an error if initialization fails.
func InitDatabase() (*gorm.DB, error) {
	// logging what is occurring, but not forcing faliure
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
	// Build DSN from environment variables.
	// NOTE: godotenv.Load() is called once in config.Load() at startup.
	// No need to reload here.

	// Default sslmode to "require" in production, "disable" in development.
	defaultSSL := "disable"
	if config.IsProduction() {
		defaultSSL = "require"
	}

	DSN := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
		getenv("DB_HOST", "localhost"),
		getenv("DB_USER", "admin"),
		getenv("DB_PASSWORD", ""),
		getenv("DB_NAME", "notery_db"),
		getenv("DB_PORT", "5432"),
		getenv("DB_SSLMODE", defaultSSL),
		getenv("DB_TIMEZONE", "UTC"),
	)

	db, err := gorm.Open(postgres.Open(DSN), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Configure connection pool to prevent connection exhaustion at scale.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying *sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(getenvInt("DB_MAX_OPEN_CONNS", 25))
	sqlDB.SetMaxIdleConns(getenvInt("DB_MAX_IDLE_CONNS", 5))
	sqlDB.SetConnMaxLifetime(time.Duration(getenvInt("DB_CONN_MAX_LIFETIME_SEC", 300)) * time.Second)
	sqlDB.SetConnMaxIdleTime(time.Duration(getenvInt("DB_CONN_MAX_IDLE_SEC", 60)) * time.Second)

	return db, nil
}

// migrate applies database schema migrations using GORM AutoMigrate.
// AutoMigrate creates tables, missing columns, and indexes.
// It does NOT delete unused columns to protect data.
func migrate(db *gorm.DB) error {
	// Setup explicit join table model for user_admins so that GORM auto-populates
	// CreatedAt on Association Append/Replace, enabling admin seniority checks.
	if err := db.SetupJoinTable(&models.Subnotery{}, "Admins", &models.UserAdmin{}); err != nil {
		return fmt.Errorf("setup join table (Subnotery.Admins): %w", err)
	}
	if err := db.SetupJoinTable(&models.User{}, "AdminOf", &models.UserAdmin{}); err != nil {
		return fmt.Errorf("setup join table (User.AdminOf): %w", err)
	}

	// First, migrate all models
	if err := db.AutoMigrate(&models.Subnotery{}, &models.Note{}, &models.User{}, &models.Purchase{}, &models.Vote{}, &models.Order{}, &models.OrderItem{}, &models.Comment{}, &models.CommentVote{}, &models.RefreshToken{}, &models.EmailVerification{}, &models.PasswordReset{}, &models.Bookmark{}, &models.KarmaLedger{}, &models.Notification{}, &models.Ban{}, &models.PayoutRecord{}); err != nil {
		return err
	}

	// Backfill NULL created_at values in user_admins with current timestamp.
	// Existing admin rows pre-date the seniority tracking column, so they
	// all get the same timestamp (treated as equal seniority).
	db.Exec(`UPDATE user_admins SET created_at = NOW() WHERE created_at IS NULL`)

	// Create a unique composite index on purchases to prevent duplicate purchases.
	// A user can only purchase a note once.
	// We use raw SQL here because GORM's AutoMigrate doesn't handle composite unique indexes well.
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_purchases_user_note 
		ON purchases(user_id, note_id)
	`).Error; err != nil {
		log.Printf("Warning: Could not create unique index on purchases (may already exist): %v", err)
		// Don't return error - index might already exist
	}

	// Composite unique index on comment votes to prevent double-voting.
	// One vote per user per comment, enforced at the DB level.
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_comment_votes_user_comment
		ON comment_votes(comment_id, user_id)
	`).Error; err != nil {
		log.Printf("Warning: Could not create unique index on comment_votes (may already exist): %v", err)
	}

	// Partial unique index on usernames — only enforced when username is non-empty.
	// This allows multiple users to have no username (empty string default)
	// while ensuring chosen usernames are globally unique.
	// Drop any old full unique index left from earlier schema versions first.
	db.Exec(`DROP INDEX IF EXISTS idx_users_username`)
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_nonempty
		ON users(username) WHERE username != ''
	`).Error; err != nil {
		log.Printf("Warning: Could not create partial unique index on users.username (may already exist): %v", err)
	}

	// Partial unique index on display_name — only enforced when non-empty.
	// Chosen display names must be globally unique (case-sensitive for now).
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_users_display_name_nonempty
		ON users(display_name) WHERE display_name != ''
	`).Error; err != nil {
		log.Printf("Warning: Could not create partial unique index on users.display_name (may already exist): %v", err)
	}

	// ── Performance indexes ────────────────────────────────────────────────
	// These indexes accelerate the most common query patterns at scale.
	perfIndexes := []string{
		// Note listing (approved feed, pending queue)
		`CREATE INDEX IF NOT EXISTS idx_notes_status_created ON notes(status, created_at DESC)`,
		// Community note browsing
		`CREATE INDEX IF NOT EXISTS idx_notes_subnotery_status ON notes(subnotery_id, status)`,
		// Comment tree building
		`CREATE INDEX IF NOT EXISTS idx_comments_note_parent ON comments(note_id, parent_id)`,
		// My comments listing
		`CREATE INDEX IF NOT EXISTS idx_comments_user_created ON comments(user_id, created_at DESC)`,
		// Vote lookups (does user have existing vote on this note?)
		`CREATE INDEX IF NOT EXISTS idx_votes_user_note ON votes(user_id, note_id)`,
		// Email lookup for login/OAuth
		`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
		// Bookmark listing
		`CREATE INDEX IF NOT EXISTS idx_bookmarks_user_created ON bookmarks(user_id, created_at DESC)`,
		// Ban checks
		`CREATE INDEX IF NOT EXISTS idx_bans_user_subnotery ON bans(user_id, subnotery_id)`,
		// Order history
		`CREATE INDEX IF NOT EXISTS idx_orders_user_created ON orders(user_id, created_at DESC)`,
		// Notification listing
		`CREATE INDEX IF NOT EXISTS idx_notifications_user_read ON notifications(user_id, read, created_at DESC)`,
	}
	for _, sql := range perfIndexes {
		if err := db.Exec(sql).Error; err != nil {
			log.Printf("Warning: index creation skipped: %v", err)
		}
	}

	// Backfill materialized paths for comments that don't have one yet.
	// This is idempotent — only touches rows with empty path.
	// Uses iterative depth-first: set depth-0 paths first, then depth-1, etc.
	backfillCommentPaths(db)

	return nil
}

// getenv returns the value of an environment variable or a default if not set.
func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// getenvInt returns the value of an environment variable as an integer or a default if not set.
func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

// backfillCommentPaths iteratively sets materialized paths for comments that
// don't have one yet. Processes depth-0 first, then depth-1, etc.
// This is idempotent and safe to run on every startup — it only updates rows
// with empty path fields and stops when no more rows need updating.
func backfillCommentPaths(db *gorm.DB) {
	// Check if there are any comments without paths
	var count int64
	db.Model(&models.Comment{}).Where("path = '' OR path IS NULL").Count(&count)
	if count == 0 {
		return
	}
	log.Printf("Backfilling materialized paths for %d comments...", count)

	// Process by depth level: depth 0 first, then 1, 2, etc.
	for depth := 0; depth <= 20; depth++ {
		var comments []models.Comment
		query := db.Where("depth = ? AND (path = '' OR path IS NULL)", depth)
		if err := query.FindInBatches(&comments, 500, func(tx *gorm.DB, batch int) error {
			for i := range comments {
				c := &comments[i]
				var newPath string
				if c.ParentID == nil {
					// Top-level comment: /<id>/
					newPath = fmt.Sprintf("/%d/", c.ID)
				} else {
					// Reply: look up parent's path
					var parent models.Comment
					if err := db.Select("id, path").First(&parent, *c.ParentID).Error; err != nil {
						log.Printf("Warning: backfill skip comment %d — parent %d not found: %v", c.ID, *c.ParentID, err)
						continue
					}
					if parent.Path == "" {
						log.Printf("Warning: backfill skip comment %d — parent %d has empty path", c.ID, *c.ParentID)
						continue
					}
					newPath = fmt.Sprintf("%s%d/", parent.Path, c.ID)
				}
				if err := db.Model(c).Update("path", newPath).Error; err != nil {
					log.Printf("Warning: backfill failed for comment %d: %v", c.ID, err)
				}
			}
			return nil
		}).Error; err != nil {
			log.Printf("Warning: backfill error at depth %d: %v", depth, err)
			break
		}

		if len(comments) == 0 {
			break // No more comments at this depth or beyond
		}
	}

	log.Println("Materialized path backfill complete.")
}
