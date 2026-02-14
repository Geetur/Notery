// Package handlers/note.go contains the HTTP handlers for note operations.
package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/meilisearch/meilisearch-go"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// noteLog is the domain-specific logger for note operations.
var noteLog = helpers.NoteLog

// deletePDFFromR2 removes a note's PDF from R2 storage if R2 is configured.
func (app *App) deletePDFFromR2(ctx context.Context, noteID uint) {
	if app.R2 != nil {
		if err := app.R2.DeletePDF(ctx, noteID); err != nil {
			noteLog.Log("R2", "Failed to delete PDF", "noteID", noteID, "error", err)
		} else {
			noteLog.Log("R2", "Deleted PDF", "noteID", noteID)
		}
	}
}

// CreateNote handles the creation of a new note.
// Auto-creates subnotery when missing and assigns the creator as its first admin.
func (app *App) CreateNote(c *gin.Context) {
	noteLog.Log("CREATE", "Processing note creation request")

	// Bind and validate request body
	var req struct {
		SubnoteryName string `json:"subnotery_name" binding:"required"`
		Title         string `json:"title"`
		Author        string `json:"author"`
		Price         int64  `json:"price"`
	}
	if !helpers.BindJSON(c, &req) {
		noteLog.Log("CREATE", "Failed to bind JSON request")
		return
	}
	noteLog.Log("CREATE", "Request validated", "subnotery", req.SubnoteryName, "title", req.Title)

	// Extract authenticated user ID
	userID := helpers.GetUserID(c)
	noteLog.Log("CREATE", "User identified", "userID", userID)

	// Basic validation
	if req.Title == "" || req.SubnoteryName == "" || req.Author == "" || req.Price < 0 {
		noteLog.Log("CREATE", "Validation failed: missing required fields")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Title, SubnoteryName, Author, and Price are required"})
		return
	}


	// Execute note creation in transaction
	var note models.Note
	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		var subnotery models.Subnotery
		subnoteryCreated := false

		// Find or create subnotery
		if err := tx.Where("name = ?", req.SubnoteryName).First(&subnotery).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				noteLog.Log("CREATE", "Creating new subnotery", "name", req.SubnoteryName)
				subnotery = models.Subnotery{Name: req.SubnoteryName}
				if err := tx.Create(&subnotery).Error; err != nil {
					noteLog.Log("CREATE", "Failed to create subnotery", "error", err)
					return err
				}
				subnoteryCreated = true
			} else {
				return err
			}
		}
		noteLog.Log("CREATE", "Subnotery resolved", "subnoteryID", subnotery.ID)

		// Assign creator as first admin if subnotery was just created
		if subnoteryCreated {
			var user models.User
			if err := tx.First(&user, userID).Error; err != nil {
				noteLog.Log("CREATE", "Failed to fetch user", "error", err)
				return err
			}
			if err := tx.Model(&subnotery).Association("Admins").Append(&user); err != nil {
				noteLog.Log("CREATE", "Failed to assign admin", "error", err)
				return err
			}
			if err := tx.Model(&subnotery).Association("Members").Append(&user); err != nil {
				noteLog.Log("CREATE", "Failed to add member", "error", err)
				return err
			}
			noteLog.Log("CREATE", "Creator assigned as admin/member", "subnoteryID", subnotery.ID)
		}

		// Create the note record
		note = models.Note{
			Title:       req.Title,
			Author:      req.Author,
			Price:       req.Price,
			Status:      models.StatusPending,
			SubnoteryID: subnotery.ID,
			CreatorID:   userID,
		}
		if err := tx.Create(&note).Error; err != nil {
			noteLog.Log("CREATE", "Failed to create note", "error", err)
			return err
		}
		return nil
	}); err != nil {
		noteLog.Log("CREATE", "Transaction failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to internally create note"})
		return
	}

	noteLog.Log("CREATE", "Note created successfully", "noteID", note.ID)
	c.JSON(http.StatusCreated, note)
}

