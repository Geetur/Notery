// thumbnail.go — HTTP handlers for note thumbnail upload, retrieval, and deletion.
//
// ENDPOINTS:
//
//	POST   /notes/:id/thumbnail   Upload a thumbnail image for a note (creator only)
//	GET    /notes/:id/thumbnail   Public proxy for a note's thumbnail (cached 24h)
//	DELETE /notes/:id/thumbnail   Delete a note's thumbnail (creator only)
//
// ARCHITECTURE:
//
//	Thumbnails are stored in Cloudflare R2 at "notes/{note_id}/thumbnail.{ext}".
//	Uploads are validated by both Content-Type header and magic-byte file
//	signatures (reusing avatar validation logic).
//
//	Supported formats: JPEG, PNG, WebP, GIF
//	Maximum file size: 5 MB
package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// thumbnailLog is the domain-specific logger for thumbnail operations.
var thumbnailLog = helpers.NewLogger("THUMBNAIL")

// Thumbnail upload constraints.
const (
	// MaxThumbnailSize is the maximum allowed thumbnail file size (5 MB).
	MaxThumbnailSize = 5 << 20

	// thumbnailBucketPrefix is the R2 key prefix for thumbnail storage.
	thumbnailBucketPrefix = "notes"
)

// getThumbnailObjectKey generates the R2 object key for a note's thumbnail.
// Format: "notes/{noteID}/thumbnail.{ext}"
func getThumbnailObjectKey(noteID uint, ext string) string {
	return fmt.Sprintf("%s/%d/thumbnail.%s", thumbnailBucketPrefix, noteID, ext)
}

// UploadThumbnail handles thumbnail image upload with MIME and magic-byte validation.
//
// Only the note creator can upload a thumbnail. Validates file size (≤5 MB),
// Content-Type (JPEG/PNG/WebP/GIF), and magic bytes. On success, uploads to R2
// and updates the note's thumbnail fields.
//
// DB: SELECT note by ID, UPDATE note.has_thumbnail + note.thumbnail_url via GORM.
// Technologies: Cloudflare R2 (S3 PutObject), PostgreSQL (GORM).
//
// Route: POST /api/v1/notes/:id/thumbnail
func (app *App) UploadThumbnail(c *gin.Context) {
	userID := helpers.GetUserID(c)
	thumbnailLog.Log("UPLOAD", "Processing thumbnail upload", "userID", userID)

	if app.R2 == nil {
		thumbnailLog.Log("UPLOAD", "R2 not configured")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "File storage not configured"})
		return
	}

	// Fetch the note and verify ownership
	note, ok := helpers.MustFetchNote(c, app.DB)
	if !ok {
		return
	}

	if note.CreatorID != userID {
		thumbnailLog.Log("UPLOAD", "User is not the note creator", "userID", userID, "creatorID", note.CreatorID)
		c.JSON(http.StatusForbidden, gin.H{"error": "Only the note creator can upload a thumbnail"})
		return
	}

	// Enforce file size limit
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxThumbnailSize+512)

	file, header, err := c.Request.FormFile("thumbnail")
	if err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			thumbnailLog.Log("UPLOAD", "File too large", "userID", userID)
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("File too large. Maximum size is %d MB", MaxThumbnailSize>>20)})
			return
		}
		thumbnailLog.Log("UPLOAD", "Failed to read uploaded file", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Thumbnail file required (multipart field 'thumbnail')"})
		return
	}
	defer file.Close()

	// Validate file size
	if header.Size > MaxThumbnailSize {
		thumbnailLog.Log("UPLOAD", "File exceeds size limit", "size", header.Size)
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("File too large. Maximum size is %d MB", MaxThumbnailSize>>20)})
		return
	}

	// Validate Content-Type against allowed image types (reuse avatar allowedImageTypes)
	contentType := header.Header.Get("Content-Type")
	ext, ok := allowedImageTypes[contentType]
	if !ok {
		thumbnailLog.Log("UPLOAD", "Unsupported content type", "contentType", contentType)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":           "Unsupported image format",
			"allowed_formats": []string{"JPEG", "PNG", "WebP", "GIF"},
		})
		return
	}

	// Read first 12 bytes for magic-byte validation
	magicBuf := make([]byte, 12)
	n, err := io.ReadFull(file, magicBuf)
	if err != nil && err != io.ErrUnexpectedEOF {
		thumbnailLog.Log("UPLOAD", "Failed to read file header", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read file"})
		return
	}
	magicBuf = magicBuf[:n]

	// Validate magic bytes match declared content type (reuse avatar validateMagicBytes)
	if !validateMagicBytes(contentType, magicBuf) {
		thumbnailLog.Log("UPLOAD", "Magic byte mismatch — possible disguised file",
			"declaredType", contentType, "userID", userID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "File content does not match declared type"})
		return
	}

	// Seek back to beginning for the full upload
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		thumbnailLog.Log("UPLOAD", "Failed to seek file", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process file"})
		return
	}

	// Delete old thumbnail if one exists
	if note.HasThumbnail && note.ThumbnailURL != "" {
		ctx := c.Request.Context()
		if _, delErr := app.R2.S3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(app.R2.BucketName),
			Key:    aws.String(note.ThumbnailURL),
		}); delErr != nil {
			thumbnailLog.Log("UPLOAD", "Failed to delete old thumbnail", "error", delErr)
			// Continue — not fatal
		}
	}

	// Upload to R2
	objectKey := getThumbnailObjectKey(note.ID, ext)
	ctx := c.Request.Context()
	if _, err := app.R2.S3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(app.R2.BucketName),
		Key:           aws.String(objectKey),
		Body:          file,
		ContentLength: aws.Int64(header.Size),
		ContentType:   aws.String(contentType),
	}); err != nil {
		thumbnailLog.Log("UPLOAD", "Failed to upload to R2", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload thumbnail"})
		return
	}

	// Update note's thumbnail fields in database
	if err := app.DB.Model(&models.Note{}).Where("id = ?", note.ID).Updates(map[string]interface{}{
		"has_thumbnail": true,
		"thumbnail_url": objectKey,
	}).Error; err != nil {
		thumbnailLog.Log("UPLOAD", "Failed to update note thumbnail fields", "error", err)
		// File is stored, DB update can be retried
	}

	thumbnailLog.Log("UPLOAD", "Thumbnail uploaded successfully", "noteID", note.ID, "key", objectKey)
	c.JSON(http.StatusOK, gin.H{
		"message":       "Thumbnail uploaded successfully",
		"thumbnail_url": objectKey,
	})
}

