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

	"github.com/Geetur/Notery/internal/database"
	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// noteLog is the domain-specific logger for note operations.
var noteLog = helpers.NoteLog

// NoteHandler handles HTTP requests for note operations.
type NoteHandler struct {
	DB          *gorm.DB
	Search      meilisearch.ServiceManager
	SearchIndex string
	Feed        *FeedHandler       // optional, for hot feed integration
	R2          *database.R2Client // optional, for PDF storage cleanup
}

// CreateNoteHandler returns a new NoteHandler with the given dependencies.
func CreateNoteHandler(db *gorm.DB, search meilisearch.ServiceManager, indexName string) *NoteHandler {
	return &NoteHandler{
		DB:          db,
		Search:      search,
		SearchIndex: indexName,
	}
}

// SetFeedHandler sets the FeedHandler for hot feed integration.
func (handler *NoteHandler) SetFeedHandler(feed *FeedHandler) {
	handler.Feed = feed
}

// SetR2Client sets the R2Client for PDF storage cleanup during note deletion.
func (handler *NoteHandler) SetR2Client(r2 *database.R2Client) {
	handler.R2 = r2
}

// deletePDFFromR2 removes a note's PDF from R2 storage if R2 is configured.
func (handler *NoteHandler) deletePDFFromR2(ctx context.Context, noteID uint) {
	if handler.R2 != nil {
		if err := handler.R2.DeletePDF(ctx, noteID); err != nil {
			noteLog.Log("R2", "Failed to delete PDF", "noteID", noteID, "error", err)
		} else {
			noteLog.Log("R2", "Deleted PDF", "noteID", noteID)
		}
	}
}

// addToFeed adds a note to the hot feed if FeedHandler is configured.
func (handler *NoteHandler) addToFeed(ctx context.Context, note *models.Note) {
	if handler.Feed != nil {
		if err := handler.Feed.AddNoteToFeed(ctx, note); err != nil {
			noteLog.Log("FEED", "Failed to add note to feed", "noteID", note.ID, "error", err)
		}
	}
}

// removeFromFeed removes a note from the hot feed if FeedHandler is configured.
func (handler *NoteHandler) removeFromFeed(ctx context.Context, note *models.Note) {
	if handler.Feed != nil {
		if err := handler.Feed.RemoveNoteFromFeed(ctx, note); err != nil {
			noteLog.Log("FEED", "Failed to remove note from feed", "noteID", note.ID, "error", err)
		}
	}
}

