// verified.go — Middleware that requires email verification for write operations.
//
// MIDDLEWARE:
//
//	RequireVerified  Verifies the authenticated user has a verified email address.
//	                 Rejects with 403 if the user's email is not verified.
//
// DESIGN:
//
//	This middleware must run AFTER RequireAuth (it expects user_id in context).
//	It looks up the user in the database and checks the EmailVerified field.
//	Unverified users receive a clear error message directing them to verify.
//
// DB: SELECT user by ID.
// Technologies: PostgreSQL (GORM).
// Helpers: helpers.GetUserID.
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// RequireVerified returns middleware that checks if the authenticated user has
// verified their email address. Unverified users are rejected with 403.
// Must be used after RequireAuth middleware.
func RequireVerified(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := helpers.GetUserID(c)

		var user models.User
		if err := db.Select("id, email_verified").First(&user, userID).Error; err != nil {
			mwLog.Log("VERIFIED", "Failed to fetch user", "userID", userID, "error", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
			return
		}

		if !user.EmailVerified {
			mwLog.Log("VERIFIED", "Unverified user blocked", "userID", userID)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "Email verification required. Please verify your email to access this feature.",
				"code":  "EMAIL_NOT_VERIFIED",
			})
			return
		}

		c.Next()
	}
}
