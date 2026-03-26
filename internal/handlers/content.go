// content.go — HTTP handlers for PDF content operations (upload, view, access control).
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
// | Note Creator         | No                | Yes (own notes)     |
// | Subnotery Admin      | Yes (their sub)   | Yes (their sub)     |
// | Global Admin         | Yes (all)         | Yes (all)           |
//
// PREVIEW SECURITY:
// -----------------
// The preview endpoint uses server-side page extraction via pdfcpu to serve
// only the requested number of pages. The full PDF is never sent to clients
// who don't have purchase/creator/admin access. Extracted previews are cached
// in R2 to avoid repeated CPU work.
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
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// contentLog is the shared logger for content operations
var contentLog = helpers.ContentLog

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
//
// Parameters:
// - userID: The authenticated user's ID
// - note: The note being accessed
//
// Returns the highest access level the user has.
func (app *App) CheckNoteAccess(userID uint64, note *models.Note) AccessLevel {
	contentLog.Log("ACCESS_CHECK", "checking permissions", "user_id", userID, "note_id", note.ID, "note_status", note.Status)

	// Check if user is the creator of this note (always has access)
	if note.CreatorID == userID {
		contentLog.Log("ACCESS_GRANTED", "user is creator", "user_id", userID, "note_id", note.ID)
		return AccessCreator
	}

	// Check if user is global admin
	var user models.User
	if err := app.DB.Select("id", "is_global_admin").First(&user, userID).Error; err != nil {
		contentLog.Log("ACCESS_ERROR", "failed to fetch user", "user_id", userID, "error", err)
		return AccessNone
	}

	if user.IsGlobalAdmin {
		contentLog.Log("ACCESS_GRANTED", "global admin", "user_id", userID, "note_id", note.ID)
		return AccessGlobalAdmin
	}

	// Check if user is admin of this note's subnotery
	var adminCount int64
	app.DB.Table("user_admins").
		Where("user_id = ? AND subnotery_id = ?", userID, note.SubnoteryID).
		Count(&adminCount)

	if adminCount > 0 {
		contentLog.Log("ACCESS_GRANTED", "subnotery admin", "user_id", userID, "note_id", note.ID, "subnotery_id", note.SubnoteryID)
		return AccessSubAdmin
	}

	// For approved notes, check if the note is free or user purchased it
	if note.Status == models.StatusApproved {
		// Free notes are accessible to any authenticated user
		if note.Price == 0 {
			contentLog.Log("ACCESS_GRANTED", "free note", "user_id", userID, "note_id", note.ID)
			return AccessPurchased
		}

		var purchaseCount int64
		app.DB.Model(&models.Purchase{}).
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
func (app *App) CanViewPendingNote(userID uint64, note *models.Note) bool {
	access := app.CheckNoteAccess(userID, note)
	return access == AccessCreator || access == AccessSubAdmin || access == AccessGlobalAdmin
}

// CanViewApprovedNote checks if a user can view an approved note's PDF.
// Requires purchase OR admin access.
func (app *App) CanViewApprovedNote(userID uint64, note *models.Note) bool {
	access := app.CheckNoteAccess(userID, note)
	return access != AccessNone
}

// ----- PDF PAGE EXTRACTION -----

// maxPreviewPDFSize is the maximum PDF size (in bytes) that the preview
// extraction will process. Larger PDFs are rejected to avoid excessive memory use.
const maxPreviewPDFSize = 20 * 1024 * 1024 // 20 MB

// extractPreviewPages reads a full PDF from r, extracts pages 1..pages, and
// returns the resulting valid PDF as bytes along with the total page count.
//
// Returns an error if:
// - The PDF exceeds maxPreviewPDFSize
// - The PDF cannot be parsed
// - The requested page count is >= total pages (would serve entire PDF)
func extractPreviewPages(r io.Reader, pages int) (extracted []byte, totalPages int, err error) {
	// Buffer everything so pdfcpu can seek. Guard against oversized files.
	buf, err := io.ReadAll(io.LimitReader(r, maxPreviewPDFSize+1))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read PDF: %w", err)
	}
	if len(buf) > maxPreviewPDFSize {
		return nil, 0, fmt.Errorf("PDF exceeds %d MB size limit for preview", maxPreviewPDFSize/(1024*1024))
	}

	// Discover the total page count using the dedicated PageCount API.
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	totalPages, err = api.PageCount(bytes.NewReader(buf), conf)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse PDF: %w", err)
	}

	if pages >= totalPages {
		return nil, totalPages, fmt.Errorf("requested %d pages but PDF only has %d", pages, totalPages)
	}

	// Build page selection string "1-N" for pdfcpu Trim.
	pageSelection := fmt.Sprintf("1-%d", pages)

	in := bytes.NewReader(buf)
	var out bytes.Buffer
	if err := api.Trim(in, &out, []string{pageSelection}, conf); err != nil {
		return nil, totalPages, fmt.Errorf("failed to extract pages: %w", err)
	}

	return out.Bytes(), totalPages, nil
}

