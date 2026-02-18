// note.go — HTTP handlers for note CRUD and admin approval/rejection.
//
// ENDPOINTS:
//
//	POST  /notes              Create a new note (auto-creates subnotery if needed)
//	GET   /notes/:id          Get a single approved note by ID
//	GET   /notes/approved     List all approved notes (paginated)
//	GET   /notes/pending      List pending notes for the requesting admin (paginated)
//	PATCH /notes/:id/approve  Approve a pending note (requires PDF, indexes in Meilisearch)
//	PATCH /notes/:id/reject   Reject a note (removes from search/feed, deletes from DB)
//	DELETE /notes/:id         Delete a note (removes from search/feed/R2, deletes from DB)
//
// DESIGN:
//
//	Notes follow a Pending → Approved/Rejected lifecycle. Only approved notes are
//	visible to non-admin users and indexed in Meilisearch for search. Approval
//	requires a PDF to be uploaded first (via content.go). Subnoteries are auto-created
//	during note creation: the creator becomes the first admin and member.
//
//	Admin scope is enforced by upstream middleware: global admins see all pending
//	notes; subnotery admins see only notes in their subnoteries. Pending note
//	listing is paginated with total count.
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

// deletePDFFromR2 removes a note's PDF from Cloudflare R2 storage if R2 is configured.
// Best-effort: logs failures but does not propagate errors to the caller.
//
// Technologies: Cloudflare R2 (S3 DeleteObject).
func (app *App) deletePDFFromR2(ctx context.Context, noteID uint) {
	if app.R2 != nil {
		if err := app.R2.DeletePDF(ctx, noteID); err != nil {
			noteLog.Log("R2", "Failed to delete PDF", "noteID", noteID, "error", err)
		} else {
			noteLog.Log("R2", "Deleted PDF", "noteID", noteID)
		}
	}
}