// DeleteNote handles the deletion of a note by ID.
// DeleteNote removes the note from both the database and Meilisearch if approved.
func (app *App) DeleteNote(c *gin.Context) {
	noteLog.Log("DELETE", "Processing note deletion request")

	// Fetch the note using helper
	note, ok := helpers.MustFetchNote(c, app.DB)
	if !ok {
		noteLog.Log("DELETE", "Note not found")
		return
	}
	noteLog.Log("DELETE", "Note fetched", "noteID", note.ID, "status", note.Status)

	// Remove from Meilisearch and feed if approved
	if note.Status == models.StatusApproved {
		if err := app.removeNoteFromIndex(note.ID); err != nil {
			noteLog.Log("DELETE", "Failed to remove from search index", "noteID", note.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove approved note from search index"})
			return
		}
		if err := app.RemoveNoteFromFeed(c.Request.Context(), note); err != nil {
			noteLog.Log("DELETE", "Failed to remove from feed", "noteID", note.ID, "error", err)
		}
		noteLog.Log("DELETE", "Removed from search index and feed", "noteID", note.ID)
	}

	// Delete from database
	if err := app.DB.Delete(note).Error; err != nil {
		noteLog.Log("DELETE", "Failed to delete from database", "noteID", note.ID, "error", err)
		// Attempt to re-index if we had removed it
		if note.Status == models.StatusApproved {
			if reindexErr := app.indexNote(*note); reindexErr != nil {
				noteLog.Log("DELETE", "Failed to re-index after delete error", "error", reindexErr)
			}
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete note"})
		return
	}

	// Cleanup PDF from R2 if exists
	if note.HasPDF {
		app.deletePDFFromR2(c.Request.Context(), note.ID)
	}

	noteLog.Log("DELETE", "Note deleted successfully", "noteID", note.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Note deleted successfully"})
}

// RejectNote handles rejecting a note by ID.
func (app *App) RejectNote(c *gin.Context) {
	noteLog.Log("REJECT", "Processing note rejection request")

	// Fetch the note using helper
	note, ok := helpers.MustFetchNote(c, app.DB)
	if !ok {
		noteLog.Log("REJECT", "Note not found")
		return
	}
	noteLog.Log("REJECT", "Note fetched", "noteID", note.ID, "status", note.Status)

	previousStatus := note.Status

	// Update status to Rejected
	if err := app.DB.Model(note).Update("status", models.StatusRejected).Error; err != nil {
		noteLog.Log("REJECT", "Failed to update status", "noteID", note.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject note"})
		return
	}

	// Handle edge case: if note was approved, remove from search index
	if previousStatus == models.StatusApproved {
		noteLog.Log("REJECT", "Removing previously approved note from search", "noteID", note.ID)
		if err := app.removeNoteFromIndex(note.ID); err != nil {
			noteLog.Log("REJECT", "Failed to remove from search index", "error", err)
			// Rollback status update
			if rollbackErr := app.DB.Model(note).Update("status", previousStatus).Error; rollbackErr != nil {
				noteLog.Log("REJECT", "Failed to rollback status", "error", rollbackErr)
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove approved note from search index"})
			return
		}
		if err := app.RemoveNoteFromFeed(c.Request.Context(), note); err != nil {
			noteLog.Log("REJECT", "Failed to remove from feed", "noteID", note.ID, "error", err)
		}
	}

	// Delete the note from database
	if err := app.DB.Delete(note).Error; err != nil {
		noteLog.Log("REJECT", "Failed to delete note", "noteID", note.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete note after rejection"})
		return
	}

	// Cleanup PDF from R2 if exists
	if note.HasPDF {
		app.deletePDFFromR2(c.Request.Context(), note.ID)
	}

	noteLog.Log("REJECT", "Note rejected and deleted successfully", "noteID", note.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Note rejected successfully"})
}

// ApproveNote handles approving a note by ID.
// ApproveNote updates the note's status to "Approved" and indexes it in Meilisearch.
func (app *App) ApproveNote(c *gin.Context) {
	noteLog.Log("APPROVE", "Processing note approval request")

	// Fetch the note using helper
	note, ok := helpers.MustFetchNote(c, app.DB)
	if !ok {
		noteLog.Log("APPROVE", "Note not found")
		return
	}
	noteLog.Log("APPROVE", "Note fetched", "noteID", note.ID, "hasPDF", note.HasPDF)

	// Validate note has PDF content before approval
	if !note.HasPDF {
		noteLog.Log("APPROVE", "Cannot approve note without PDF", "noteID", note.ID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot approve note without PDF content"})
		return
	}

	previousStatus := note.Status
	wasApproved := previousStatus == models.StatusApproved

	// Update status to Approved if not already
	if !wasApproved {
		if err := app.DB.Model(note).Update("status", models.StatusApproved).Error; err != nil {
			noteLog.Log("APPROVE", "Failed to update status", "noteID", note.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve note"})
			return
		}
		note.Status = models.StatusApproved
		noteLog.Log("APPROVE", "Status updated to Approved", "noteID", note.ID)
	}

	// Index note in Meilisearch for search
	if err := app.indexNote(*note); err != nil {
		noteLog.Log("APPROVE", "Failed to index note", "noteID", note.ID, "error", err)
		// Rollback status if we just changed it
		if !wasApproved {
			if rollbackErr := app.DB.Model(note).Update("status", previousStatus).Error; rollbackErr != nil {
				noteLog.Log("APPROVE", "Failed to rollback status", "error", rollbackErr)
			}
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to index approved note"})
		return
	}
	noteLog.Log("APPROVE", "Note indexed in search", "noteID", note.ID)

	// Add to hot feed
	if err := app.AddNoteToFeed(c.Request.Context(), note); err != nil {
		noteLog.Log("APPROVE", "Failed to add to feed", "noteID", note.ID, "error", err)
	}

	noteLog.Log("APPROVE", "Note approved successfully", "noteID", note.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Note approved successfully"})
}

// GetNoteByID retrieves a note by its ID.
func (app *App) GetNoteByID(c *gin.Context) {
	noteLog.Log("GET", "Processing get note by ID request")

	note, ok := helpers.MustFetchNote(c, app.DB)
	if !ok {
		noteLog.Log("GET", "Note not found")
		return
	}

	// only return approved note
	if note.Status != models.StatusApproved {
		noteLog.Log("GET", "Note not approved", "noteID", note.ID, "status", note.Status)
		c.JSON(http.StatusForbidden, gin.H{"error": "Note is not approved"})
		return
	}

	noteLog.Log("GET", "Note retrieved", "noteID", note.ID)
	c.JSON(http.StatusOK, note)
}

// GetPendingNotes retrieves pending notes scoped to the requesting admin.
func (app *App) GetPendingNotes(c *gin.Context) {
	noteLog.Log("PENDING", "Processing get pending notes request")

	userID := helpers.GetUserID(c)
	isGlobal := helpers.GetAdminType(c)
	noteLog.Log("PENDING", "Admin identified", "userID", userID, "isGlobal", isGlobal)

	var notes []models.Note
	if isGlobal {
		// Global admin: fetch all pending notes
		noteLog.Log("PENDING", "Fetching all pending notes (global admin)")
		if err := app.DB.Where("status = ?", models.StatusPending).Find(&notes).Error; err != nil {
			noteLog.Log("PENDING", "Failed to fetch pending notes", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch pending notes"})
			return
		}
	} else {
		// Subnotery admin: fetch pending notes for their communities only
		noteLog.Log("PENDING", "Fetching pending notes for admin subnoteries", "userID", userID)
		if err := app.DB.
			Joins("JOIN user_admins ON user_admins.subnotery_id = notes.subnotery_id").
			Where("user_admins.user_id = ? AND notes.status = ?", userID, models.StatusPending).
			Find(&notes).Error; err != nil {
			noteLog.Log("PENDING", "Failed to fetch pending notes", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch pending notes"})
			return
		}
	}

	noteLog.Log("PENDING", "Pending notes retrieved", "count", len(notes))
	c.JSON(http.StatusOK, notes)
}

// GetApprovedNotes retrieves approved notes with pagination.
// Query params: page (default 1), limit (default 25, max 100).
func (app *App) GetApprovedNotes(c *gin.Context) {
	noteLog.Log("APPROVED", "Processing get approved notes request")

	pg := helpers.ParsePagination(c)

	var notes []models.Note
	var total int64

	// Count total approved notes for pagination metadata
	if err := app.DB.Model(&models.Note{}).Where("status = ?", models.StatusApproved).Count(&total).Error; err != nil {
		noteLog.Log("APPROVED", "Failed to count approved notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch approved notes"})
		return
	}

	if err := app.DB.Where("status = ?", models.StatusApproved).
		Order("created_at DESC").
		Offset(pg.Offset).Limit(pg.Limit).
		Find(&notes).Error; err != nil {
		noteLog.Log("APPROVED", "Failed to fetch approved notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch approved notes"})
		return
	}

	noteLog.Log("APPROVED", "Approved notes retrieved", "count", len(notes), "total", total, "page", pg.Page)
	c.JSON(http.StatusOK, gin.H{
		"notes": notes,
		"total": total,
		"page":  pg.Page,
		"limit": pg.Limit,
	})
}

// GetMyNotes retrieves all notes created by the authenticated user.
// Supports filtering by status via ?status= query param (pending, approved, rejected, or all).
// Returns paginated results.
//
// Route: GET /api/v1/me/notes
func (app *App) GetMyNotes(c *gin.Context) {
	userID := helpers.GetUserID(c)
	noteLog.Log("MY_NOTES", "Processing get my notes request", "userID", userID)

	pg := helpers.ParsePagination(c)
	statusFilter := c.DefaultQuery("status", "all")

	query := app.DB.Model(&models.Note{}).Where("creator_id = ?", userID)

	// Apply optional status filter
	switch statusFilter {
	case "pending":
		query = query.Where("status = ?", models.StatusPending)
	case "approved":
		query = query.Where("status = ?", models.StatusApproved)
	case "rejected":
		query = query.Where("status = ?", models.StatusRejected)
	case "all":
		// No additional filter
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error":         "Invalid status filter",
			"valid_options": []string{"all", "pending", "approved", "rejected"},
		})
		return
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		noteLog.Log("MY_NOTES", "Failed to count notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notes"})
		return
	}

	var notes []models.Note
	if err := query.Order("created_at DESC").
		Offset(pg.Offset).Limit(pg.Limit).
		Find(&notes).Error; err != nil {
		noteLog.Log("MY_NOTES", "Failed to fetch notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notes"})
		return
	}

	noteLog.Log("MY_NOTES", "Notes retrieved", "userID", userID, "count", len(notes), "total", total, "status", statusFilter)
	c.JSON(http.StatusOK, gin.H{
		"notes":  notes,
		"total":  total,
		"page":   pg.Page,
		"limit":  pg.Limit,
		"status": statusFilter,
	})
}

// indexNote adds or updates a note in the Meilisearch index.
func (app *App) indexNote(note models.Note) error {
	noteLog.Log("INDEX", "Indexing note in Meilisearch", "noteID", note.ID)
	if app.Search == nil || app.SearchIndex == "" {
		return errors.New("meilisearch is not configured")
	}
	index := app.Search.Index(app.SearchIndex)
	_, err := index.AddDocuments([]models.Note{note}, &meilisearch.DocumentOptions{
		PrimaryKey: meilisearch.StringPtr("id"),
	})
	if err != nil {
		return err
	}
	noteLog.Log("INDEX", "Note indexed successfully", "noteID", note.ID)
	return nil
}

// removeNoteFromIndex removes a note from the Meilisearch index.
func (app *App) removeNoteFromIndex(noteID uint) error {
	noteLog.Log("INDEX", "Removing note from Meilisearch", "noteID", noteID)
	if app.Search == nil || app.SearchIndex == "" {
		return errors.New("meilisearch is not configured")
	}
	index := app.Search.Index(app.SearchIndex)
	_, err := index.DeleteDocument(strconv.FormatUint(uint64(noteID), 10), nil)
	if err == nil {
		noteLog.Log("INDEX", "Note removed from index", "noteID", noteID)
	}
	return err
}
