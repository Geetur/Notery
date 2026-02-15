// Package config centralizes environment variable loading for the Notery API.
// All configuration is loaded once at startup, eliminating per-request godotenv.Load() calls.
package config

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all application configuration values loaded from environment variables.
type Config struct {
	// JWTSecret is the HMAC key used to sign and verify JWT tokens.
	JWTSecret string

	// StripeSecretKey is the Stripe API secret key (sk_live_xxx or sk_test_xxx).
	StripeSecretKey string

	// StripeWebhookSecret is the signing secret for verifying Stripe webhook signatures (whsec_xxx).
	StripeWebhookSecret string

	// CORSOrigins is the list of allowed origins for cross-origin requests.
	// Loaded from CORS_ORIGINS env var (comma-separated). Defaults to localhost dev ports.
	CORSOrigins []string
}

// Load reads the .env file (if present) and returns a populated Config.
// Call this once at application startup, before initializing any other components.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found (ok):", err)
	}

	cfg := &Config{
		JWTSecret:           os.Getenv("JWT_SECRET"),
		StripeSecretKey:     os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		CORSOrigins:         parseCORSOrigins(os.Getenv("CORS_ORIGINS")),
	}

	if cfg.JWTSecret == "" {
		log.Println("WARNING: JWT_SECRET is not set — authentication will fail")
	}
	if cfg.StripeSecretKey == "" {
		log.Println("INFO: STRIPE_SECRET_KEY is not set — payments will auto-fulfil (development mode)")
	}

	return cfg
}

// parseCORSOrigins splits a comma-separated string of origins into a slice.
// Defaults to localhost:3000 and localhost:5173 if empty.
func parseCORSOrigins(raw string) []string {
	if raw == "" {
		return []string{"http://localhost:3000", "http://localhost:5173"}
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	if len(origins) == 0 {
		return []string{"http://localhost:3000", "http://localhost:5173"}
	}
	return origins
}