// CreateNote handles the creation of a new note with metadata.
//
// If the specified subnotery doesn't exist, it is created automatically and the
// requesting user becomes its first admin and member. The note starts in Pending
// status and must be approved by an admin before it appears in search or the feed.
//
// DB: SELECT subnotery by name, INSERT subnotery (if new), INSERT user_admins + user_memberships
//     (if new), INSERT note. All in a single GORM transaction.
// Technologies: PostgreSQL (GORM transaction).
// Helpers: helpers.BindJSON, helpers.GetUserID.
//
// Route: POST /api/v1/notes
func (app *App) CreateNote(c *gin.Context) {
	noteLog.Log("CREATE", "Processing note creation request")

	// Bind and validate request body
	var req struct {
		SubnoteryName string `json:"subnotery_name" binding:"required"`
		Title         string `json:"title"`
		Description   string `json:"description"`
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
	if req.Title == "" || req.SubnoteryName == "" || req.Price < 0 {
		noteLog.Log("CREATE", "Validation failed: missing required fields")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Title, SubnoteryName, and Price are required"})
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

		// Fetch the creator user (needed for author name, and admin assignment if new subnotery)
		var creator models.User
		if err := tx.First(&creator, userID).Error; err != nil {
			noteLog.Log("CREATE", "Failed to fetch creator", "error", err)
			return err
		}

		// Assign creator as first admin if subnotery was just created
		if subnoteryCreated {
			if err := tx.Model(&subnotery).Association("Admins").Append(&creator); err != nil {
				noteLog.Log("CREATE", "Failed to assign admin", "error", err)
				return err
			}
			noteLog.Log("CREATE", "Creator assigned as admin", "subnoteryID", subnotery.ID)
		}

		// Always auto-join creator as member (idempotent — GORM ignores duplicates)
		if err := tx.Model(&subnotery).Association("Members").Append(&creator); err != nil {
			noteLog.Log("CREATE", "Failed to add member", "error", err)
			return err
		}
		noteLog.Log("CREATE", "Creator ensured as member", "subnoteryID", subnotery.ID)

		// Create the note record (author = creator's display name)
		note = models.Note{
			Title:       req.Title,
			Description: req.Description,
			Author:      creator.DisplayName(),
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

	// Populate subnotery name for the response
	var sub models.Subnotery
	if err := app.DB.Select("id, name").First(&sub, note.SubnoteryID).Error; err == nil {
		note.SubnoteryName = sub.Name
	}

	c.JSON(http.StatusCreated, note)
}

// DeleteNote permanently removes a note and all associated resources.
//
// If the note was approved, it is first removed from the Meilisearch index and
// the Redis hot feed. Then the note record is deleted from the database. If the
// note had a PDF, it is cleaned up from Cloudflare R2. On index removal failure,
// the delete is aborted to maintain search consistency.
//
// DB: SELECT note by ID, DELETE note. Conditional: re-index on rollback.
// Technologies: PostgreSQL (GORM), Meilisearch (document delete), Redis ZREM (feed removal),
//     Cloudflare R2 (PDF cleanup).
// Helpers: helpers.MustFetchNote.
//
// Route: DELETE /api/v1/notes/:id
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

	// Cleanup thumbnail from R2 if exists
	if note.HasThumbnail && note.ThumbnailURL != "" {
		app.deleteThumbnailFromR2(c.Request.Context(), note.ThumbnailURL)
	}

	noteLog.Log("DELETE", "Note deleted successfully", "noteID", note.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Note deleted successfully"})
}

// RejectNote rejects a note, removing it from search/feed and deleting it from the DB.
//
// If the note was previously approved, it is removed from the Meilisearch index
// and the Redis hot feed first. The note is then deleted from the database.
// On index removal failure, the status change is rolled back.
//
// DB: SELECT note by ID, UPDATE status to Rejected, DELETE note. Conditional rollback on failure.
// Technologies: PostgreSQL (GORM), Meilisearch (document delete), Redis ZREM (feed removal),
//     Cloudflare R2 (PDF cleanup if exists).
// Helpers: helpers.MustFetchNote.
//
// Route: PATCH /api/v1/notes/:id/reject
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

	// Cleanup thumbnail from R2 if exists
	if note.HasThumbnail && note.ThumbnailURL != "" {
		app.deleteThumbnailFromR2(c.Request.Context(), note.ThumbnailURL)
	}

	noteLog.Log("REJECT", "Note rejected and deleted successfully", "noteID", note.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Note rejected successfully"})
}

// ApproveNote transitions a note from Pending to Approved status.
//
// Validates that the note has a PDF before approval. Updates the status,
// indexes the note in Meilisearch for full-text search, and adds it to the
// Redis hot feed. On indexing failure, the status change is rolled back.
//
// DB: SELECT note by ID, UPDATE status to Approved. Conditional rollback on index failure.
// Technologies: PostgreSQL (GORM), Meilisearch (document add/update), Redis ZADD (feed).
// Helpers: helpers.MustFetchNote.
//
// Route: PATCH /api/v1/notes/:id/approve
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

// GetNoteByID retrieves a single note by its ID.
//
// Approved notes are visible to all authenticated users. Non-approved notes
// (Pending/Rejected) are only visible to admins (global or scoped to the
// note's subnotery). Non-admin users receive 403 Forbidden.
//
// DB: SELECT note by ID via GORM; optional admin check via user + user_admins.
// Technologies: PostgreSQL (GORM).
// Helpers: helpers.MustFetchNote, helpers.GetUserID.
//
// Route: GET /api/v1/notes/:id
func (app *App) GetNoteByID(c *gin.Context) {
	noteLog.Log("GET", "Processing get note by ID request")

	note, ok := helpers.MustFetchNote(c, app.DB)
	if !ok {
		noteLog.Log("GET", "Note not found")
		return
	}

	// Approved notes are visible to all authenticated users.
	// Non-approved notes are only visible to admins (global or scoped to the note's subnotery).
	if note.Status != models.StatusApproved {
		userID := helpers.GetUserID(c)

		// Check global admin
		var user models.User
		if err := app.DB.Select("id", "is_global_admin").First(&user, userID).Error; err != nil {
			noteLog.Log("GET", "Note not approved, user lookup failed", "noteID", note.ID)
			c.JSON(http.StatusForbidden, gin.H{"error": "Note is not approved"})
			return
		}

		if !user.IsGlobalAdmin {
			// Check subnotery admin
			var adminCount int64
			app.DB.Table("user_admins").
				Where("user_id = ? AND subnotery_id = ?", userID, note.SubnoteryID).
				Count(&adminCount)
			if adminCount == 0 {
				noteLog.Log("GET", "Note not approved, user not admin", "noteID", note.ID, "status", note.Status)
				c.JSON(http.StatusForbidden, gin.H{"error": "Note is not approved"})
				return
			}
		}

		noteLog.Log("GET", "Admin viewing non-approved note", "noteID", note.ID, "status", note.Status, "userID", userID)
	}

	noteLog.Log("GET", "Note retrieved", "noteID", note.ID)

	// Populate subnotery name
	var sub models.Subnotery
	if err := app.DB.Select("id, name").First(&sub, note.SubnoteryID).Error; err == nil {
		note.SubnoteryName = sub.Name
	}

	c.JSON(http.StatusOK, note)
}

// GetPendingNotes retrieves pending notes visible to the requesting admin.
//
// Global admins see all pending notes; subnotery admins see only notes in their
// administered subnoteries (via JOIN on user_admins). Paginated response includes
// total count for frontend pagination.
//
// DB: COUNT + SELECT from notes with optional JOIN on user_admins. Paginated with OFFSET/LIMIT.
// Technologies: PostgreSQL (GORM, conditional JOIN for scoping).
// Helpers: helpers.GetUserID, helpers.GetAdminType, helpers.ParsePagination.
//
// Route: GET /api/v1/notes/pending
func (app *App) GetPendingNotes(c *gin.Context) {
	noteLog.Log("PENDING", "Processing get pending notes request")

	userID := helpers.GetUserID(c)
	isGlobal := helpers.GetAdminType(c)
	pag := helpers.ParsePagination(c)
	noteLog.Log("PENDING", "Admin identified", "userID", userID, "isGlobal", isGlobal)

	var notes []models.Note
	var total int64

	query := app.DB.Model(&models.Note{})
	if isGlobal {
		query = query.Where("status = ?", models.StatusPending)
	} else {
		query = query.
			Joins("JOIN user_admins ON user_admins.subnotery_id = notes.subnotery_id").
			Where("user_admins.user_id = ? AND notes.status = ?", userID, models.StatusPending)
	}

	// Optional subnotery filter — used by community detail pages.
	if subID := c.Query("subnotery_id"); subID != "" {
		query = query.Where("notes.subnotery_id = ?", subID)
	}

	if err := query.Count(&total).Error; err != nil {
		noteLog.Log("PENDING", "Failed to count pending notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch pending notes"})
		return
	}

	if err := query.Offset(pag.Offset).Limit(pag.Limit).Order("created_at DESC").Find(&notes).Error; err != nil {
		noteLog.Log("PENDING", "Failed to fetch pending notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch pending notes"})
		return
	}

	noteLog.Log("PENDING", "Pending notes retrieved", "count", len(notes), "total", total)

	// Populate subnotery names
	app.populateSubnoteryNames(notes)

	c.JSON(http.StatusOK, gin.H{
		"notes": notes,
		"total": total,
		"page":  pag.Page,
		"limit": pag.Limit,
	})
}

// GetApprovedNotes returns a paginated list of all approved notes.
//
// Public endpoint (requires auth but no admin). Returns notes ordered by
// creation time descending with total count for frontend pagination.
//
// DB: COUNT + SELECT from notes WHERE status = Approved. Paginated with OFFSET/LIMIT.
// Technologies: PostgreSQL (GORM).
// Helpers: helpers.ParsePagination.
//
// Route: GET /api/v1/notes/approved
func (app *App) GetApprovedNotes(c *gin.Context) {
	noteLog.Log("APPROVED", "Processing get approved notes request")
	pag := helpers.ParsePagination(c)

	var notes []models.Note
	var total int64

	query := app.DB.Model(&models.Note{}).Where("status = ?", models.StatusApproved)

	if err := query.Count(&total).Error; err != nil {
		noteLog.Log("APPROVED", "Failed to count approved notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch approved notes"})
		return
	}

	if err := query.Offset(pag.Offset).Limit(pag.Limit).Order("created_at DESC").Find(&notes).Error; err != nil {
		noteLog.Log("APPROVED", "Failed to fetch approved notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch approved notes"})
		return
	}

	noteLog.Log("APPROVED", "Approved notes retrieved", "count", len(notes), "total", total)

	// Populate subnotery names
	app.populateSubnoteryNames(notes)

	c.JSON(http.StatusOK, gin.H{
		"notes": notes,
		"total": total,
		"page":  pag.Page,
		"limit": pag.Limit,
	})
}

