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

	// BaseURL is the public-facing API URL used for email verification links.
	BaseURL string

	// CORSOrigins is a comma-separated list of allowed CORS origins.
	// Defaults to "http://localhost:3000,http://localhost:5173" for development.
	CORSOrigins []string

	// SMTPHost is the SMTP server host for sending emails.
	SMTPHost string

	// SMTPPort is the SMTP server port.
	SMTPPort string

	// SMTPUser is the SMTP authentication username.
	SMTPUser string

	// SMTPPass is the SMTP authentication password.
	SMTPPass string

	// SMTPFrom is the "From" address for outgoing emails.
	SMTPFrom string
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
		BaseURL:             getenv("BASE_URL", "http://localhost:8080"),
		CORSOrigins:         parseCORSOrigins(getenv("CORS_ORIGINS", "http://localhost:3000,http://localhost:5173")),
		SMTPHost:            os.Getenv("SMTP_HOST"),
		SMTPPort:            getenv("SMTP_PORT", "587"),
		SMTPUser:            os.Getenv("SMTP_USER"),
		SMTPPass:            os.Getenv("SMTP_PASS"),
		SMTPFrom:            getenv("SMTP_FROM", "noreply@notery.app"),
	}

	if cfg.JWTSecret == "" {
		log.Println("WARNING: JWT_SECRET is not set — authentication will fail")
	}
	if cfg.StripeSecretKey == "" {
		log.Println("INFO: STRIPE_SECRET_KEY is not set — payments will auto-fulfil (development mode)")
	}

	return cfg
}

// getenv returns the value of an environment variable or a default if not set.
func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// parseCORSOrigins splits a comma-separated origin string into a slice,
// trimming whitespace from each entry and discarding empty values.
func parseCORSOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			origins = append(origins, p)
		}
	}
	return origins
}
