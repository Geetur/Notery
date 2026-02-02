// Package database/r2.go contains the Cloudflare R2 storage client initialization and operations.
// R2 is used to store PDF content for notes securely. All PDF operations go through this package.
//
// ARCHITECTURE OVERVIEW:
// ---------------------
// Cloudflare R2 is an S3-compatible object storage. We use it to:
// 1. Store PDF files uploaded by note creators
// 2. Generate short-lived signed URLs for secure viewing (no permanent download links)
// 3. Serve PDFs through a proxy endpoint to prevent direct URL sharing
//
// SECURITY MODEL:
// ---------------
// - PDFs are stored in a private bucket (no public access)
// - We NEVER give users direct R2 URLs
// - Instead, the API proxies the PDF content through an authenticated endpoint
// - This prevents users from sharing direct links and ensures access control
// - Pre-signed URLs are only used internally by the server for fetching content
//
// KEY CONCEPTS:
// -------------
// - S3Client: The main client for upload/delete operations
// - PresignClient: Generates temporary signed URLs (used server-side only)
// - Object keys: PDFs are stored as "notes/{note_id}/content.pdf" for easy lookup
package database

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// r2Client is the S3-compatible client for Cloudflare R2 operations.
// Used for upload, delete, and object management operations.
var r2Client *s3.Client

// preSignedClient is used to generate time-limited signed URLs.
// These URLs are only used server-side for fetching PDF content securely.
var preSignedClient *s3.PresignClient

// r2BucketName stores the bucket name for PDF storage.
// Configured via R2_BUCKET_NAME environment variable.
var r2BucketName string

// R2Client provides a wrapper around R2 operations for PDF management.
// This struct encapsulates all PDF-related R2 operations in a clean interface.
type R2Client struct {
	S3Client      *s3.Client
	PresignClient *s3.PresignClient
	BucketName    string
}

// InitR2 initializes the Cloudflare R2 S3-compatible client and presign client.
// Returns the R2Client wrapper or an error if initialization fails.
//
// Required environment variables:
// - R2_ACCOUNT_ID: Your Cloudflare account ID
// - R2_ACCESS_KEY_ID: R2 API access key
// - R2_SECRET_ACCESS_KEY: R2 API secret key
// - R2_BUCKET_NAME: The bucket to store PDFs in
func InitR2() (*R2Client, error) {
	log.Println("Initializing Cloudflare R2 client...")

	accountID := getenv("R2_ACCOUNT_ID", "")
	accessKeyID := getenv("R2_ACCESS_KEY_ID", "")
	secretAccessKey := getenv("R2_SECRET_ACCESS_KEY", "")
	r2BucketName = getenv("R2_BUCKET_NAME", "notery-pdfs")

	// Validate required configuration
	if accountID == "" || accessKeyID == "" || secretAccessKey == "" {
		return nil, fmt.Errorf("R2 configuration incomplete: ensure R2_ACCOUNT_ID, R2_ACCESS_KEY_ID, and R2_SECRET_ACCESS_KEY are set")
	}

	// R2 uses a custom endpoint based on your Cloudflare account ID.
	// The endpoint format is: https://{account_id}.r2.cloudflarestorage.com
	// We use "auto" as the region since R2 doesn't use traditional AWS regions.
	customResolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			if service == s3.ServiceID && region == "auto" {
				return aws.Endpoint{
					URL:           "https://" + accountID + ".r2.cloudflarestorage.com",
					SigningRegion: "auto",
				}, nil
			}
			// Fall back to default resolution for other services
			return aws.Endpoint{}, &aws.EndpointNotFoundError{}
		},
	)

	// Load AWS config with R2 credentials and custom endpoint.
	// We use static credentials since R2 has its own access keys separate from AWS.
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithEndpointResolverWithOptions(customResolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKeyID,
			secretAccessKey,
			"", // session token not used with R2
		)),
		config.WithRegion("auto"), // R2 uses "auto" region
	)
	if err != nil {
		log.Printf("Failed to load R2 config: %v", err)
		return nil, err
	}

	// Initialize the S3 client for R2 operations
	r2Client = s3.NewFromConfig(cfg)
	preSignedClient = s3.NewPresignClient(r2Client)

	log.Println("Cloudflare R2 client initialized successfully.")

	return &R2Client{
		S3Client:      r2Client,
		PresignClient: preSignedClient,
		BucketName:    r2BucketName,
	}, nil
}

