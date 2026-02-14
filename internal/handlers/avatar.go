// Package handlers/avatar.go handles avatar upload, retrieval, and deletion.
//
// ARCHITECTURE:
//
//	Avatars are stored in Cloudflare R2 at "avatars/{user_id}/avatar.{ext}".
//	Uploads are validated by both Content-Type and magic-byte file signatures
//	to prevent disguised file uploads.
//
//	Supported formats: JPEG, PNG, WebP, GIF
//	Maximum file size: 5 MB
//
//	After upload, user.AvatarURL is updated with the R2 object key.
//	The avatar is served through a proxy endpoint (same pattern as PDFs).
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
)

// avatarLog is the domain-specific logger for avatar operations.
var avatarLog = helpers.NewLogger("AVATAR")

// Avatar upload constraints.
const (
	// MaxAvatarSize is the maximum allowed avatar file size (5 MB).
	MaxAvatarSize = 5 << 20 // 5 * 1024 * 1024

	// avatarBucketPrefix is the R2 key prefix for avatar storage.
	avatarBucketPrefix = "avatars"
)

// allowedImageTypes maps Content-Type to file extension for allowed avatar formats.
var allowedImageTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
	"image/gif":  "gif",
}

// magicBytes maps file format to their magic byte signatures.
// Used to validate that the file content matches the declared Content-Type.
var magicBytes = map[string][][]byte{
	"image/jpeg": {
		{0xFF, 0xD8, 0xFF}, // JPEG/JFIF/Exif
	},
	"image/png": {
		{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, // PNG
	},
	"image/webp": {
		// WebP starts with RIFF....WEBP — check RIFF header + WEBP at offset 8
		{0x52, 0x49, 0x46, 0x46}, // "RIFF" (first 4 bytes; WEBP checked separately)
	},
	"image/gif": {
		{0x47, 0x49, 0x46, 0x38, 0x37, 0x61}, // GIF87a
		{0x47, 0x49, 0x46, 0x38, 0x39, 0x61}, // GIF89a
	},
}

// UploadAvatar handles avatar file upload with MIME and magic-byte validation.
// Endpoint: POST /me/avatar
func (app *App) UploadAvatar(c *gin.Context) {
	userID := c.GetUint64("user_id")
	avatarLog.Log("UPLOAD", "Processing avatar upload", "userID", userID)

	if app.R2 == nil {
		avatarLog.Log("UPLOAD", "R2 not configured")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "File storage not configured"})
		return
	}

	// Enforce file size limit via request body limiter
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxAvatarSize+512) // small buffer for multipart overhead

	file, header, err := c.Request.FormFile("avatar")
	if err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			avatarLog.Log("UPLOAD", "File too large", "userID", userID)
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("File too large. Maximum size is %d MB", MaxAvatarSize>>20)})
			return
		}
		avatarLog.Log("UPLOAD", "Failed to read uploaded file", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Avatar file required (multipart field 'avatar')"})
		return
	}
	defer file.Close()

	// Validate file size
	if header.Size > MaxAvatarSize {
		avatarLog.Log("UPLOAD", "File exceeds size limit", "size", header.Size, "limit", MaxAvatarSize)
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("File too large. Maximum size is %d MB", MaxAvatarSize>>20)})
		return
	}

	// Validate Content-Type against allowed list
	contentType := header.Header.Get("Content-Type")
	ext, ok := allowedImageTypes[contentType]
	if !ok {
		avatarLog.Log("UPLOAD", "Unsupported content type", "contentType", contentType)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":           "Unsupported image format",
			"allowed_formats": []string{"JPEG", "PNG", "WebP", "GIF"},
		})
		return
	}

	// Read the first 12 bytes for magic-byte validation
	magicBuf := make([]byte, 12)
	n, err := io.ReadFull(file, magicBuf)
	if err != nil && err != io.ErrUnexpectedEOF {
		avatarLog.Log("UPLOAD", "Failed to read file header", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read file"})
		return
	}
	magicBuf = magicBuf[:n]

	// Validate magic bytes match declared content type
	if !validateMagicBytes(contentType, magicBuf) {
		avatarLog.Log("UPLOAD", "Magic byte mismatch — possible disguised file",
			"declaredType", contentType, "userID", userID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "File content does not match declared type"})
		return
	}

	// Seek back to beginning for the full upload
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		avatarLog.Log("UPLOAD", "Failed to seek file", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process file"})
		return
	}

	// Upload to R2
	objectKey := getAvatarObjectKey(userID, ext)
	ctx := c.Request.Context()
	if err := app.uploadAvatar(ctx, objectKey, file, header.Size, contentType); err != nil {
		avatarLog.Log("UPLOAD", "Failed to upload to R2", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload avatar"})
		return
	}

	// Update user's avatar URL in database
	if err := app.DB.Model(&struct{ ID uint64 }{ID: userID}).
		Table("users").
		Where("id = ?", userID).
		Update("avatar_url", objectKey).Error; err != nil {
		avatarLog.Log("UPLOAD", "Failed to update user avatar URL", "error", err)
		// Don't fail the request — avatar is stored, URL update can be retried
	}

	avatarLog.Log("UPLOAD", "Avatar uploaded successfully", "userID", userID, "key", objectKey)
	c.JSON(http.StatusOK, gin.H{
		"message":    "Avatar uploaded successfully",
		"avatar_url": objectKey,
	})
}

