// Package config centralizes environment variable loading for the Notery API.
// All configuration is loaded once at startup, eliminating per-request godotenv.Load() calls.
package config

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// IsProduction returns true when the ENVIRONMENT env var is set to "production".
func IsProduction() bool {
	return strings.EqualFold(os.Getenv("ENVIRONMENT"), "production")
}

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

	// BaseURL is the public URL for this API (used in email links).
	// Loaded from BASE_URL env var. Defaults to http://localhost:8080.
	BaseURL string

	// SMTP configuration for outbound emails.
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string

	// OAuth2 configuration for Google and GitHub login.
	GoogleClientID     string
	GoogleClientSecret string
	GitHubClientID     string
	GitHubClientSecret string

	// FrontendURL is the public URL for the frontend (used for OAuth redirects).
	// Loaded from FRONTEND_URL env var. Defaults to http://localhost:3000.
	FrontendURL string
}

// Load reads the .env file (if present) and returns a populated Config.
// Call this once at application startup, before initializing any other components.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found (ok):", err)
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	cfg := &Config{
		JWTSecret:           os.Getenv("JWT_SECRET"),
		StripeSecretKey:     os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		CORSOrigins:         parseCORSOrigins(os.Getenv("CORS_ORIGINS")),
		BaseURL:             baseURL,
		SMTPHost:            os.Getenv("SMTP_HOST"),
		SMTPPort:            os.Getenv("SMTP_PORT"),
		SMTPUser:            os.Getenv("SMTP_USER"),
		SMTPPass:            os.Getenv("SMTP_PASS"),
		SMTPFrom:            os.Getenv("SMTP_FROM"),
		GoogleClientID:      os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret:  os.Getenv("GOOGLE_CLIENT_SECRET"),
		GitHubClientID:      os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret:  os.Getenv("GITHUB_CLIENT_SECRET"),
		FrontendURL:         frontendURL,
	}

	if cfg.JWTSecret == "" {
		if IsProduction() {
			log.Fatal("FATAL: JWT_SECRET is not set — refusing to start in production")
		}
		log.Println("WARNING: JWT_SECRET is not set — authentication will fail")
	}
	if cfg.StripeSecretKey == "" {
		log.Println("INFO: STRIPE_SECRET_KEY is not set — payments will auto-fulfil (development mode)")
	}
	if cfg.StripeWebhookSecret == "" && cfg.StripeSecretKey != "" {
		if IsProduction() {
			log.Fatal("FATAL: STRIPE_WEBHOOK_SECRET is not set while Stripe is active — refusing to start in production")
		}
		log.Println("WARNING: STRIPE_WEBHOOK_SECRET is not set — webhook verification disabled")
	}

	// In production, URLs must be explicitly configured.
	if IsProduction() {
		if baseURL == "" || strings.HasPrefix(baseURL, "http://localhost") {
			log.Fatal("FATAL: BASE_URL must be set to a public URL in production")
		}
		if frontendURL == "" || strings.HasPrefix(frontendURL, "http://localhost") {
			log.Fatal("FATAL: FRONTEND_URL must be set to a public URL in production")
		}
		if os.Getenv("CORS_ORIGINS") == "" {
			log.Fatal("FATAL: CORS_ORIGINS must be set in production")
		}
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