// extractPageCount returns the total page count of a PDF without extracting pages.
func extractPageCount(r io.Reader) (int, error) {
	buf, err := io.ReadAll(io.LimitReader(r, maxPreviewPDFSize+1))
	if err != nil {
		return 0, fmt.Errorf("failed to read PDF: %w", err)
	}
	if len(buf) > maxPreviewPDFSize {
		return 0, fmt.Errorf("PDF exceeds size limit")
	}

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	return api.PageCount(bytes.NewReader(buf), conf)
}

// ----- HTTP HANDLERS -----

// UploadNotePDF handles PDF upload for a note.
//
// This endpoint is called after note creation to upload the PDF content.
// Only the note's status and existence are checked - any authenticated user
// can upload to their own pending note.
//
// Request: multipart/form-data with "pdf" field containing the PDF file
// Response: JSON with success message or error
//
// Route: POST /api/v1/notes/:id/content
func (app *App) UploadNotePDF(c *gin.Context) {
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
	if err := app.DB.First(&note, noteID).Error; err != nil {
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
	if note.Status != models.StatusPending {
		contentLog.Log("UPLOAD", "rejected - note not pending", "note_id", noteID, "status", note.Status)
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot modify PDF for approved or rejected notes"})
		return
	}

	// Only the note creator or an admin may upload content.
	// This prevents malicious users from replacing someone else's pending PDF.
	access := app.CheckNoteAccess(userID, &note)
	if access != AccessCreator && access != AccessSubAdmin && access != AccessGlobalAdmin {
		contentLog.Log("UPLOAD", "denied - not creator or admin", "user_id", userID, "note_id", noteID, "creator_id", note.CreatorID)
		c.JSON(http.StatusForbidden, gin.H{"error": "Only the note creator or an admin can upload content"})
		return
	}

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
	if err := app.R2.UploadPDF(ctx, uint(noteID), file, header.Size); err != nil {
		contentLog.Log("UPLOAD", "R2 upload failed", "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload PDF"})
		return
	}

	contentLog.Log("UPLOAD", "R2 upload successful", "note_id", noteID)

	// Invalidate any cached preview PDFs (best-effort).
	go func() {
		if err := app.R2.DeletePreviewPDFs(c.Request.Context(), uint(noteID)); err != nil {
			contentLog.Log("UPLOAD", "preview cache invalidation failed (non-fatal)", "note_id", noteID, "error", err)
		}
	}()

	// Extract page count from the uploaded PDF for preview calculations.
	// Re-open the file to read it again (FormFile was already consumed by UploadPDF).
	var pdfPages int
	if reopened, _, reopenErr := c.Request.FormFile("pdf"); reopenErr == nil {
		defer reopened.Close()
		if count, countErr := extractPageCount(reopened); countErr == nil {
			pdfPages = count
		} else {
			contentLog.Log("UPLOAD", "page count extraction failed (non-fatal)", "note_id", noteID, "error", countErr)
		}
	}

	// Update note metadata
	updateMap := map[string]interface{}{
		"has_pdf":         true,
		"pdf_size":        header.Size,
		"pdf_uploaded_at": time.Now(),
	}
	if pdfPages > 0 {
		updateMap["pdf_pages"] = pdfPages
	}

	if err := app.DB.Model(&note).Updates(updateMap).Error; err != nil {
		contentLog.Log("UPLOAD", "metadata update failed", "note_id", noteID, "error", err)
		// PDF was uploaded but metadata failed - not ideal but not fatal
		c.JSON(http.StatusInternalServerError, gin.H{"error": "PDF uploaded but metadata update failed"})
		return
	}

	// Auto-approve free notes if the subnotery setting is enabled.
	// This runs after upload so the note is still Pending when the PDF is saved.
	if note.Price == 0 && note.Status == models.StatusPending {
		var sub models.Subnotery
		if err := app.DB.Select("id", "auto_approve_free_notes").First(&sub, note.SubnoteryID).Error; err == nil && sub.AutoApproveFreeNotes {
			if err := app.DB.Model(&note).Update("status", models.StatusApproved).Error; err != nil {
				contentLog.Log("UPLOAD", "auto-approve update failed", "note_id", noteID, "error", err)
			} else {
				note.Status = models.StatusApproved
				contentLog.Log("UPLOAD", "auto-approved free note", "note_id", noteID)

				// Index in Meilisearch and add to hot feed
				if err := app.indexNote(note); err != nil {
					contentLog.Log("UPLOAD", "failed to index auto-approved note", "note_id", noteID, "error", err)
				}
				if err := app.AddNoteToFeed(c.Request.Context(), &note); err != nil {
					contentLog.Log("UPLOAD", "failed to add auto-approved note to feed", "note_id", noteID, "error", err)
				}
			}
		}
	}

	duration := time.Since(start)
	contentLog.Log("UPLOAD", "completed successfully", "note_id", noteID, "size_bytes", header.Size, "duration_ms", duration.Milliseconds())
	c.JSON(http.StatusOK, gin.H{
		"message":  "PDF uploaded successfully",
		"pdf_size": header.Size,
	})
}

