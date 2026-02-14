// Package handlers/bookmark.go contains HTTP handlers for saved/bookmarked notes.
//
// ARCHITECTURE:
//
//	Users can save (bookmark) approved notes for quick access later.
//	Bookmarks are stored in PostgreSQL with a composite unique index
//	on (user_id, note_id) to prevent duplicates.
//
//	Only approved notes can be bookmarked. If a bookmarked note is later
//	rejected or deleted, the bookmark remains but the note won't appear
//	in the saved list (filtered at query time).
//
// ENDPOINTS:
//
//	POST   /api/v1/notes/:id/bookmark   — Save a note
//	DELETE /api/v1/notes/:id/bookmark   — Unsave a note
//	GET    /api/v1/me/bookmarks         — List saved notes (paginated)
//	GET    /api/v1/notes/:id/bookmarked — Check if note is bookmarked
package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// bookmarkLog is the domain-specific logger for bookmark operations.
var bookmarkLog = helpers.NewLogger("BOOKMARK")

// BookmarkNote saves a note to the authenticated user's bookmarks.
// Only approved notes can be bookmarked. Duplicate bookmarks are idempotent (200 OK).
//
// Route: POST /api/v1/notes/:id/bookmark
func (app *App) BookmarkNote(c *gin.Context) {
	noteID, ok := helpers.MustParseNoteID(c)
	if !ok {
		return
	}
	userID := helpers.GetUserID(c)
	bookmarkLog.Log("ADD", "Processing bookmark request", "userID", userID, "noteID", noteID)

	// Verify note exists and is approved
	var note models.Note
	if err := app.DB.First(&note, noteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch note"})
		return
	}
	if note.Status != models.StatusApproved {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only approved notes can be bookmarked"})
		return
	}

	// Check if already bookmarked (idempotent)
	var existing models.Bookmark
	if err := app.DB.Where("user_id = ? AND note_id = ?", userID, noteID).First(&existing).Error; err == nil {
		bookmarkLog.Log("ADD", "Already bookmarked (idempotent)", "userID", userID, "noteID", noteID)
		c.JSON(http.StatusOK, gin.H{"message": "Note already saved", "bookmarked": true})
		return
	}

	bookmark := models.Bookmark{
		UserID:    userID,
		NoteID:    uint64(noteID),
		CreatedAt: time.Now(),
	}
	if err := app.DB.Create(&bookmark).Error; err != nil {
		bookmarkLog.Log("ADD", "Failed to create bookmark", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save note"})
		return
	}

	bookmarkLog.Log("ADD", "Note bookmarked successfully", "userID", userID, "noteID", noteID)
	c.JSON(http.StatusCreated, gin.H{"message": "Note saved", "bookmarked": true})
}

// RemoveBookmark removes a note from the authenticated user's bookmarks.
// Idempotent — removing a non-existent bookmark returns 200.
//
// Route: DELETE /api/v1/notes/:id/bookmark
func (app *App) RemoveBookmark(c *gin.Context) {
	noteID, ok := helpers.MustParseNoteID(c)
	if !ok {
		return
	}
	userID := helpers.GetUserID(c)
	bookmarkLog.Log("REMOVE", "Processing remove bookmark request", "userID", userID, "noteID", noteID)

	result := app.DB.Where("user_id = ? AND note_id = ?", userID, noteID).Delete(&models.Bookmark{})
	if result.Error != nil {
		bookmarkLog.Log("REMOVE", "Failed to remove bookmark", "error", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove bookmark"})
		return
	}

	bookmarkLog.Log("REMOVE", "Bookmark removed", "userID", userID, "noteID", noteID, "affected", result.RowsAffected)
	c.JSON(http.StatusOK, gin.H{"message": "Bookmark removed", "bookmarked": false})
}

// GetMyBookmarks returns the authenticated user's saved notes (paginated).
// Only includes notes that are still approved.
//
// Route: GET /api/v1/me/bookmarks
func (app *App) GetMyBookmarks(c *gin.Context) {
	userID := helpers.GetUserID(c)
	bookmarkLog.Log("LIST", "Processing list bookmarks request", "userID", userID)

	pg := helpers.ParsePagination(c)

	// Count total bookmarks for approved notes
	var total int64
	if err := app.DB.Model(&models.Bookmark{}).
		Joins("JOIN notes ON notes.id = bookmarks.note_id").
		Where("bookmarks.user_id = ? AND notes.status = ?", userID, models.StatusApproved).
		Count(&total).Error; err != nil {
		bookmarkLog.Log("LIST", "Failed to count bookmarks", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch bookmarks"})
		return
	}

	// Fetch bookmarked notes with bookmark timestamp
	type BookmarkedNote struct {
		models.Note
		BookmarkedAt time.Time `json:"bookmarked_at"`
	}

	var bookmarkedNotes []BookmarkedNote
	if err := app.DB.Table("bookmarks").
		Select("notes.*, bookmarks.created_at as bookmarked_at").
		Joins("JOIN notes ON notes.id = bookmarks.note_id").
		Where("bookmarks.user_id = ? AND notes.status = ?", userID, models.StatusApproved).
		Order("bookmarks.created_at DESC").
		Offset(pg.Offset).Limit(pg.Limit).
		Scan(&bookmarkedNotes).Error; err != nil {
		bookmarkLog.Log("LIST", "Failed to fetch bookmarks", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch bookmarks"})
		return
	}

	bookmarkLog.Log("LIST", "Bookmarks retrieved", "userID", userID, "count", len(bookmarkedNotes), "total", total)
	c.JSON(http.StatusOK, gin.H{
		"bookmarks": bookmarkedNotes,
		"total":     total,
		"page":      pg.Page,
		"limit":     pg.Limit,
	})
}

// CheckBookmarkStatus checks whether the authenticated user has bookmarked a note.
//
// Route: GET /api/v1/notes/:id/bookmarked
func (app *App) CheckBookmarkStatus(c *gin.Context) {
	noteID, ok := helpers.MustParseNoteID(c)
	if !ok {
		return
	}
	userID := helpers.GetUserID(c)

	var count int64
	app.DB.Model(&models.Bookmark{}).
		Where("user_id = ? AND note_id = ?", userID, noteID).
		Count(&count)

	c.JSON(http.StatusOK, gin.H{
		"bookmarked": count > 0,
		"note_id":    noteID,
	})
}
