// subnotery.go — Subnotery fetching and ID parsing helpers.
package helpers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/models"
)

// ----- SUBNOTERY HELPERS -----

// MustParseSubnoteryID extracts the "subnotery_id" parameter from URL.
// On failure, sends a 400 response and returns 0, false.
func MustParseSubnoteryID(c *gin.Context) (uint64, bool) {
	subnoteryID, ok := ParseUintParam(c, "subnotery_id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subnotery ID"})
		return 0, false
	}
	return subnoteryID, true
}

// FetchSubnotery retrieves a subnotery by ID from the database.
// Returns the subnotery and true on success.
// On failure, sends 404 or 500 response and returns nil, false.
func FetchSubnotery(c *gin.Context, db *gorm.DB, subnoteryID uint64) (*models.Subnotery, bool) {
	var subnotery models.Subnotery
	if err := db.First(&subnotery, subnoteryID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Subnotery not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subnotery"})
		}
		return nil, false
	}
	return &subnotery, true
}
