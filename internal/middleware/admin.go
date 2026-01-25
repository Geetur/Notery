// Package middleware/admin.go provides middleware for admin authorization
package middleware

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/Geetur/Notery/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RequireAdmin is a middleware function that checks if the authenticated user has admin privileges
// RequireAdmin interacts with the database to verify admin status for a subnotery.
// RequireAdmin interacts with the User and Note models.
func RequireAdmin(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Println("Checking admin privileges...")

		// get user from context
		userIDValue, exists := c.Get("user_id")
		log.Println("Getting user from context:", userIDValue)
		if !exists {
			log.Println("No user in context")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		userIDStr := fmt.Sprint(userIDValue)
		userID, err := strconv.ParseUint(userIDStr, 10, 64)
		if err != nil {
			log.Println("Invalid user ID in context:", userIDStr)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		log.Println("User found in context:", userID)

		// check if user is a global admin
		log.Println("Checking if user is a global admin...")
		var user models.User
		if err := db.Select("id", "is_global_admin").First(&user, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.Println("User not found for admin check:", userID)
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
				return
			}
			log.Println("Failed to look up user for admin check:", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify admin status"})
			return
		}
		if user.IsGlobalAdmin {
			log.Println("Global admin privileges verified for user:", user.ID)
			c.Next()
			return
		}
		log.Println("User is not a global admin:", user.ID)

		// since user is not a global admin, check subnotery admin status

		// resolve subnotery context if provided or derivable from note
		subnoteryIDStr := c.Param("subnotery_id")
		var subnoteryID uint64
		if subnoteryIDStr != "" {
			log.Println("Subnotery ID found in route params:", subnoteryIDStr)
			subnoteryID, err = strconv.ParseUint(subnoteryIDStr, 10, 64)
			if err != nil {
				log.Println("Invalid subnotery ID in route params:", subnoteryIDStr)
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid subnotery ID"})
				return
			}
		} else {
			noteIDStr := c.Param("id")
			if noteIDStr != "" {
				log.Println("Deriving subnotery from note ID:", noteIDStr)
				noteID, parseErr := strconv.ParseUint(noteIDStr, 10, 64)
				if parseErr != nil {
					log.Println("Invalid note ID in route params:", noteIDStr)
					c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid note ID"})
					return
				}
				var note models.Note
				if err := db.Select("id", "subnotery_id").First(&note, noteID).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						log.Println("Note not found for admin check:", noteID)
						c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Note not found"})
						return
					}
					log.Println("Failed to look up note for admin check:", err)
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify admin status"})
					return
				}
				subnoteryID = uint64(note.SubnoteryID)
				if subnoteryID == 0 {
					log.Println("Note has no subnotery assigned:", note.ID)
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify admin status"})
					return
				}
				log.Println("Resolved subnotery ID from note:", subnoteryID)
			}
		}

		// if no subnotery context exists, check if user is admin of any subnotery
		log.Println("Verifying admin status for user...")
		if subnoteryID == 0 {
			var count int64
			err := db.Table("user_admins").
				Where("user_id = ?", user.ID).
				Count(&count).Error
			if err != nil {
				log.Println("Failed to verify admin status:", err)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify admin status"})
				return
			}
			if count == 0 {
				log.Println("User is not an admin of any subnotery:", user.ID)
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
				return
			}
			log.Println("Admin privileges verified for user:", user.ID)
			c.Next()
			return
		}

		// check if user is an admin of the resolved subnotery
		var count int64
		err = db.Table("user_admins").
			Where("user_id = ? AND subnotery_id = ?", user.ID, subnoteryID).
			Count(&count).Error
		if err != nil {
			log.Println("Failed to verify admin status for subnotery:", subnoteryID, err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify admin status"})
			return
		}
		if count == 0 {
			log.Println("User is not an admin for subnotery:", subnoteryID)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
			return
		}
		log.Println("Admin privileges verified for user:", user.ID)

		c.Next()
	}
}
