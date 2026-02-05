// Package middleware/admin.go provides middleware for admin authorization.
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

// RequireAdmin is a middleware function that checks if the authenticated user has admin privileges
// RequireAdmin interacts with the database to verify admin status for a subnotery.
// RequireAdmin interacts with the User and Note models.
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
			_, err := strconv.ParseUint(subnoteryIDStr, 10, 64)
			if err != nil {
				mwLog.Log("ADMIN", "Invalid subnotery ID", "value", subnoteryIDStr)
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid subnotery ID"})
				return
			}
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

		// If no subnotery context, check if user is admin of any subnotery
		if subnoteryID == 0 {
			var count int64
			err := db.Table("user_admins").
				Where("user_id = ?", user.ID).
				Count(&count).Error
			if err != nil {
				mwLog.Log("ADMIN", "Failed to verify admin status", "error", err)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify admin status"})
				return
			}
			if count == 0 {
				mwLog.Log("ADMIN", "User is not admin of any subnotery", "userID", user.ID)
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
				return
			}
			mwLog.Log("ADMIN", "Admin privileges verified (any subnotery)", "userID", user.ID)
			c.Next()
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
		c.Next()
	}
}
