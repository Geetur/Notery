// session.go — Domain models for session and token management.
//
// SESSION ARCHITECTURE:
//
//	Authentication uses a short-lived access token (JWT, 15-minute expiry)
//	paired with a long-lived refresh token (opaque, 30-day expiry) stored in the DB.
//
//	Refresh tokens support rotation: each use invalidates the old token and issues
//	a new one. If a revoked token is reused (token theft detection), the entire
//	family is invalidated.
//
//	Email verification uses a separate token with a 24-hour TTL.
//
//	Token revocation is supported at two levels:
//	  - Single session: revoke one refresh token
//	  - All sessions: revoke all refresh tokens for a user (password change, account compromise)
package models

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// RefreshToken represents a persistent refresh token for session management.
// Each token belongs to a user and is part of a "family" for rotation tracking.
type RefreshToken struct {
	ID        uint      `json:"-" gorm:"primaryKey"`
	TokenHash string    `json:"-" gorm:"uniqueIndex;not null"` // SHA-256 hash of the opaque token
	UserID    uint64    `json:"-" gorm:"index;not null"`
	FamilyID  string    `json:"-" gorm:"index;not null"` // Groups rotated tokens for theft detection
	Revoked   bool      `json:"-" gorm:"default:false"`
	ExpiresAt time.Time `json:"-" gorm:"not null"`
	CreatedAt time.Time `json:"-"`
}

// TableName overrides GORM's default table name.
func (RefreshToken) TableName() string {
	return "refresh_tokens"
}

// IsExpired returns true if the token has passed its expiry time.
func (rt *RefreshToken) IsExpired() bool {
	return time.Now().After(rt.ExpiresAt)
}

// EmailVerification represents a pending email verification token.
type EmailVerification struct {
	ID        uint      `json:"-" gorm:"primaryKey"`
	UserID    uint64    `json:"-" gorm:"index;not null"`
	TokenHash string    `json:"-" gorm:"uniqueIndex;not null"` // SHA-256 hash
	ExpiresAt time.Time `json:"-" gorm:"not null"`
	CreatedAt time.Time `json:"-"`
}

// TableName overrides GORM's default table name.
func (EmailVerification) TableName() string {
	return "email_verifications"
}

// IsExpired returns true if the verification token has passed its expiry time.
func (ev *EmailVerification) IsExpired() bool {
	return time.Now().After(ev.ExpiresAt)
}

// PasswordReset represents a pending password reset token.
// Tokens are single-use and expire after PasswordResetTTL.
type PasswordReset struct {
	ID        uint      `json:"-" gorm:"primaryKey"`
	UserID    uint64    `json:"-" gorm:"index;not null"`
	TokenHash string    `json:"-" gorm:"uniqueIndex;not null"` // SHA-256 hash
	Used      bool      `json:"-" gorm:"default:false"`
	ExpiresAt time.Time `json:"-" gorm:"not null"`
	CreatedAt time.Time `json:"-"`
}

// TableName overrides GORM's default table name.
func (PasswordReset) TableName() string {
	return "password_resets"
}

// IsExpired returns true if the reset token has passed its expiry time.
func (pr *PasswordReset) IsExpired() bool {
	return time.Now().After(pr.ExpiresAt)
}

// Session lifecycle constants.
const (
	// AccessTokenTTL is the lifetime of a JWT access token.
	AccessTokenTTL = 15 * time.Minute

	// RefreshTokenTTL is the lifetime of a refresh token.
	RefreshTokenTTL = 30 * 24 * time.Hour // 30 days

	// EmailVerificationTTL is the lifetime of an email verification token.
	EmailVerificationTTL = 24 * time.Hour

	// RefreshTokenBytes is the number of random bytes for a refresh token.
	RefreshTokenBytes = 32

	// EmailVerificationTokenBytes is the number of random bytes for a verification token.
	EmailVerificationTokenBytes = 32

	// PasswordResetTTL is the lifetime of a password reset token.
	PasswordResetTTL = 1 * time.Hour

	// PasswordResetTokenBytes is the number of random bytes for a password reset token.
	PasswordResetTokenBytes = 32
)

// GenerateSecureToken creates a cryptographically random hex-encoded token.
func GenerateSecureToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashToken returns the SHA-256 hex digest of a token.
// We store only the hash in the database; the raw token is returned to the client.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// ZeroTime returns a time well in the past, useful for creating expired test tokens.
func ZeroTime() time.Time {
	return time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
}

// FutureTime returns a time 1 hour in the future, useful for creating valid test tokens.
func FutureTime() time.Time {
	return time.Now().Add(1 * time.Hour)
}