// GetNotePDFContent serves the PDF content for viewing.
//
// This is the core endpoint for the in-app PDF viewer.
// It proxies the PDF from R2 with headers that encourage viewing over downloading.
//
// Response: application/pdf stream with inline disposition
//
// Route: GET /api/v1/notes/:id/content
func (app *App) GetNotePDFContent(c *gin.Context) {
	start := time.Now()
	contentLog.Log("VIEW", "request received")

	// Parse note ID and get user ID using helpers
	noteID, ok := helpers.MustParseNoteID(c)
	if !ok {
		return
	}
	userID := helpers.GetUserID(c)
	contentLog.Log("VIEW", "processing", "user_id", userID, "note_id", noteID)

	// Fetch the note (including soft-deleted for purchasers)
	var note models.Note
	if err := app.DB.Unscoped().First(&note, noteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			contentLog.Log("VIEW", "note not found", "note_id", noteID)
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
			return
		}
		contentLog.Log("VIEW", "database error", "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch note"})
		return
	}

	// If soft-deleted, only purchasers/creator/admins can access
	if note.DeletedAt.Valid {
		access := app.CheckNoteAccess(userID, &note)
		if access == AccessNone {
			contentLog.Log("VIEW", "denied - soft-deleted note, no access", "user_id", userID, "note_id", noteID)
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
			return
		}
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
	case models.StatusPending:
		// Only admins can view pending notes
		hasAccess = app.CanViewPendingNote(userID, &note)
		if !hasAccess {
			contentLog.Log("VIEW", "denied - pending note", "user_id", userID, "note_id", noteID)
			c.JSON(http.StatusForbidden, gin.H{"error": "This note is pending approval"})
			return
		}

	case models.StatusApproved:
		// Must have purchased or be admin
		hasAccess = app.CanViewApprovedNote(userID, &note)
		if !hasAccess {
			contentLog.Log("VIEW", "denied - not purchased", "user_id", userID, "note_id", noteID)
			c.JSON(http.StatusForbidden, gin.H{"error": "You must purchase this note to view it"})
			return
		}

	case models.StatusRejected:
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
	pdfContent, contentLength, err := app.R2.GetPDFContent(ctx, uint(noteID))
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
	c.Header("X-Notery-Access", "full")

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
//
// The access control in GetNotePDFContent already handles admin verification,
// so this just provides a semantically clearer endpoint for the admin workflow.
//
// Route: GET /api/v1/admin/notes/:id/preview
func (app *App) AdminPreviewPDF(c *gin.Context) {
	// The actual access control is in GetNotePDFContent
	// This endpoint just provides clearer semantics for admin UIs
	app.GetNotePDFContent(c)
}

// DeleteNotePDF removes a note's PDF from R2 storage.
//
// This should be called when a note is rejected or deleted.
// It's also exposed as an admin endpoint for manual cleanup.
//
// Route: DELETE /api/v1/admin/notes/:id/content (admin only)
func (app *App) DeleteNotePDF(c *gin.Context) {
	noteID, ok := helpers.MustParseNoteID(c)
	if !ok {
		return
	}

	contentLog.Log("DELETE", "processing", "note_id", noteID)

	// Verify note exists
	var note models.Note
	if err := app.DB.First(&note, noteID).Error; err != nil {
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
	if err := app.R2.DeletePDF(ctx, uint(noteID)); err != nil {
		contentLog.Log("DELETE", "R2 delete failed", "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete PDF"})
		return
	}

	// Invalidate cached preview PDFs (best-effort).
	go func() {
		if err := app.R2.DeletePreviewPDFs(ctx, uint(noteID)); err != nil {
			contentLog.Log("DELETE", "preview cache invalidation failed (non-fatal)", "note_id", noteID, "error", err)
		}
	}()

	// Update note metadata
	if err := app.DB.Model(&note).Updates(map[string]interface{}{
		"has_pdf":         false,
		"pdf_size":        0,
		"pdf_uploaded_at": nil,
		"pdf_pages":       0,
	}).Error; err != nil {
		contentLog.Log("DELETE", "metadata update failed (non-fatal)", "note_id", noteID, "error", err)
		// PDF deleted but metadata update failed - log but continue
	}

	contentLog.Log("DELETE", "completed successfully", "note_id", noteID)
	c.JSON(http.StatusOK, gin.H{"message": "PDF deleted successfully"})
}

// GetNotePreview serves a page-limited PDF preview for any authenticated user.
//
// The frontend requests a specific number of preview pages via the ?pages=N
// query parameter. The server extracts only those pages using pdfcpu and
// returns a valid PDF containing pages 1..N. Extracted previews are cached
// in R2 so repeated requests skip the extraction step.
//
// Admins viewing non-approved notes bypass preview extraction and receive
// the full PDF (they have full access through GetNotePDFContent anyway).
//
// Route: GET /api/v1/notes/:id/preview?pages=N
func (app *App) GetNotePreview(c *gin.Context) {
	contentLog.Log("PREVIEW", "request received")

	noteID, ok := helpers.MustParseNoteID(c)
	if !ok {
		return
	}

	// Parse required ?pages=N query parameter.
	pagesStr := c.Query("pages")
	if pagesStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pages query parameter is required"})
		return
	}
	pages, err := strconv.Atoi(pagesStr)
	if err != nil || pages < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pages must be a positive integer"})
		return
	}

	// Fetch the note (including soft-deleted for purchasers).
	var note models.Note
	if err := app.DB.Unscoped().First(&note, noteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch note"})
		return
	}

	// If soft-deleted, only purchasers/creator/admins can access.
	if note.DeletedAt.Valid {
		userID := helpers.GetUserID(c)
		access := app.CheckNoteAccess(userID, &note)
		if access == AccessNone {
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
			return
		}
	}

	// Only approved notes can be previewed publicly; admins can preview any note.
	isAdminPreview := false
	if note.Status != models.StatusApproved {
		userID := helpers.GetUserID(c)
		if userID == 0 || !app.CanViewPendingNote(userID, &note) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Note is not available for preview"})
			return
		}
		isAdminPreview = true
		contentLog.Log("PREVIEW", "admin viewing non-approved note", "note_id", noteID, "status", note.Status)
	}

	if !note.HasPDF {
		c.JSON(http.StatusNotFound, gin.H{"error": "No PDF content available"})
		return
	}

	ctx := c.Request.Context()

	// Admin preview of non-approved notes: serve full PDF (no extraction).
	if isAdminPreview {
		pdfContent, contentLength, err := app.R2.GetPDFContent(ctx, uint(noteID))
		if err != nil {
			contentLog.Log("PREVIEW", "R2 fetch failed", "note_id", noteID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve PDF"})
			return
		}
		defer pdfContent.Close()

		c.Header("Content-Type", "application/pdf")
		c.Header("Content-Disposition", "inline")
		c.Header("Content-Length", strconv.FormatInt(contentLength, 10))
		c.Header("Cache-Control", "no-store, private")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Header("X-Notery-Access", "admin-preview")
		c.Status(http.StatusOK)
		if _, err := io.Copy(c.Writer, pdfContent); err != nil {
			contentLog.Log("PREVIEW", "admin stream failed", "note_id", noteID, "error", err)
		}
		contentLog.Log("PREVIEW", "admin full preview served", "note_id", noteID, "bytes", contentLength)
		return
	}

	// --- Approved note preview: serve only the requested pages ---

	// 1) Check R2 cache for a previously extracted preview.
	cachedContent, cachedLen, cacheErr := app.R2.GetPreviewPDF(ctx, uint(noteID), pages)
	if cacheErr == nil {
		defer cachedContent.Close()
		contentLog.Log("PREVIEW", "serving cached preview", "note_id", noteID, "pages", pages)

		c.Header("Content-Type", "application/pdf")
		c.Header("Content-Disposition", "inline")
		c.Header("Content-Length", strconv.FormatInt(cachedLen, 10))
		c.Header("Cache-Control", "public, max-age=3600")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Header("X-Notery-Access", "preview")
		if note.PDFPages > 0 {
			c.Header("X-Total-Pages", strconv.Itoa(note.PDFPages))
		}

		c.Status(http.StatusOK)
		if _, err := io.Copy(c.Writer, cachedContent); err != nil {
			contentLog.Log("PREVIEW", "cached stream failed", "note_id", noteID, "error", err)
		}
		return
	}

	// 2) Cache miss — fetch the full PDF and extract the requested pages.
	pdfContent, _, fetchErr := app.R2.GetPDFContent(ctx, uint(noteID))
	if fetchErr != nil {
		contentLog.Log("PREVIEW", "R2 fetch failed", "note_id", noteID, "error", fetchErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve PDF"})
		return
	}
	defer pdfContent.Close()

	extracted, totalPages, extractErr := extractPreviewPages(pdfContent, pages)
	if extractErr != nil {
		contentLog.Log("PREVIEW", "extraction failed", "note_id", noteID, "pages", pages, "error", extractErr)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Preview extraction failed: " + extractErr.Error()})
		return
	}

	// 3) Lazy-backfill pdf_pages if not yet stored.
	if note.PDFPages == 0 && totalPages > 0 {
		if err := app.DB.Model(&note).Update("pdf_pages", totalPages).Error; err != nil {
			contentLog.Log("PREVIEW", "failed to backfill pdf_pages", "note_id", noteID, "error", err)
		} else {
			note.PDFPages = totalPages
		}
	}

	// 4) Cache the extracted preview in R2 (best-effort).
	go func() {
		if uploadErr := app.R2.UploadPreviewPDF(ctx, uint(noteID), pages, bytes.NewReader(extracted), int64(len(extracted))); uploadErr != nil {
			contentLog.Log("PREVIEW", "cache upload failed (non-fatal)", "note_id", noteID, "pages", pages, "error", uploadErr)
		} else {
			contentLog.Log("PREVIEW", "cached preview in R2", "note_id", noteID, "pages", pages)
		}
	}()

	// 5) Serve the extracted preview.
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", "inline")
	c.Header("Content-Length", strconv.Itoa(len(extracted)))
	c.Header("Cache-Control", "public, max-age=3600")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "SAMEORIGIN")
	c.Header("X-Notery-Access", "preview")
	c.Header("X-Total-Pages", strconv.Itoa(totalPages))

	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, bytes.NewReader(extracted)); err != nil {
		contentLog.Log("PREVIEW", "stream failed", "note_id", noteID, "error", err)
	}
	contentLog.Log("PREVIEW", "served extracted preview", "note_id", noteID, "pages", pages, "total_pages", totalPages, "size_bytes", len(extracted))
}