// GetMyNotes returns a paginated list of notes created by the authenticated user.
//
// Supports optional status filtering via ?status= query parameter (Pending, Approved, Rejected).
// If no status filter is provided, returns notes of all statuses. Ordered by creation
// time descending so the newest notes appear first.
//
// DB: COUNT + SELECT from notes WHERE creator_id = userID [AND status = ...]. Paginated.
// Technologies: PostgreSQL (GORM).
// Helpers: helpers.GetUserID, helpers.ParsePagination.
//
// Route: GET /api/v1/me/notes
func (app *App) GetMyNotes(c *gin.Context) {
	noteLog.Log("MY_NOTES", "Processing get my notes request")

	userID := helpers.GetUserID(c)
	pag := helpers.ParsePagination(c)

	var notes []models.Note
	var total int64

	query := app.DB.Model(&models.Note{}).Where("creator_id = ?", userID)

	// Optional status filter
	if statusFilter := c.Query("status"); statusFilter != "" {
		status := models.NoteStatus(statusFilter)
		if status != models.StatusPending && status != models.StatusApproved && status != models.StatusRejected {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status filter. Must be Pending, Approved, or Rejected"})
			return
		}
		query = query.Where("status = ?", status)
		noteLog.Log("MY_NOTES", "Filtering by status", "status", statusFilter)
	}

	if err := query.Count(&total).Error; err != nil {
		noteLog.Log("MY_NOTES", "Failed to count notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch your notes"})
		return
	}

	if err := query.Offset(pag.Offset).Limit(pag.Limit).Order("created_at DESC").Find(&notes).Error; err != nil {
		noteLog.Log("MY_NOTES", "Failed to fetch notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch your notes"})
		return
	}

	noteLog.Log("MY_NOTES", "Notes retrieved", "count", len(notes), "total", total, "userID", userID)

	// Populate subnotery names
	app.populateSubnoteryNames(notes)

	c.JSON(http.StatusOK, gin.H{
		"notes": notes,
		"total": total,
		"page":  pag.Page,
		"limit": pag.Limit,
	})
}

