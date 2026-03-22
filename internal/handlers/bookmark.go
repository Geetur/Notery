// bookmark.go — Bookmark handlers: add, remove, list, and check bookmarks.
//
// ENDPOINTS:
//
//	POST   /bookmarks/:note_id   Add a bookmark for the given note
//	DELETE /bookmarks/:note_id   Remove a bookmark for the given note
//	GET    /bookmarks            List the authenticated user's bookmarks (paginated)
//	GET    /bookmarks/:note_id   Check whether a note is bookmarked by the user
//
// DESIGN:
//
//	Bookmarks use a DB table (bookmarks) with a unique composite index on
//	(user_id, note_id). Only approved notes can be bookmarked. Duplicate adds
//	are idempotent (return success). The list endpoint returns full Note objects
//	joined from the bookmarks table.
package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// bookmarkLog is the domain-specific logger for bookmark operations.
var bookmarkLog = helpers.NewLogger("BOOKMARK")

// AddBookmark saves a note to the user's bookmarks.
//
// Only approved notes can be bookmarked. Duplicate bookmarks are silently
// accepted (idempotent). Returns 201 on success or 200 if already bookmarked.
//
// DB: SELECT note (verify exists + approved), INSERT bookmark (ON CONFLICT ignore).
// Technologies: PostgreSQL (GORM).
// Helpers: helpers.GetUserID.
//
// Route: POST /api/v1/bookmarks/:note_id
func (app *App) AddBookmark(c *gin.Context) {
	userID := helpers.GetUserID(c)
	noteID, err := strconv.ParseUint(c.Param("note_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid note ID"})
		return
	}

	// Verify note exists and is approved.
	var note models.Note
	if err := app.DB.First(&note, noteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
			return
		}
		bookmarkLog.Log("ADD", "DB error looking up note", "noteID", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add bookmark"})
		return
	}
	if note.Status != models.StatusApproved {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only approved notes can be bookmarked"})
		return
	}

	bookmark := models.Bookmark{
		UserID: userID,
		NoteID: uint(noteID),
	}

	result := app.DB.Where("user_id = ? AND note_id = ?", userID, noteID).FirstOrCreate(&bookmark)
	if result.Error != nil {
		bookmarkLog.Log("ADD", "DB error", "userID", userID, "noteID", noteID, "error", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add bookmark"})
		return
	}

	if result.RowsAffected == 0 {
		bookmarkLog.Log("ADD", "Already bookmarked", "userID", userID, "noteID", noteID)
		c.JSON(http.StatusOK, gin.H{"message": "Already bookmarked", "bookmarked": true})
		return
	}

	bookmarkLog.Log("ADD", "Bookmark added", "userID", userID, "noteID", noteID)
	c.JSON(http.StatusCreated, gin.H{"message": "Bookmark added", "bookmarked": true})
}

// RemoveBookmark removes a note from the user's bookmarks.
//
// Idempotent — removing a non-existent bookmark returns success.
//
// DB: DELETE from bookmarks WHERE user_id AND note_id.
// Technologies: PostgreSQL (GORM).
// Helpers: helpers.GetUserID.
//
// Route: DELETE /api/v1/bookmarks/:note_id
func (app *App) RemoveBookmark(c *gin.Context) {
	userID := helpers.GetUserID(c)
	noteID, err := strconv.ParseUint(c.Param("note_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid note ID"})
		return
	}

	if err := app.DB.Where("user_id = ? AND note_id = ?", userID, noteID).Delete(&models.Bookmark{}).Error; err != nil {
		bookmarkLog.Log("REMOVE", "DB error", "userID", userID, "noteID", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove bookmark"})
		return
	}

	bookmarkLog.Log("REMOVE", "Bookmark removed", "userID", userID, "noteID", noteID)
	c.JSON(http.StatusOK, gin.H{"message": "Bookmark removed", "bookmarked": false})
}

// GetBookmarks lists the authenticated user's bookmarked notes (paginated).
//
// Returns full Note objects by joining bookmarks → notes, ordered by bookmark
// creation time (newest first). Only approved notes are returned.
//
// DB: COUNT + SELECT from bookmarks JOIN notes, paginated with OFFSET/LIMIT.
// Technologies: PostgreSQL (GORM JOIN).
// Helpers: helpers.GetUserID, helpers.ParsePagination.
//
// Route: GET /api/v1/bookmarks
func (app *App) GetBookmarks(c *gin.Context) {
	userID := helpers.GetUserID(c)
	pag := helpers.ParsePagination(c)

	var total int64
	app.DB.Model(&models.Bookmark{}).
		Joins("JOIN notes ON notes.id = bookmarks.note_id").
		Where("bookmarks.user_id = ? AND notes.status = ?", userID, models.StatusApproved).
		Count(&total)

	var bookmarks []models.Bookmark
	if err := app.DB.
		Where("user_id = ?", userID).
		Joins("JOIN notes ON notes.id = bookmarks.note_id AND notes.status = ?", models.StatusApproved).
		Order("bookmarks.created_at DESC").
		Offset(pag.Offset).Limit(pag.Limit).
		Find(&bookmarks).Error; err != nil {
		bookmarkLog.Log("LIST", "DB error", "userID", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list bookmarks"})
		return
	}

	// Fetch actual notes for the bookmarks.
	noteIDs := make([]uint, len(bookmarks))
	for i, b := range bookmarks {
		noteIDs[i] = b.NoteID
	}

	var notes []models.Note
	if len(noteIDs) > 0 {
		if err := app.DB.Where("id IN ?", noteIDs).Find(&notes).Error; err != nil {
			bookmarkLog.Log("LIST", "DB error fetching notes", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list bookmarks"})
			return
		}
	}

	// Maintain bookmark order (most recently bookmarked first).
	noteMap := make(map[uint]models.Note, len(notes))
	for _, n := range notes {
		noteMap[n.ID] = n
	}
	ordered := make([]models.Note, 0, len(noteIDs))
	for _, id := range noteIDs {
		if n, ok := noteMap[id]; ok {
			ordered = append(ordered, n)
		}
	}

	// Populate SubnoteryName and comment counts so cards render fully.
	app.populateSubnoteryNames(ordered)
	app.populateCommentCounts(ordered)

	bookmarkLog.Log("LIST", "Bookmarks listed", "userID", userID, "count", len(ordered), "total", total)
	c.JSON(http.StatusOK, gin.H{
		"notes": ordered,
		"total": total,
		"page":  pag.Page,
		"limit": pag.Limit,
	})
}

// CheckBookmark checks whether a note is bookmarked by the authenticated user.
//
// DB: SELECT 1 from bookmarks WHERE user_id AND note_id.
// Technologies: PostgreSQL (GORM).
// Helpers: helpers.GetUserID.
//
// Route: GET /api/v1/bookmarks/:note_id
func (app *App) CheckBookmark(c *gin.Context) {
	userID := helpers.GetUserID(c)
	noteID, err := strconv.ParseUint(c.Param("note_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid note ID"})
		return
	}

	var count int64
	app.DB.Model(&models.Bookmark{}).Where("user_id = ? AND note_id = ?", userID, noteID).Count(&count)

	c.JSON(http.StatusOK, gin.H{"bookmarked": count > 0})
}