// GetThumbnail proxies the thumbnail image from R2.
//
// Public endpoint — returns the thumbnail with a 24-hour cache header.
// Returns 404 if note has no thumbnail.
//
// Route: GET /api/v1/notes/:id/thumbnail
func (app *App) GetThumbnail(c *gin.Context) {
	if app.R2 == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "File storage not configured"})
		return
	}

	// Fetch note to get thumbnail URL
	note, ok := helpers.MustFetchNote(c, app.DB)
	if !ok {
		return
	}

	if !note.HasThumbnail || note.ThumbnailURL == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "No thumbnail found"})
		return
	}

	// Fetch from R2 and stream to client
	ctx := c.Request.Context()
	result, err := app.R2.S3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(app.R2.BucketName),
		Key:    aws.String(note.ThumbnailURL),
	})
	if err != nil {
		thumbnailLog.Log("GET", "Failed to fetch thumbnail from R2", "error", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Thumbnail not found"})
		return
	}
	defer result.Body.Close()

	contentLength := int64(0)
	if result.ContentLength != nil {
		contentLength = *result.ContentLength
	}
	contentType := "application/octet-stream"
	if result.ContentType != nil {
		contentType = *result.ContentType
	}

	c.Header("Cache-Control", "public, max-age=86400") // 24h cache
	c.DataFromReader(http.StatusOK, contentLength, contentType, result.Body, nil)
}

// DeleteThumbnail removes the note's thumbnail from R2 and clears the fields.
//
// Only the note creator can delete their thumbnail. Returns 404 if no
// thumbnail exists.
//
// Route: DELETE /api/v1/notes/:id/thumbnail
func (app *App) DeleteThumbnail(c *gin.Context) {
	userID := helpers.GetUserID(c)
	thumbnailLog.Log("DELETE", "Processing thumbnail deletion", "userID", userID)

	if app.R2 == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "File storage not configured"})
		return
	}

	// Fetch note and verify ownership
	note, ok := helpers.MustFetchNote(c, app.DB)
	if !ok {
		return
	}

	if note.CreatorID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only the note creator can delete the thumbnail"})
		return
	}

	if !note.HasThumbnail || note.ThumbnailURL == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "No thumbnail to delete"})
		return
	}

	// Delete from R2
	ctx := c.Request.Context()
	if _, err := app.R2.S3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(app.R2.BucketName),
		Key:    aws.String(note.ThumbnailURL),
	}); err != nil {
		thumbnailLog.Log("DELETE", "Failed to delete from R2", "error", err)
		// Continue to clear fields
	}

	// Clear thumbnail fields in database
	if err := app.DB.Model(&models.Note{}).Where("id = ?", note.ID).Updates(map[string]interface{}{
		"has_thumbnail": false,
		"thumbnail_url": "",
	}).Error; err != nil {
		thumbnailLog.Log("DELETE", "Failed to update note", "error", err)
	}

	thumbnailLog.Log("DELETE", "Thumbnail deleted successfully", "noteID", note.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Thumbnail deleted successfully"})
}

// deleteThumbnailFromR2 removes a note's thumbnail from R2. Best-effort: logs failures
// but does not propagate errors to the caller.
func (app *App) deleteThumbnailFromR2(ctx context.Context, objectKey string) {
	if app.R2 != nil {
		if _, err := app.R2.S3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(app.R2.BucketName),
			Key:    aws.String(objectKey),
		}); err != nil {
			thumbnailLog.Log("CLEANUP", "Failed to delete thumbnail from R2", "key", objectKey, "error", err)
		} else {
			thumbnailLog.Log("CLEANUP", "Deleted thumbnail from R2", "key", objectKey)
		}
	}
}