// indexNote adds or updates a note in the Meilisearch full-text search index.
// Called during note approval to make the note discoverable via search.
//
// Technologies: Meilisearch (AddDocuments with primary key "id").
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

// removeNoteFromIndex removes a note from the Meilisearch full-text search index.
// Called during note deletion or rejection to keep search results consistent.
//
// Technologies: Meilisearch (DeleteDocument by ID).
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

// GetUserNotes returns a paginated list of approved notes created by a specific user.
//
// Public endpoint. Only returns approved notes so unapproved content is not leaked.
// Paginated with total count for frontend display on user profile pages.
//
// DB: COUNT + SELECT from notes WHERE creator_id AND status = Approved. Paginated.
// Technologies: PostgreSQL (GORM).
// Helpers: helpers.ParsePagination.
//
// Route: GET /api/v1/users/:id/notes
func (app *App) GetUserNotes(c *gin.Context) {
	noteLog.Log("USER_NOTES", "Processing get user notes request")

	userID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	pag := helpers.ParsePagination(c)

	var notes []models.Note
	var total int64

	query := app.DB.Model(&models.Note{}).
		Where("creator_id = ? AND status = ?", userID, models.StatusApproved)

	if err := query.Count(&total).Error; err != nil {
		noteLog.Log("USER_NOTES", "Failed to count notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user notes"})
		return
	}

	if err := query.Offset(pag.Offset).Limit(pag.Limit).Order("created_at DESC").Find(&notes).Error; err != nil {
		noteLog.Log("USER_NOTES", "Failed to fetch notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user notes"})
		return
	}

	// Populate subnotery names
	app.populateSubnoteryNames(notes)

	noteLog.Log("USER_NOTES", "User notes retrieved", "userID", userID, "count", len(notes), "total", total)
	c.JSON(http.StatusOK, gin.H{
		"notes": notes,
		"total": total,
		"page":  pag.Page,
		"limit": pag.Limit,
	})
}
