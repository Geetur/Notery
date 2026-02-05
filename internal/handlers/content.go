// Package handlers/content.go contains HTTP handlers for PDF content operations.
// This file manages all PDF-related operations: upload, viewing, and access control.
//
// SECURITY ARCHITECTURE:
// ----------------------
// PDFs are stored in Cloudflare R2 (private bucket). Users NEVER get direct R2 URLs.
// Instead, all PDF access goes through these authenticated endpoints which:
// 1. Verify the user has permission to view the PDF
// 2. Proxy the PDF content from R2 to the user
// 3. Set headers to ensure in-browser viewing, not downloading
//
// ACCESS CONTROL MATRIX:
// ----------------------
// | User Type            | Pending Notes     | Approved Notes      |
// |----------------------|-------------------|---------------------|
// | Anonymous            | No                | No                  |
// | Authenticated        | No                | Only if purchased   |
// | Note Creator         | Yes (own notes)   | Yes (own notes)     |
// | Subnotery Admin      | Yes (their sub)   | Yes (their sub)     |
// | Global Admin         | Yes (all)         | Yes (all)           |
//
// FRONTEND INTEGRATION:
// ---------------------
// The frontend should use a PDF viewer library (e.g., PDF.js, react-pdf) that:
// 1. Fetches the PDF from our proxy endpoint (GET /api/v1/notes/:id/content)
// 2. Renders it in a canvas/iframe - no download option exposed
// 3. The API returns the PDF with headers that ensure in-browser viewing, not downloading
//
// WHY PROXY INSTEAD OF PRESIGNED URLs:
// ------------------------------------
// We COULD give users presigned URLs that expire quickly, but:
// - Users could still share those URLs while valid
// - Users could save/download the PDF from the URL
// - No control over how the content is displayed
// By proxying, we maintain complete control over the viewing experience.
package handlers

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/database"
	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// contentLog is the shared logger for content operations
var contentLog = helpers.ContentLog

// ContentHandler handles PDF content operations.
// It manages uploads, downloads (proxied viewing), and access verification.
type ContentHandler struct {
	DB *gorm.DB
	R2 *database.R2Client
}

// CreateContentHandler returns a new ContentHandler with the given dependencies.
// CreateContentHandler interacts with no other handler methods.
// CreateContentHandler interacts with the database and R2 client.
func CreateContentHandler(db *gorm.DB, r2 *database.R2Client) *ContentHandler {
	return &ContentHandler{
		DB: db,
		R2: r2,
	}
}

// ----- ACCESS VERIFICATION HELPERS -----
// These helpers centralize access control logic for reuse across handlers.

// AccessLevel represents what level of access a user has to a note
type AccessLevel int

const (
	AccessNone        AccessLevel = iota // No access
	AccessPurchased                      // User purchased this note
	AccessCreator                        // User created this note
	AccessSubAdmin                       // User is admin of the note's subnotery
	AccessGlobalAdmin                    // User is a global admin
)

// CheckNoteAccess determines what access level a user has for a specific note.
// CheckNoteAccess interacts with the database to verify permissions.
// CheckNoteAccess does not interact with any other handler methods.
//
// Parameters:
// - userID: The authenticated user's ID
// - note: The note being accessed
//
// Returns the highest access level the user has.
func (handler *ContentHandler) CheckNoteAccess(userID uint64, note *models.Note) AccessLevel {
	contentLog.Log("ACCESS_CHECK", "checking permissions", "user_id", userID, "note_id", note.ID, "note_status", note.Status)

	// Check if user is the creator of this note (always has access)
	if note.CreatorID == userID {
		contentLog.Log("ACCESS_GRANTED", "user is creator", "user_id", userID, "note_id", note.ID)
		return AccessCreator
	}

	// Check if user is global admin
	var user models.User
	if err := handler.DB.Select("id", "is_global_admin").First(&user, userID).Error; err != nil {
		contentLog.Log("ACCESS_ERROR", "failed to fetch user", "user_id", userID, "error", err)
		return AccessNone
	}

	if user.IsGlobalAdmin {
		contentLog.Log("ACCESS_GRANTED", "global admin", "user_id", userID, "note_id", note.ID)
		return AccessGlobalAdmin
	}

	// Check if user is admin of this note's subnotery
	var adminCount int64
	handler.DB.Table("user_admins").
		Where("user_id = ? AND subnotery_id = ?", userID, note.SubnoteryID).
		Count(&adminCount)

	if adminCount > 0 {
		contentLog.Log("ACCESS_GRANTED", "subnotery admin", "user_id", userID, "note_id", note.ID, "subnotery_id", note.SubnoteryID)
		return AccessSubAdmin
	}

	// For approved notes, check if user purchased it
	if note.Status == "Approved" {
		var purchaseCount int64
		handler.DB.Model(&models.Purchase{}).
			Where("user_id = ? AND note_id = ?", userID, note.ID).
			Count(&purchaseCount)

		if purchaseCount > 0 {
			contentLog.Log("ACCESS_GRANTED", "purchased", "user_id", userID, "note_id", note.ID)
			return AccessPurchased
		}
	}

	contentLog.Log("ACCESS_DENIED", "no permission", "user_id", userID, "note_id", note.ID)
	return AccessNone
}

