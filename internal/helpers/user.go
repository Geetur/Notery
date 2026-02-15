// user.go — User fetching helpers.
package helpers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/models"
)

// ----- USER HELPERS -----

// FetchUser retrieves a user by ID from the database.
// Returns the user and true on success.
// On failure, sends 404 or 500 response and returns nil, false.
func FetchUser(c *gin.Context, db *gorm.DB, userID uint64) (*models.User, bool) {
	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user"})
		}
		return nil, false
	}
	return &user, true
}

// FetchUserByEmail retrieves a user by email from the database.
// Returns the user and true on success, nil and false if not found.
func FetchUserByEmail(db *gorm.DB, email string) (*models.User, bool) {
	var user models.User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, false
	}
	return &user, true
}