// ----- PDF STORAGE OPERATIONS -----
// These methods handle the core CRUD operations for PDF files in R2.

// GetPDFObjectKey generates the R2 object key for a note's PDF content.
// Format: "notes/{noteID}/content.pdf"
// This consistent naming allows easy lookup and management of PDF files.
func (r *R2Client) GetPDFObjectKey(noteID uint) string {
	return fmt.Sprintf("notes/%d/content.pdf", noteID)
}

// UploadPDF uploads a PDF file to R2 for a specific note.
// UploadPDF interacts with R2 to store the PDF content.
// UploadPDF does not interact with any other handler methods.
//
// Parameters:
// - ctx: Request context for cancellation/timeout
// - noteID: The note this PDF belongs to
// - content: The PDF file content as an io.Reader
// - contentLength: Size of the PDF in bytes (required for R2)
//
// Returns error if upload fails.
func (r *R2Client) UploadPDF(ctx context.Context, noteID uint, content io.Reader, contentLength int64) error {
	objectKey := r.GetPDFObjectKey(noteID)
	log.Printf("Uploading PDF for note %d to R2: %s", noteID, objectKey)

	// PutObject uploads the file to R2.
	// We set ContentType to application/pdf for proper handling.
	// ContentDisposition is set to "inline" to encourage browser viewing over download.
	_, err := r.S3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:             aws.String(r.BucketName),
		Key:                aws.String(objectKey),
		Body:               content,
		ContentLength:      aws.Int64(contentLength),
		ContentType:        aws.String("application/pdf"),
		ContentDisposition: aws.String("inline"), // Hints to view inline, not download
	})

	if err != nil {
		log.Printf("Failed to upload PDF for note %d: %v", noteID, err)
		return fmt.Errorf("failed to upload PDF: %w", err)
	}

	log.Printf("Successfully uploaded PDF for note %d", noteID)
	return nil
}

// GetPDFContent retrieves the raw PDF content from R2.
// GetPDFContent interacts with R2 to fetch the PDF bytes.
// GetPDFContent does not interact with any other handler methods.
//
// This method is used by the proxy endpoint to stream PDF content to authenticated users.
// The returned io.ReadCloser MUST be closed by the caller to prevent resource leaks.
//
// Parameters:
// - ctx: Request context for cancellation/timeout
// - noteID: The note whose PDF to retrieve
//
// Returns:
// - io.ReadCloser: The PDF content stream (caller must close)
// - int64: Content length in bytes
// - error: Any error that occurred
func (r *R2Client) GetPDFContent(ctx context.Context, noteID uint) (io.ReadCloser, int64, error) {
	objectKey := r.GetPDFObjectKey(noteID)
	log.Printf("Fetching PDF content for note %d from R2: %s", noteID, objectKey)

	// GetObject retrieves the file from R2
	result, err := r.S3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.BucketName),
		Key:    aws.String(objectKey),
	})

	if err != nil {
		log.Printf("Failed to get PDF for note %d: %v", noteID, err)
		return nil, 0, fmt.Errorf("failed to retrieve PDF: %w", err)
	}

	contentLength := int64(0)
	if result.ContentLength != nil {
		contentLength = *result.ContentLength
	}

	log.Printf("Successfully retrieved PDF for note %d (size: %d bytes)", noteID, contentLength)
	return result.Body, contentLength, nil
}