// CanViewPendingNote checks if a user can view a pending note's PDF.
// Only creators and admins (subnotery or global) can view pending notes.
// CanViewPendingNote interacts with CheckNoteAccess.
// CanViewPendingNote does not interact with any other handler methods.
func (handler *ContentHandler) CanViewPendingNote(userID uint64, note *models.Note) bool {
	access := handler.CheckNoteAccess(userID, note)
	return access == AccessCreator || access == AccessSubAdmin || access == AccessGlobalAdmin
}

// CanViewApprovedNote checks if a user can view an approved note's PDF.
// Requires purchase OR admin access.
// CanViewApprovedNote interacts with CheckNoteAccess.
// CanViewApprovedNote does not interact with any other handler methods.
func (handler *ContentHandler) CanViewApprovedNote(userID uint64, note *models.Note) bool {
	access := handler.CheckNoteAccess(userID, note)
	return access != AccessNone
}

// ----- HTTP HANDLERS -----

// UploadNotePDF handles PDF upload for a note.
// UploadNotePDF interacts with R2 to store the PDF.
// UploadNotePDF interacts with the database to update the note's PDF metadata.
//
// This endpoint is called after note creation to upload the PDF content.
// Only the note's status and existence are checked - any authenticated user
// can upload to their own pending note.
//
// Request: multipart/form-data with "pdf" field containing the PDF file
// Response: JSON with success message or error
//
// Route: POST /api/v1/notes/:id/content
func (handler *ContentHandler) UploadNotePDF(c *gin.Context) {
	start := time.Now()
	contentLog.Log("UPLOAD", "request received")

	// Parse note ID and get user ID using helpers
	noteID, ok := helpers.MustParseNoteID(c)
	if !ok {
		return
	}
	userID := helpers.GetUserID(c)
	contentLog.Log("UPLOAD", "processing", "user_id", userID, "note_id", noteID)

	// Fetch the note
	var note models.Note
	if err := handler.DB.First(&note, noteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			contentLog.Log("UPLOAD", "note not found", "note_id", noteID)
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
			return
		}
		contentLog.Log("UPLOAD", "database error fetching note", "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch note"})
		return
	}

	// Only allow upload for pending notes (can't change approved note content)
	// This prevents content bait-and-switch after approval
	if note.Status != "Pending" {
		contentLog.Log("UPLOAD", "rejected - note not pending", "note_id", noteID, "status", note.Status)
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot modify PDF for approved or rejected notes"})
		return
	}

	// For now, we allow any authenticated user to upload to pending notes.
	// In production, we might want to verify the uploader is the note creator.
	// This could be done by adding a CreatorID field to the Note model.

	// Get the uploaded file
	file, header, err := c.Request.FormFile("pdf")
	if err != nil {
		contentLog.Log("UPLOAD", "no file provided", "note_id", noteID, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "PDF file required"})
		return
	}
	defer file.Close()

	// Validate file type (basic check - production should be more thorough)
	contentType := header.Header.Get("Content-Type")
	if contentType != "application/pdf" && contentType != "" {
		contentLog.Log("UPLOAD", "invalid content type", "note_id", noteID, "content_type", contentType)
		c.JSON(http.StatusBadRequest, gin.H{"error": "File must be a PDF"})
		return
	}

	// Validate file size (e.g., max 50MB)
	maxSize := int64(50 * 1024 * 1024) // 50MB
	if header.Size > maxSize {
		contentLog.Log("UPLOAD", "file too large", "note_id", noteID, "size_bytes", header.Size, "max_bytes", maxSize)
		c.JSON(http.StatusBadRequest, gin.H{"error": "PDF file too large (max 50MB)"})
		return
	}

	contentLog.Log("UPLOAD", "uploading to R2", "note_id", noteID, "size_bytes", header.Size, "filename", header.Filename)

	// Upload to R2
	ctx := c.Request.Context()
	if err := handler.R2.UploadPDF(ctx, uint(noteID), file, header.Size); err != nil {
		contentLog.Log("UPLOAD", "R2 upload failed", "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload PDF"})
		return
	}

	contentLog.Log("UPLOAD", "R2 upload successful", "note_id", noteID)

	// Update note metadata

	if err := handler.DB.Model(&note).Updates(map[string]interface{}{
		"has_pdf":         true,
		"pdf_size":        header.Size,
		"pdf_uploaded_at": time.Now(),
	}).Error; err != nil {
		contentLog.Log("UPLOAD", "metadata update failed", "note_id", noteID, "error", err)
		// PDF was uploaded but metadata failed - not ideal but not fatal
		c.JSON(http.StatusInternalServerError, gin.H{"error": "PDF uploaded but metadata update failed"})
		return
	}

	duration := time.Since(start)
	contentLog.Log("UPLOAD", "completed successfully", "note_id", noteID, "size_bytes", header.Size, "duration_ms", duration.Milliseconds())
	c.JSON(http.StatusOK, gin.H{
		"message":  "PDF uploaded successfully",
		"pdf_size": header.Size,
	})
}

