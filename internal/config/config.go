// Package config centralizes environment variable loading for the Notery API.
// All configuration is loaded once at startup, eliminating per-request godotenv.Load() calls.
package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all application configuration values loaded from environment variables.
type Config struct {
	// JWTSecret is the HMAC key used to sign and verify JWT tokens.
	JWTSecret string
}

// Load reads the .env file (if present) and returns a populated Config.
// Call this once at application startup, before initializing any other components.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found (ok):", err)
	}

	cfg := &Config{
		JWTSecret: os.Getenv("JWT_SECRET"),
	}

	if cfg.JWTSecret == "" {
		log.Println("WARNING: JWT_SECRET is not set — authentication will fail")
	}

	return cfg
}
