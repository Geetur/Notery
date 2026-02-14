package config

import (
	"os"
	"testing"
)

func TestParseCORSOrigins_Default(t *testing.T) {
	origins := parseCORSOrigins("http://localhost:3000,http://localhost:5173")
	if len(origins) != 2 {
		t.Fatalf("got %d origins, want 2", len(origins))
	}
	if origins[0] != "http://localhost:3000" {
		t.Fatalf("first=%q, want http://localhost:3000", origins[0])
	}
	if origins[1] != "http://localhost:5173" {
		t.Fatalf("second=%q, want http://localhost:5173", origins[1])
	}
}

func TestParseCORSOrigins_WithSpaces(t *testing.T) {
	origins := parseCORSOrigins("  http://a.com , http://b.com  ")
	if len(origins) != 2 {
		t.Fatalf("got %d origins, want 2", len(origins))
	}
	if origins[0] != "http://a.com" {
		t.Fatalf("first=%q", origins[0])
	}
	if origins[1] != "http://b.com" {
		t.Fatalf("second=%q", origins[1])
	}
}

func TestParseCORSOrigins_SingleOrigin(t *testing.T) {
	origins := parseCORSOrigins("https://app.notery.io")
	if len(origins) != 1 || origins[0] != "https://app.notery.io" {
		t.Fatalf("origins=%v", origins)
	}
}

func TestParseCORSOrigins_EmptyString(t *testing.T) {
	origins := parseCORSOrigins("")
	if len(origins) != 0 {
		t.Fatalf("expected 0 origins for empty string, got %d", len(origins))
	}
}

func TestLoad_CORSOriginsDefault(t *testing.T) {
	// Clear env to test defaults
	os.Unsetenv("CORS_ORIGINS")
	cfg := Load()
	if len(cfg.CORSOrigins) != 2 {
		t.Fatalf("default CORS origins should have 2 entries, got %d", len(cfg.CORSOrigins))
	}
}

func TestLoad_CORSOriginsFromEnv(t *testing.T) {
	os.Setenv("CORS_ORIGINS", "https://prod.notery.io")
	defer os.Unsetenv("CORS_ORIGINS")

	cfg := Load()
	if len(cfg.CORSOrigins) != 1 || cfg.CORSOrigins[0] != "https://prod.notery.io" {
		t.Fatalf("CORS origins from env=%v", cfg.CORSOrigins)
	}
}