// GetNotePDFContent serves the PDF content for viewing.
// GetNotePDFContent interacts with R2 to fetch PDF content.
// GetNotePDFContent interacts with CheckNoteAccess for authorization.
//
// This is the core endpoint for the in-app PDF viewer.
// It proxies the PDF from R2 with headers that encourage viewing over downloading.
//
// Response: application/pdf stream with inline disposition
//
// Route: GET /api/v1/notes/:id/content
func (handler *ContentHandler) GetNotePDFContent(c *gin.Context) {
	start := time.Now()
	contentLog.Log("VIEW", "request received")

	// Parse note ID and get user ID using helpers
	noteID, ok := helpers.MustParseNoteID(c)
	if !ok {
		return
	}
	userID := helpers.GetUserID(c)
	contentLog.Log("VIEW", "processing", "user_id", userID, "note_id", noteID)

	// Fetch the note
	var note models.Note
	if err := handler.DB.First(&note, noteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			contentLog.Log("VIEW", "note not found", "note_id", noteID)
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
			return
		}
		contentLog.Log("VIEW", "database error", "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch note"})
		return
	}

	// Check if note has a PDF
	if !note.HasPDF {
		contentLog.Log("VIEW", "no PDF content", "note_id", noteID)
		c.JSON(http.StatusNotFound, gin.H{"error": "No PDF content available for this note"})
		return
	}

	// ----- ACCESS CONTROL -----
	// This is where we enforce who can view what
	var hasAccess bool

	switch note.Status {
	case "Pending":
		// Only admins can view pending notes
		hasAccess = handler.CanViewPendingNote(userID, &note)
		if !hasAccess {
			contentLog.Log("VIEW", "denied - pending note", "user_id", userID, "note_id", noteID)
			c.JSON(http.StatusForbidden, gin.H{"error": "This note is pending approval"})
			return
		}

	case "Approved":
		// Must have purchased or be admin
		hasAccess = handler.CanViewApprovedNote(userID, &note)
		if !hasAccess {
			contentLog.Log("VIEW", "denied - not purchased", "user_id", userID, "note_id", noteID)
			c.JSON(http.StatusForbidden, gin.H{"error": "You must purchase this note to view it"})
			return
		}

	case "Rejected":
		// Rejected notes cannot be viewed
		contentLog.Log("VIEW", "denied - note rejected", "user_id", userID, "note_id", noteID)
		c.JSON(http.StatusGone, gin.H{"error": "This note has been rejected and is no longer available"})
		return

	default:
		contentLog.Log("VIEW", "unknown status", "note_id", noteID, "status", note.Status)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unknown note status"})
		return
	}

	// Fetch PDF from R2
	contentLog.Log("VIEW", "fetching from R2", "note_id", noteID)
	ctx := c.Request.Context()
	pdfContent, contentLength, err := handler.R2.GetPDFContent(ctx, uint(noteID))
	if err != nil {
		contentLog.Log("VIEW", "R2 fetch failed", "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve PDF content"})
		return
	}
	defer pdfContent.Close()

	// Set headers for inline viewing (not download)
	// Content-Disposition: inline tells the browser to display in-browser if possible
	// We don't include a filename to further discourage saving
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", "inline")
	c.Header("Content-Length", strconv.FormatInt(contentLength, 10))

	// Security headers to prevent caching and embedding elsewhere
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "SAMEORIGIN") // Only allow embedding on same origin

	// Stream the PDF content to the response
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, pdfContent); err != nil {
		contentLog.Log("VIEW", "stream failed", "note_id", noteID, "user_id", userID, "error", err)
		// Can't return JSON error at this point, response already started
		return
	}

	duration := time.Since(start)
	contentLog.Log("VIEW", "served successfully", "note_id", noteID, "user_id", userID, "size_bytes", contentLength, "duration_ms", duration.Milliseconds())
}

