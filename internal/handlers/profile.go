// Package handlers/profile.go contains HTTP handlers for user profile management.
//
// ENDPOINTS:
//
//	GET   /me/profile           Authenticated user's own profile (full view)
//	PATCH /me/profile           Update own profile (partial update)
//	GET   /users/:id/profile    Public profile of any user (privacy-respecting)
//
// DESIGN:
//
//	Profile data lives on the User model (no separate table) for simplicity.
//	Public profiles respect the user's visibility setting:
//	  - "public": bio, avatar visible to everyone
//	  - "private": only username and display_name shown publicly
//
//	DTOs are used for all responses to prevent leaking sensitive fields.
//	Input validation enforces length limits, character constraints, and
//	whitespace normalization for all string fields.
package handlers

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// profileLog is the domain-specific logger for profile operations.
var profileLog = helpers.NewLogger("PROFILE")

// usernameRegex defines allowed characters for usernames: alphanumeric, underscores, hyphens.
// Must start with a letter or digit.
var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// displayNameRegex allows letters, digits, spaces, underscores, hyphens, and periods.
var displayNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 _.'-]*$`)

// ----- GET /me/profile -----

// GetMyProfile returns the authenticated user's full profile including private fields.
//
// Response:
//
//	{
//	  "id": 1,
//	  "email": "user@example.com",
//	  "username": "johndoe",
//	  "display_name": "John Doe",
//	  "bio": "I like notes",
//	  "avatar_url": "https://...",
//	  "profile_visibility": "public",
//	  "profile_updated_at": "2025-01-01T00:00:00Z",
//	  "created_at": "2025-01-01T00:00:00Z",
//	  "updated_at": "2025-01-01T00:00:00Z"
//	}
//
// Route: GET /api/v1/me/profile
func (app *App) GetMyProfile(c *gin.Context) {
	userID := helpers.GetUserID(c)

	var user models.User
	if err := app.DB.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		profileLog.Log("GET_SELF", "db error", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch profile"})
		return
	}

	c.JSON(http.StatusOK, user.SelfProfile())
}

// ----- PATCH /me/profile -----

// UpdateProfileRequest defines the allowed fields for profile updates.
// All fields are optional (pointer types) for partial update semantics.
type UpdateProfileRequest struct {
	DisplayName *string `json:"display_name"`
	Bio         *string `json:"bio"`
	AvatarURL   *string `json:"avatar_url"`
	Visibility  *string `json:"profile_visibility"`
	Username    *string `json:"username"`
}

// UpdateMyProfile handles partial updates to the authenticated user's profile.
//
// Request body (all fields optional):
//
//	{
//	  "display_name": "New Name",
//	  "bio": "About me",
//	  "avatar_url": "https://example.com/avatar.png",
//	  "profile_visibility": "public",
//	  "username": "newhandle"
//	}
//
// Route: PATCH /api/v1/me/profile
func (app *App) UpdateMyProfile(c *gin.Context) {
	userID := helpers.GetUserID(c)

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	var user models.User
	if err := app.DB.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		profileLog.Log("UPDATE", "db error fetching user", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch profile"})
		return
	}

	updates := make(map[string]interface{})

	// --- Username validation ---
	if req.Username != nil {
		username := strings.TrimSpace(*req.Username)
		if username != "" {
			if err := validateUsername(username); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		updates["username"] = username
	}

	// --- Display name validation ---
	if req.DisplayName != nil {
		dn := normalizeWhitespace(*req.DisplayName)
		if dn != "" {
			runeCount := utf8.RuneCountInString(dn)
			if runeCount < models.MinDisplayNameLength {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Display name too short",
					"min":   models.MinDisplayNameLength,
				})
				return
			}
			if runeCount > models.MaxDisplayNameLength {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Display name too long",
					"max":   models.MaxDisplayNameLength,
				})
				return
			}
			if !displayNameRegex.MatchString(dn) {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Display name contains invalid characters (letters, digits, spaces, hyphens, underscores, periods allowed)",
				})
				return
			}
		}
		updates["display_name"] = dn
	}

	// --- Bio validation ---
	if req.Bio != nil {
		bio := strings.TrimSpace(*req.Bio)
		if utf8.RuneCountInString(bio) > models.MaxBioLength {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Bio too long",
				"max":   models.MaxBioLength,
			})
			return
		}
		updates["bio"] = bio
	}

	// --- Avatar URL validation ---
	if req.AvatarURL != nil {
		avatar := strings.TrimSpace(*req.AvatarURL)
		if avatar != "" {
			if len(avatar) > models.MaxAvatarURLLength {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Avatar URL too long",
					"max":   models.MaxAvatarURLLength,
				})
				return
			}
			if !strings.HasPrefix(avatar, "https://") {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Avatar URL must use HTTPS",
				})
				return
			}
		}
		updates["avatar_url"] = avatar
	}

	// --- Visibility validation ---
	if req.Visibility != nil {
		vis := models.ProfileVisibility(strings.TrimSpace(*req.Visibility))
		if !models.ValidProfileVisibility(vis) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid profile visibility (must be 'public' or 'private')",
			})
			return
		}
		updates["profile_visibility"] = vis
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No fields to update"})
		return
	}

	if err := app.DB.Model(&user).Updates(updates).Error; err != nil {
		// Handle unique constraint violations (username or display_name taken)
		errStr := err.Error()
		if strings.Contains(errStr, "unique") || strings.Contains(errStr, "UNIQUE") ||
			strings.Contains(errStr, "duplicate") || strings.Contains(errStr, "idx_users_username") ||
			strings.Contains(errStr, "idx_users_display_name") {
			c.JSON(http.StatusConflict, gin.H{"error": "Username or display name already taken"})
			return
		}
		profileLog.Log("UPDATE", "db error", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile"})
		return
	}

	// Re-fetch updated user for response
	app.DB.First(&user, userID)

	profileLog.Log("UPDATE", "success", "user_id", userID, "fields", len(updates))
	c.JSON(http.StatusOK, user.SelfProfile())
}

// ----- GET /users/:id/profile -----

// GetUserProfile returns the public profile of a user by ID.
// Respects the user's privacy settings (public vs private).
//
// Route: GET /api/v1/users/:id/profile
func (app *App) GetUserProfile(c *gin.Context) {
	targetID, ok := helpers.ParseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var user models.User
	if err := app.DB.First(&user, targetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		profileLog.Log("GET_PUBLIC", "db error", "target_id", targetID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch profile"})
		return
	}

	c.JSON(http.StatusOK, user.PublicProfile())
}

// ===== VALIDATION HELPERS =====

// validateUsername checks username format and length constraints.
func validateUsername(username string) error {
	runeCount := utf8.RuneCountInString(username)
	if runeCount < models.MinUsernameLength {
		return errors.New("username too short (min 3 characters)")
	}
	if runeCount > models.MaxUsernameLength {
		return errors.New("username too long (max 30 characters)")
	}
	if !usernameRegex.MatchString(username) {
		return errors.New("username can only contain letters, digits, underscores, and hyphens, and must start with a letter or digit")
	}
	return nil
}

// normalizeWhitespace trims outer whitespace and collapses internal runs of
// whitespace to single spaces. Example: "  John   Doe  " → "John Doe".
func normalizeWhitespace(s string) string {
	s = strings.TrimSpace(s)
	// Collapse internal whitespace runs
	parts := strings.Fields(s)
	return strings.Join(parts, " ")
}
