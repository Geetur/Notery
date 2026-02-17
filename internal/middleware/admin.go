// admin.go — Middleware for admin authorization (global and subnotery-scoped).
//
// MIDDLEWARE:
//
//	RequireAdmin  Verifies calling user is a global admin or subnotery admin.
//
// DESIGN:
//
//	Admin resolution follows a two-tier model:
//	  1. Global admins — is_global_admin flag on the User model. Grants access to
//	     all admin endpoints regardless of subnotery context.
//	  2. Subnotery admins — checked via the user_admins join table. Only grants
//	     access for operations within the specific subnotery.
//
//	Subnotery context is resolved from URL params: first from :subnotery_id, then
//	by deriving it from :id (note ID → note.subnotery_id). If no subnotery context
//	can be determined, non-global admins are denied with 403.
//
//	Sets "admin_type" (bool) and optionally "admin_subnotery_id" in Gin context
//	for downstream handlers.
//
// DB: SELECT user by ID, conditional SELECT note for subnotery derivation,
//     COUNT on user_admins join table.
// Technologies: PostgreSQL (GORM).
// Helpers: helpers.GetUserID.
package middleware

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// RequireAdmin returns middleware that checks if the authenticated user has admin
// privileges. Global admins pass immediately; subnotery admins are verified against
// the resolved subnotery context (from URL :subnotery_id or derived from :id note).
func RequireAdmin(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		mwLog.Log("ADMIN", "Checking admin privileges")

		// Get user from context (set by RequireAuth middleware)
		userID := helpers.GetUserID(c)
		mwLog.Log("ADMIN", "User identified", "userID", userID)

		// Check if user is a global admin
		var user models.User
		if err := db.Select("id", "is_global_admin").First(&user, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				mwLog.Log("ADMIN", "User not found", "userID", userID)
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
				return
			}
			mwLog.Log("ADMIN", "Failed to lookup user", "userID", userID, "error", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify admin status"})
			return
		}

		// Set admin type in context for downstream handlers
		c.Set("admin_type", user.IsGlobalAdmin)

		if user.IsGlobalAdmin {
			mwLog.Log("ADMIN", "Global admin verified", "userID", user.ID)
			c.Next()
			return
		}

		// Not a global admin - check subnotery admin status
		mwLog.Log("ADMIN", "Checking subnotery admin status", "userID", user.ID)

		// Resolve subnotery context from URL params
		subnoteryIDStr := c.Param("subnotery_id")
		var subnoteryID uint64
		if subnoteryIDStr != "" {
			mwLog.Log("ADMIN", "Subnotery ID in URL", "subnoteryID", subnoteryIDStr)
			parsedID, err := strconv.ParseUint(subnoteryIDStr, 10, 64)
			if err != nil {
				mwLog.Log("ADMIN", "Invalid subnotery ID", "value", subnoteryIDStr)
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid subnotery ID"})
				return
			}
			subnoteryID = parsedID
			mwLog.Log("ADMIN", "Resolved subnotery from URL param", "subnoteryID", subnoteryID)
		} else {
			// Try to derive subnotery from note ID
			noteIDStr := c.Param("id")
			if noteIDStr != "" {
				mwLog.Log("ADMIN", "Deriving subnotery from note ID", "noteID", noteIDStr)
				noteID, parseErr := strconv.ParseUint(noteIDStr, 10, 64)
				if parseErr != nil {
					mwLog.Log("ADMIN", "Invalid note ID", "value", noteIDStr)
					c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid note ID"})
					return
				}
				var note models.Note
				if err := db.Select("id", "subnotery_id").First(&note, noteID).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						mwLog.Log("ADMIN", "Note not found", "noteID", noteID)
						c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Note not found"})
						return
					}
					mwLog.Log("ADMIN", "Failed to lookup note", "error", err)
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify admin status"})
					return
				}
				subnoteryID = uint64(note.SubnoteryID)
				if subnoteryID == 0 {
					mwLog.Log("ADMIN", "Note has no subnotery", "noteID", note.ID)
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify admin status"})
					return
				}
				mwLog.Log("ADMIN", "Resolved subnotery from note", "subnoteryID", subnoteryID)
			}
		}

		// If no subnotery context could be resolved, deny access.
		// Subnotery admins must always operate within a resolved subnotery scope.
		// Without a specific subnotery to check against, we cannot verify authorization.
		if subnoteryID == 0 {
			mwLog.Log("ADMIN", "No subnotery context resolved, denying access", "userID", user.ID)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden: could not determine subnotery scope"})
			return
		}

		// Check if user is admin of the specific subnotery
		var count int64
		err := db.Table("user_admins").
			Where("user_id = ? AND subnotery_id = ?", user.ID, subnoteryID).
			Count(&count).Error
		if err != nil {
			mwLog.Log("ADMIN", "Failed to verify subnotery admin", "subnoteryID", subnoteryID, "error", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify admin status"})
			return
		}
		if count == 0 {
			mwLog.Log("ADMIN", "User not admin for subnotery", "userID", user.ID, "subnoteryID", subnoteryID)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
			return
		}

		mwLog.Log("ADMIN", "Subnotery admin verified", "userID", user.ID, "subnoteryID", subnoteryID)
		c.Set("admin_subnotery_id", subnoteryID)
		c.Next()
	}
}
