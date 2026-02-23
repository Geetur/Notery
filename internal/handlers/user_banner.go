// user_banner.go — HTTP handlers for user profile banner upload, retrieval, and deletion.
//
// ENDPOINTS:
//
//	POST   /me/banner          Upload a new profile banner (multipart, validated)
//	GET    /users/:id/banner   Public proxy for a user's banner (cached 24h)
//	DELETE /me/banner          Delete own profile banner
//
// ARCHITECTURE:
//
//	User banners are stored in Cloudflare R2 at "user-banners/{user_id}/banner.{ext}".
//	Uploads are validated by both Content-Type header and magic-byte file
//	signatures to prevent disguised file uploads.
//
//	Supported formats: JPEG, PNG, WebP, GIF
//	Maximum file size: 5 MB
//
//	After upload, user.BannerURL is updated with the R2 object key.
//	The banner is served through a public proxy endpoint with a 24-hour cache.
package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"

	"github.com/Geetur/Notery/internal/helpers"
)

// userBannerLog is the domain-specific logger for user banner operations.
var userBannerLog = helpers.NewLogger("USER_BANNER")

// User banner constants.
const (
	// MaxUserBannerSize is the maximum allowed banner file size (5 MB).
	MaxUserBannerSize = 5 << 20

	// userBannerBucketPrefix is the R2 key prefix for user banner storage.
	userBannerBucketPrefix = "user-banners"
)

// UploadUserBanner handles user profile banner upload with MIME and magic-byte validation.
//
// Reads the multipart "banner" field, validates file size (≤5 MB), checks
// Content-Type against the allowed list (JPEG/PNG/WebP/GIF), and verifies
// magic bytes match the declared type. On success, uploads to R2 and updates
// the user's banner_url in the database.
//
// Route: POST /api/v1/me/banner
func (app *App) UploadUserBanner(c *gin.Context) {
	userID := c.GetUint64("user_id")
	userBannerLog.Log("UPLOAD", "Processing banner upload", "userID", userID)

	if app.R2 == nil {
		userBannerLog.Log("UPLOAD", "R2 not configured")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "File storage not configured"})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxUserBannerSize+512)

	file, header, err := c.Request.FormFile("banner")
	if err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			userBannerLog.Log("UPLOAD", "File too large", "userID", userID)
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("File too large. Maximum size is %d MB", MaxUserBannerSize>>20)})
			return
		}
		userBannerLog.Log("UPLOAD", "Failed to read uploaded file", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Banner file required (multipart field 'banner')"})
		return
	}
	defer file.Close()

	if header.Size > MaxUserBannerSize {
		userBannerLog.Log("UPLOAD", "File exceeds size limit", "size", header.Size, "limit", MaxUserBannerSize)
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("File too large. Maximum size is %d MB", MaxUserBannerSize>>20)})
		return
	}

	contentType := header.Header.Get("Content-Type")
	ext, ok := allowedImageTypes[contentType]
	if !ok {
		userBannerLog.Log("UPLOAD", "Unsupported content type", "contentType", contentType)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":           "Unsupported image format",
			"allowed_formats": []string{"JPEG", "PNG", "WebP", "GIF"},
		})
		return
	}

	magicBuf := make([]byte, 12)
	n, err := io.ReadFull(file, magicBuf)
	if err != nil && err != io.ErrUnexpectedEOF {
		userBannerLog.Log("UPLOAD", "Failed to read file header", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read file"})
		return
	}
	magicBuf = magicBuf[:n]

	if !validateMagicBytes(contentType, magicBuf) {
		userBannerLog.Log("UPLOAD", "Magic byte mismatch — possible disguised file",
			"declaredType", contentType, "userID", userID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "File content does not match declared type"})
		return
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		userBannerLog.Log("UPLOAD", "Failed to seek file", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process file"})
		return
	}

	objectKey := fmt.Sprintf("%s/%d/banner.%s", userBannerBucketPrefix, userID, ext)
	ctx := c.Request.Context()
	_, err = app.R2.S3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(app.R2.BucketName),
		Key:           aws.String(objectKey),
		Body:          file,
		ContentLength: aws.Int64(header.Size),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		userBannerLog.Log("UPLOAD", "Failed to upload to R2", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload banner"})
		return
	}

	if err := app.DB.Table("users").Where("id = ?", userID).Update("banner_url", objectKey).Error; err != nil {
		userBannerLog.Log("UPLOAD", "Failed to update user banner URL", "error", err)
	}

	userBannerLog.Log("UPLOAD", "Banner uploaded successfully", "userID", userID, "key", objectKey)
	c.JSON(http.StatusOK, gin.H{
		"message":    "Banner uploaded successfully",
		"banner_url": objectKey,
	})
}

// DeleteUserBanner removes the user's profile banner from R2 and clears the URL.
//
// Route: DELETE /api/v1/me/banner
func (app *App) DeleteUserBanner(c *gin.Context) {
	userID := c.GetUint64("user_id")
	userBannerLog.Log("DELETE", "Processing banner deletion", "userID", userID)

	if app.R2 == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "File storage not configured"})
		return
	}

	var bannerURL string
	app.DB.Table("users").Where("id = ?", userID).Pluck("banner_url", &bannerURL)

	if bannerURL == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "No banner to delete"})
		return
	}

	ctx := c.Request.Context()
	_, _ = app.R2.S3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(app.R2.BucketName),
		Key:    aws.String(bannerURL),
	})

	app.DB.Table("users").Where("id = ?", userID).Update("banner_url", "")

	userBannerLog.Log("DELETE", "Banner deleted successfully", "userID", userID)
	c.JSON(http.StatusOK, gin.H{"message": "Banner deleted successfully"})
}

// GetUserBanner proxies the user's profile banner from R2.
//
// Route: GET /api/v1/users/:id/banner
func (app *App) GetUserBanner(c *gin.Context) {
	userIDParam := c.Param("id")

	if app.R2 == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "File storage not configured"})
		return
	}

	var bannerURL string
	app.DB.Table("users").Where("id = ?", userIDParam).Pluck("banner_url", &bannerURL)

	if bannerURL == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "No banner found"})
		return
	}

	ctx := c.Request.Context()
	result, err := app.R2.S3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(app.R2.BucketName),
		Key:    aws.String(bannerURL),
	})
	if err != nil {
		userBannerLog.Log("GET", "Failed to fetch banner from R2", "error", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Banner not found"})
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

	c.Header("Cache-Control", "public, max-age=86400")
	c.DataFromReader(http.StatusOK, contentLength, contentType, result.Body, nil)
}
