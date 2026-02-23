// profile.go — HTTP handlers for user profile management.
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
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// profileLog is the domain-specific logger for profile operations.
var profileLog = helpers.ProfileLog

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
	Bio        *string `json:"bio"`
	AvatarURL  *string `json:"avatar_url"`
	Visibility *string `json:"profile_visibility"`
	Username   *string `json:"username"`
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
	if !helpers.BindJSON(c, &req) {
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
			if err := helpers.ValidateUsername(username); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		updates["username"] = username
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