// AdminPreviewPDF is an alias for GetNotePDFContent that's clearer in admin routes.
// AdminPreviewPDF interacts with GetNotePDFContent (same handler).
// AdminPreviewPDF does not interact with any other handler methods.
//
// The access control in GetNotePDFContent already handles admin verification,
// so this just provides a semantically clearer endpoint for the admin workflow.
//
// Route: GET /api/v1/admin/notes/:id/preview
func (handler *ContentHandler) AdminPreviewPDF(c *gin.Context) {
	// The actual access control is in GetNotePDFContent
	// This endpoint just provides clearer semantics for admin UIs
	handler.GetNotePDFContent(c)
}

// GetMyPurchases returns all notes purchased by the authenticated user.
// GetMyPurchases interacts with the database to fetch purchases.
// GetMyPurchases does not interact with any other handler methods.
//
// This is used to show the "My Purchased Notes" section in the user's account.
//
// Response: JSON array of notes with purchase info
//
// Route: GET /api/v1/me/purchases
func (handler *ContentHandler) GetMyPurchases(c *gin.Context) {
	userID := helpers.GetUserID(c)
	contentLog.Log("MY_PURCHASES", "fetching", "user_id", userID)

	// Fetch purchases with note details
	type PurchasedNote struct {
		models.Note
		PricePaid   float64   `json:"price_paid"`
		PurchasedAt time.Time `json:"purchased_at"`
	}

	var purchasedNotes []PurchasedNote

	// Join purchases with notes to get full note info
	err := handler.DB.Table("purchases").
		Select("notes.*, purchases.price_paid, purchases.purchased_at").
		Joins("JOIN notes ON notes.id = purchases.note_id").
		Where("purchases.user_id = ?", userID).
		Order("purchases.purchased_at DESC").
		Scan(&purchasedNotes).Error

	if err != nil {
		contentLog.Log("MY_PURCHASES", "database error", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch purchases"})
		return
	}

	contentLog.Log("MY_PURCHASES", "success", "user_id", userID, "count", len(purchasedNotes))
	c.JSON(http.StatusOK, gin.H{"purchases": purchasedNotes})
}

// DeleteNotePDF removes a note's PDF from R2 storage.
// DeleteNotePDF interacts with R2 to delete the PDF.
// DeleteNotePDF interacts with the database to update note metadata.
//
// This should be called when a note is rejected or deleted.
// It's also exposed as an admin endpoint for manual cleanup.
//
// Route: DELETE /api/v1/admin/notes/:id/content (admin only)
func (handler *ContentHandler) DeleteNotePDF(c *gin.Context) {
	noteID, ok := helpers.MustParseNoteID(c)
	if !ok {
		return
	}

	contentLog.Log("DELETE", "processing", "note_id", noteID)

	// Verify note exists
	var note models.Note
	if err := handler.DB.First(&note, noteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			contentLog.Log("DELETE", "note not found", "note_id", noteID)
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
			return
		}
		contentLog.Log("DELETE", "database error", "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch note"})
		return
	}

	// Delete from R2
	ctx := c.Request.Context()
	if err := handler.R2.DeletePDF(ctx, uint(noteID)); err != nil {
		contentLog.Log("DELETE", "R2 delete failed", "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete PDF"})
		return
	}

	// Update note metadata
	if err := handler.DB.Model(&note).Updates(map[string]interface{}{
		"has_pdf":         false,
		"pdf_size":        0,
		"pdf_uploaded_at": nil,
	}).Error; err != nil {
		contentLog.Log("DELETE", "metadata update failed (non-fatal)", "note_id", noteID, "error", err)
		// PDF deleted but metadata update failed - log but continue
	}

	contentLog.Log("DELETE", "completed successfully", "note_id", noteID)
	c.JSON(http.StatusOK, gin.H{"message": "PDF deleted successfully"})
}