// DeletePDF removes a note's PDF from R2.
// DeletePDF interacts with R2 to remove the PDF file.
// DeletePDF does not interact with any other handler methods.
//
// Called when a note is deleted or rejected to clean up storage.
//
// Parameters:
// - ctx: Request context for cancellation/timeout
// - noteID: The note whose PDF to delete
//
// Returns error if deletion fails.
func (r *R2Client) DeletePDF(ctx context.Context, noteID uint) error {
	objectKey := r.GetPDFObjectKey(noteID)
	log.Printf("Deleting PDF for note %d from R2: %s", noteID, objectKey)

	_, err := r.S3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.BucketName),
		Key:    aws.String(objectKey),
	})

	if err != nil {
		log.Printf("Failed to delete PDF for note %d: %v", noteID, err)
		return fmt.Errorf("failed to delete PDF: %w", err)
	}

	log.Printf("Successfully deleted PDF for note %d", noteID)
	return nil
}

// PDFExists checks if a PDF exists in R2 for the given note.
// PDFExists interacts with R2 to check object existence.
// PDFExists does not interact with any other handler methods.
//
// Uses HeadObject which is more efficient than GetObject for existence checks.
//
// Parameters:
// - ctx: Request context for cancellation/timeout
// - noteID: The note to check
//
// Returns true if PDF exists, false otherwise.
func (r *R2Client) PDFExists(ctx context.Context, noteID uint) bool {
	objectKey := r.GetPDFObjectKey(noteID)

	// HeadObject checks if an object exists without downloading it
	_, err := r.S3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(r.BucketName),
		Key:    aws.String(objectKey),
	})

	return err == nil
}

// ----- INTERNAL PRESIGNED URL GENERATION -----
// These are used internally by the server. Users never see these URLs.

// GeneratePresignedURL creates a time-limited signed URL for internal use.
// GeneratePresignedURL interacts with R2 presign client.
// GeneratePresignedURL does not interact with any other handler methods.
//
// IMPORTANT: This is for SERVER-SIDE use only.
// The returned URL should NEVER be exposed to clients directly.
// Instead, use GetPDFContent to proxy the content through an authenticated endpoint.
//
// Parameters:
// - ctx: Request context for cancellation/timeout
// - noteID: The note whose PDF URL to generate
// - duration: How long the URL should remain valid
//
// Returns the signed URL or an error.
func (r *R2Client) GeneratePresignedURL(ctx context.Context, noteID uint, duration time.Duration) (string, error) {
	objectKey := r.GetPDFObjectKey(noteID)

	// Generate a presigned GET URL
	presignResult, err := r.PresignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.BucketName),
		Key:    aws.String(objectKey),
	}, s3.WithPresignExpires(duration))

	if err != nil {
		log.Printf("Failed to generate presigned URL for note %d: %v", noteID, err)
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignResult.URL, nil
}

// GenerateUploadURL creates a time-limited signed URL for uploading PDFs.
// GenerateUploadURL interacts with R2 presign client.
// GenerateUploadURL does not interact with any other handler methods.
//
// This can be used for direct-to-R2 uploads from clients (advanced use case).
// For simplicity, the current implementation uses server-side uploads instead.
//
// Parameters:
// - ctx: Request context for cancellation/timeout
// - noteID: The note to upload PDF for
// - duration: How long the upload URL should remain valid
//
// Returns the signed upload URL or an error.
func (r *R2Client) GenerateUploadURL(ctx context.Context, noteID uint, duration time.Duration) (string, error) {
	objectKey := r.GetPDFObjectKey(noteID)

	// Generate a presigned PUT URL
	presignResult, err := r.PresignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(r.BucketName),
		Key:         aws.String(objectKey),
		ContentType: aws.String("application/pdf"),
	}, s3.WithPresignExpires(duration))

	if err != nil {
		log.Printf("Failed to generate upload URL for note %d: %v", noteID, err)
		return "", fmt.Errorf("failed to generate upload URL: %w", err)
	}

	return presignResult.URL, nil
}
