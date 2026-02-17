// Package handlers provides HTTP request handlers for the Notery API.
// This file defines the unified App struct that consolidates all handler dependencies.
package handlers

import (
	"github.com/meilisearch/meilisearch-go"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/database"
	"github.com/Geetur/Notery/internal/email"
	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/payment"
)

// appLog is the domain-specific logger for app-level operations.
var appLog = helpers.NewLogger("APP")

// App is the unified application struct that holds all shared dependencies.
// All handler methods are defined on App, eliminating redundant struct definitions
// and simplifying dependency injection.
type App struct {
	// Database connection (PostgreSQL via GORM)
	DB *gorm.DB

	// Redis client for caching, sessions, and real-time features
	RDB *redis.Client

	// Cloudflare R2 client for PDF storage
	R2 *database.R2Client

	// Meilisearch client for full-text search
	Search      meilisearch.ServiceManager
	SearchIndex string

	// JWTSecret is the signing key for JWT authentication tokens.
	JWTSecret string

	// Payment is the payment processing service (e.g., Stripe).
	// When nil, purchases auto-fulfil without payment (development mode).
	Payment payment.Service

	// Mailer sends transactional emails (verification, password reset).
	// When SMTP is not configured, falls back to LogMailer (stdout).
	Mailer email.Mailer

	// BaseURL is the public URL for this API instance (used in email links).
	BaseURL string
}

// AppConfig holds the configuration options for creating an App instance.
type AppConfig struct {
	DB          *gorm.DB
	Redis       *redis.Client
	R2          *database.R2Client
	Meilisearch meilisearch.ServiceManager
	SearchIndex string
	JWTSecret   string
	Payment     payment.Service
	Mailer      email.Mailer
	BaseURL     string
}

// NewApp creates and returns a new App instance with the given configuration.
// All dependencies are required except R2 and Meilisearch which are optional.
func NewApp(cfg AppConfig) *App {
	app := &App{
		DB:          cfg.DB,
		RDB:         cfg.Redis,
		R2:          cfg.R2,
		Search:      cfg.Meilisearch,
		SearchIndex: cfg.SearchIndex,
		JWTSecret:   cfg.JWTSecret,
		Payment:     cfg.Payment,
		Mailer:      cfg.Mailer,
		BaseURL:     cfg.BaseURL,
	}

	appLog.Log("INIT", "App initialized", "hasDB", cfg.DB != nil, "hasRedis", cfg.Redis != nil, "hasR2", cfg.R2 != nil, "hasMeili", cfg.Meilisearch != nil, "hasJWT", cfg.JWTSecret != "", "hasPayment", cfg.Payment != nil)
	return app
}