// CreateNote handles the creation of a new note.
// Auto-creates subnotery when missing and assigns the creator as its first admin.
func (handler *NoteHandler) CreateNote(c *gin.Context) {
	noteLog.Log("CREATE", "Processing note creation request")

	// Bind and validate request body
	var req struct {
		SubnoteryName string  `json:"subnotery_name" binding:"required"`
		Title         string  `json:"title"`
		Author        string  `json:"author"`
		Price         float64 `json:"price"`
	}
	if !helpers.BindJSON(c, &req) {
		noteLog.Log("CREATE", "Failed to bind JSON request")
		return
	}
	noteLog.Log("CREATE", "Request validated", "subnotery", req.SubnoteryName, "title", req.Title)

	// Extract authenticated user ID
	userID := helpers.GetUserID(c)
	noteLog.Log("CREATE", "User identified", "userID", userID)

	// Execute note creation in transaction
	var note models.Note
	if err := handler.DB.Transaction(func(tx *gorm.DB) error {
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
			Status:      "Pending",
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

// DeleteNote is a method of NoteHandler that handles the deletion of a note by ID.
// DeleteNote removes the note from both the database and Meilisearch if approved.
// DeleteNote interacts with the removeNoteFromIndex and indexNote handler methods.
func (handler *NoteHandler) DeleteNote(c *gin.Context) {
	noteLog.Log("DELETE", "Processing note deletion request")

	// Fetch the note using helper
	note, ok := helpers.MustFetchNote(c, handler.DB)
	if !ok {
		noteLog.Log("DELETE", "Note not found")
		return
	}
	noteLog.Log("DELETE", "Note fetched", "noteID", note.ID, "status", note.Status)

	// Remove from Meilisearch and feed if approved
	if note.Status == "Approved" {
		if err := handler.removeNoteFromIndex(note.ID); err != nil {
			noteLog.Log("DELETE", "Failed to remove from search index", "noteID", note.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove approved note from search index"})
			return
		}
		handler.removeFromFeed(c.Request.Context(), note)
		noteLog.Log("DELETE", "Removed from search index and feed", "noteID", note.ID)
	}

	// Delete from database
	if err := handler.DB.Delete(note).Error; err != nil {
		noteLog.Log("DELETE", "Failed to delete from database", "noteID", note.ID, "error", err)
		// Attempt to re-index if we had removed it
		if note.Status == "Approved" {
			if reindexErr := handler.indexNote(*note); reindexErr != nil {
				noteLog.Log("DELETE", "Failed to re-index after delete error", "error", reindexErr)
			}
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete note"})
		return
	}

	// Cleanup PDF from R2 if exists
	if note.HasPDF {
		handler.deletePDFFromR2(c.Request.Context(), note.ID)
	}

	noteLog.Log("DELETE", "Note deleted successfully", "noteID", note.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Note deleted successfully"})
}

// RejectNote is a method of NoteHandler that handles rejecting a note by ID.
// RejectNote interacts with the database to update the note's status to "Rejected".
// RejectNote does not interact with any handler methods.
func (handler *NoteHandler) RejectNote(c *gin.Context) {
	noteLog.Log("REJECT", "Processing note rejection request")

	// Fetch the note using helper
	note, ok := helpers.MustFetchNote(c, handler.DB)
	if !ok {
		noteLog.Log("REJECT", "Note not found")
		return
	}
	noteLog.Log("REJECT", "Note fetched", "noteID", note.ID, "status", note.Status)

	previousStatus := note.Status

	// Update status to Rejected
	if err := handler.DB.Model(note).Update("status", "Rejected").Error; err != nil {
		noteLog.Log("REJECT", "Failed to update status", "noteID", note.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject note"})
		return
	}

	// Handle edge case: if note was approved, remove from search index
	if previousStatus == "Approved" {
		noteLog.Log("REJECT", "Removing previously approved note from search", "noteID", note.ID)
		if err := handler.removeNoteFromIndex(note.ID); err != nil {
			noteLog.Log("REJECT", "Failed to remove from search index", "error", err)
			// Rollback status update
			if rollbackErr := handler.DB.Model(note).Update("status", previousStatus).Error; rollbackErr != nil {
				noteLog.Log("REJECT", "Failed to rollback status", "error", rollbackErr)
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove approved note from search index"})
			return
		}
		handler.removeFromFeed(c.Request.Context(), note)
	}

	// Delete the note from database
	if err := handler.DB.Delete(note).Error; err != nil {
		noteLog.Log("REJECT", "Failed to delete note", "noteID", note.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete note after rejection"})
		return
	}

	// Cleanup PDF from R2 if exists
	if note.HasPDF {
		handler.deletePDFFromR2(c.Request.Context(), note.ID)
	}

	noteLog.Log("REJECT", "Note rejected and deleted successfully", "noteID", note.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Note rejected successfully"})
}

// ApproveNote is a method of NoteHandler that handles approving a note by ID.
// ApproveNote updates the note's status to "Approved" and indexes it in Meilisearch.
// ApproveNote interacts with the indexNote handler method.
func (handler *NoteHandler) ApproveNote(c *gin.Context) {
	noteLog.Log("APPROVE", "Processing note approval request")

	// Fetch the note using helper
	note, ok := helpers.MustFetchNote(c, handler.DB)
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
	wasApproved := previousStatus == "Approved"

	// Update status to Approved if not already
	if !wasApproved {
		if err := handler.DB.Model(note).Update("status", "Approved").Error; err != nil {
			noteLog.Log("APPROVE", "Failed to update status", "noteID", note.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve note"})
			return
		}
		note.Status = "Approved"
		noteLog.Log("APPROVE", "Status updated to Approved", "noteID", note.ID)
	}

	// Index note in Meilisearch for search
	if err := handler.indexNote(*note); err != nil {
		noteLog.Log("APPROVE", "Failed to index note", "noteID", note.ID, "error", err)
		// Rollback status if we just changed it
		if !wasApproved {
			if rollbackErr := handler.DB.Model(note).Update("status", previousStatus).Error; rollbackErr != nil {
				noteLog.Log("APPROVE", "Failed to rollback status", "error", rollbackErr)
			}
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to index approved note"})
		return
	}
	noteLog.Log("APPROVE", "Note indexed in search", "noteID", note.ID)

	// Add to hot feed
	handler.addToFeed(c.Request.Context(), note)

	noteLog.Log("APPROVE", "Note approved successfully", "noteID", note.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Note approved successfully"})
}

// even though we will use meilisearch, we still want
// to be able to get singular notes from the database directly

// GetNoteByID is a method of NoteHandler that retrieves a note by its ID.
// GetNoteByID interacts with the database to fetch the note record.
// GetNoteByID does not interact with any handler methods.
func (handler *NoteHandler) GetNoteByID(c *gin.Context) {
	noteLog.Log("GET", "Processing get note by ID request")

	note, ok := helpers.MustFetchNote(c, handler.DB)
	if !ok {
		noteLog.Log("GET", "Note not found")
		return
	}

	noteLog.Log("GET", "Note retrieved", "noteID", note.ID)
	c.JSON(http.StatusOK, note)
}

// GetPendingNotes retrieves pending notes scoped to the requesting admin.
// GetPendingNotes interacts with the database to fetch pending notes and user scope.
// GetPendingNotes does not interact with any handler methods.
func (handler *NoteHandler) GetPendingNotes(c *gin.Context) {
	noteLog.Log("PENDING", "Processing get pending notes request")

	userID := helpers.GetUserID(c)
	isGlobal := helpers.GetAdminType(c)
	noteLog.Log("PENDING", "Admin identified", "userID", userID, "isGlobal", isGlobal)

	var notes []models.Note
	if isGlobal {
		// Global admin: fetch all pending notes
		noteLog.Log("PENDING", "Fetching all pending notes (global admin)")
		if err := handler.DB.Where("status = ?", "Pending").Find(&notes).Error; err != nil {
			noteLog.Log("PENDING", "Failed to fetch pending notes", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch pending notes"})
			return
		}
	} else {
		// Subnotery admin: fetch pending notes for their communities only
		noteLog.Log("PENDING", "Fetching pending notes for admin subnoteries", "userID", userID)
		if err := handler.DB.
			Joins("JOIN user_admins ON user_admins.subnotery_id = notes.subnotery_id").
			Where("user_admins.user_id = ? AND notes.status = ?", userID, "Pending").
			Find(&notes).Error; err != nil {
			noteLog.Log("PENDING", "Failed to fetch pending notes", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch pending notes"})
			return
		}
	}

	noteLog.Log("PENDING", "Pending notes retrieved", "count", len(notes))
	c.JSON(http.StatusOK, notes)
}

// GetApprovedNotes retrieves all notes with status "Approved"
// GetApprovedNotes interacts with the database to fetch approved notes.
// GetApprovedNotes does not interact with any handler methods.
func (handler *NoteHandler) GetApprovedNotes(c *gin.Context) {
	noteLog.Log("APPROVED", "Processing get approved notes request")

	var notes []models.Note
	if err := handler.DB.Where("status = ?", "Approved").Find(&notes).Error; err != nil {
		noteLog.Log("APPROVED", "Failed to fetch approved notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch approved notes"})
		return
	}

	noteLog.Log("APPROVED", "Approved notes retrieved", "count", len(notes))
	c.JSON(http.StatusOK, notes)
}

// indexNote adds or updates a note in the Meilisearch index
// indexNote interacts with Meilisearch to index the approved note.
// indexNote interacts with no other handler methods.
func (handler *NoteHandler) indexNote(note models.Note) error {
	noteLog.Log("INDEX", "Indexing note in Meilisearch", "noteID", note.ID)
	if handler.Search == nil || handler.SearchIndex == "" {
		return errors.New("meilisearch is not configured")
	}
	index := handler.Search.Index(handler.SearchIndex)
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
func (handler *NoteHandler) removeNoteFromIndex(noteID uint) error {
	noteLog.Log("INDEX", "Removing note from Meilisearch", "noteID", noteID)
	if handler.Search == nil || handler.SearchIndex == "" {
		return errors.New("meilisearch is not configured")
	}
	index := handler.Search.Index(handler.SearchIndex)
	_, err := index.DeleteDocument(strconv.FormatUint(uint64(noteID), 10), nil)
	if err == nil {
		noteLog.Log("INDEX", "Note removed from index", "noteID", noteID)
	}
	return err
}