// DeleteAvatar removes the user's avatar from R2 and clears the URL.
// Endpoint: DELETE /me/avatar
func (app *App) DeleteAvatar(c *gin.Context) {
	userID := c.GetUint64("user_id")
	avatarLog.Log("DELETE", "Processing avatar deletion", "userID", userID)

	if app.R2 == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "File storage not configured"})
		return
	}

	// Get current avatar URL
	var avatarURL string
	app.DB.Table("users").Where("id = ?", userID).Pluck("avatar_url", &avatarURL)

	if avatarURL == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "No avatar to delete"})
		return
	}

	// Delete from R2
	ctx := c.Request.Context()
	if err := app.deleteAvatar(ctx, avatarURL); err != nil {
		avatarLog.Log("DELETE", "Failed to delete from R2", "error", err)
		// Continue to clear URL even if R2 deletion fails
	}

	// Clear avatar URL in database
	app.DB.Table("users").Where("id = ?", userID).Update("avatar_url", "")

	avatarLog.Log("DELETE", "Avatar deleted successfully", "userID", userID)
	c.JSON(http.StatusOK, gin.H{"message": "Avatar deleted successfully"})
}

// GetAvatar proxies the avatar file from R2.
// Endpoint: GET /avatars/:user_id (public)
func (app *App) GetAvatar(c *gin.Context) {
	userIDParam := c.Param("user_id")

	if app.R2 == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "File storage not configured"})
		return
	}

	// Get avatar URL from user record
	var avatarURL string
	app.DB.Table("users").Where("id = ?", userIDParam).Pluck("avatar_url", &avatarURL)

	if avatarURL == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "No avatar found"})
		return
	}

	// Fetch from R2 and stream to client
	ctx := c.Request.Context()
	body, contentLength, contentType, err := app.getAvatarContent(ctx, avatarURL)
	if err != nil {
		avatarLog.Log("GET", "Failed to fetch avatar from R2", "error", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Avatar not found"})
		return
	}
	defer body.Close()

	c.Header("Cache-Control", "public, max-age=86400") // 24h cache
	c.DataFromReader(http.StatusOK, contentLength, contentType, body, nil)
}

// ===== INTERNAL HELPERS =====

func getAvatarObjectKey(userID uint64, ext string) string {
	return fmt.Sprintf("%s/%d/avatar.%s", avatarBucketPrefix, userID, ext)
}

func validateMagicBytes(contentType string, data []byte) bool {
	signatures, ok := magicBytes[contentType]
	if !ok {
		return false
	}

	for _, sig := range signatures {
		if len(data) >= len(sig) {
			match := true
			for i, b := range sig {
				if data[i] != b {
					match = false
					break
				}
			}
			if match {
				// Special case for WebP: also verify "WEBP" at offset 8
				if contentType == "image/webp" {
					if len(data) >= 12 && string(data[8:12]) == "WEBP" {
						return true
					}
					return false
				}
				return true
			}
		}
	}
	return false
}

// uploadAvatar uploads avatar bytes to R2 using the S3 PutObject API.
func (app *App) uploadAvatar(ctx context.Context, objectKey string, content io.Reader, contentLength int64, contentType string) error {
	_, err := app.R2.S3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(app.R2.BucketName),
		Key:           aws.String(objectKey),
		Body:          content,
		ContentLength: aws.Int64(contentLength),
		ContentType:   aws.String(contentType),
	})
	return err
}

// deleteAvatar removes an avatar file from R2.
func (app *App) deleteAvatar(ctx context.Context, objectKey string) error {
	_, err := app.R2.S3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(app.R2.BucketName),
		Key:    aws.String(objectKey),
	})
	return err
}

// getAvatarContent fetches avatar bytes from R2.
func (app *App) getAvatarContent(ctx context.Context, objectKey string) (io.ReadCloser, int64, string, error) {
	result, err := app.R2.S3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(app.R2.BucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return nil, 0, "", err
	}
	contentLength := int64(0)
	if result.ContentLength != nil {
		contentLength = *result.ContentLength
	}
	contentType := "application/octet-stream"
	if result.ContentType != nil {
		contentType = *result.ContentType
	}
	return result.Body, contentLength, contentType, nil
}
