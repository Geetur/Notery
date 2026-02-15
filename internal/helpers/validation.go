// validation.go — Shared input validation utilities.
package helpers

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Geetur/Notery/internal/models"
)

// UsernameRegex defines allowed characters for usernames: alphanumeric, underscores, hyphens.
// Must start with a letter or digit.
var UsernameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// DisplayNameRegex allows letters, digits, spaces, underscores, hyphens, and periods.
var DisplayNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 _.'-]*$`)

// ValidateUsername checks username format and length constraints.
func ValidateUsername(username string) error {
	runeCount := utf8.RuneCountInString(username)
	if runeCount < models.MinUsernameLength {
		return errors.New("username too short (min 3 characters)")
	}
	if runeCount > models.MaxUsernameLength {
		return errors.New("username too long (max 30 characters)")
	}
	if !UsernameRegex.MatchString(username) {
		return errors.New("username can only contain letters, digits, underscores, and hyphens, and must start with a letter or digit")
	}
	return nil
}

// NormalizeWhitespace trims outer whitespace and collapses internal runs of
// whitespace to single spaces. Example: "  John   Doe  " → "John Doe".
func NormalizeWhitespace(s string) string {
	s = strings.TrimSpace(s)
	parts := strings.Fields(s)
	return strings.Join(parts, " ")
}
