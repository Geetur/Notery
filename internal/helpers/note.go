// note.go — Note fetching and ID parsing helpers.
package helpers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/models"
)

// ----- NOTE HELPERS -----

// MustParseNoteID extracts the "id" parameter as a note ID.
// On failure, sends a 400 response and returns 0, false.
// Usage: noteID, ok := helpers.MustParseNoteID(c); if !ok { return }
func MustParseNoteID(c *gin.Context) (uint64, bool) {
	noteID, ok := ParseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid note ID"})
		return 0, false
	}
	return noteID, true
}

// FetchNote retrieves a note by ID from the database.
// Returns the note and true on success.
// On failure, sends appropriate HTTP response (404 or 500) and returns nil, false.
func FetchNote(c *gin.Context, db *gorm.DB, noteID uint64) (*models.Note, bool) {
	var note models.Note
	if err := db.First(&note, noteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch note"})
		}
		return nil, false
	}
	return &note, true
}

// MustFetchNote combines MustParseNoteID and FetchNote for the common pattern.
// Extracts note ID from "id" param, fetches from DB, handles all errors.
// Usage: note, ok := helpers.MustFetchNote(c, db); if !ok { return }
func MustFetchNote(c *gin.Context, db *gorm.DB) (*models.Note, bool) {
	noteID, ok := MustParseNoteID(c)
	if !ok {
		return nil, false
	}
	return FetchNote(c, db, noteID)
}
