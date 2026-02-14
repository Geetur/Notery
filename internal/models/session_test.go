package models

import (
	"testing"
	"time"
)

func TestGenerateSecureToken_Length(t *testing.T) {
	token, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 32 random bytes → 64 hex chars
	if len(token) != 64 {
		t.Fatalf("expected 64 hex chars, got %d (%s)", len(token), token)
	}
}

func TestGenerateSecureToken_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := GenerateSecureToken(16)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seen[token] {
			t.Fatalf("duplicate token on iteration %d: %s", i, token)
		}
		seen[token] = true
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	h1 := HashToken("test-token-123")
	h2 := HashToken("test-token-123")
	if h1 != h2 {
		t.Fatalf("same input should produce same hash: %s != %s", h1, h2)
	}
}

func TestHashToken_DifferentInputs(t *testing.T) {
	h1 := HashToken("token-a")
	h2 := HashToken("token-b")
	if h1 == h2 {
		t.Fatal("different inputs should produce different hashes")
	}
}

func TestHashToken_Length(t *testing.T) {
	h := HashToken("any-token")
	// SHA-256 → 64 hex chars
	if len(h) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(h))
	}
}

func TestRefreshToken_IsExpired_NotExpired(t *testing.T) {
	rt := RefreshToken{
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	if rt.IsExpired() {
		t.Fatal("token should NOT be expired")
	}
}

func TestRefreshToken_IsExpired_Expired(t *testing.T) {
	rt := RefreshToken{
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	if !rt.IsExpired() {
		t.Fatal("token SHOULD be expired")
	}
}

func TestEmailVerification_IsExpired_NotExpired(t *testing.T) {
	ev := EmailVerification{
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	if ev.IsExpired() {
		t.Fatal("verification should NOT be expired")
	}
}

func TestEmailVerification_IsExpired_Expired(t *testing.T) {
	ev := EmailVerification{
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	if !ev.IsExpired() {
		t.Fatal("verification SHOULD be expired")
	}
}

func TestRefreshToken_TableName(t *testing.T) {
	rt := RefreshToken{}
	if rt.TableName() != "refresh_tokens" {
		t.Fatalf("unexpected table name: %s", rt.TableName())
	}
}

func TestEmailVerification_TableName(t *testing.T) {
	ev := EmailVerification{}
	if ev.TableName() != "email_verifications" {
		t.Fatalf("unexpected table name: %s", ev.TableName())
	}
}

func TestSessionConstants(t *testing.T) {
	if AccessTokenTTL != 15*time.Minute {
		t.Fatalf("unexpected AccessTokenTTL: %v", AccessTokenTTL)
	}
	if RefreshTokenTTL != 30*24*time.Hour {
		t.Fatalf("unexpected RefreshTokenTTL: %v", RefreshTokenTTL)
	}
	if EmailVerificationTTL != 24*time.Hour {
		t.Fatalf("unexpected EmailVerificationTTL: %v", EmailVerificationTTL)
	}
	if RefreshTokenBytes != 32 {
		t.Fatalf("unexpected RefreshTokenBytes: %d", RefreshTokenBytes)
	}
	if EmailVerificationTokenBytes != 32 {
		t.Fatalf("unexpected EmailVerificationTokenBytes: %d", EmailVerificationTokenBytes)
	}
}
