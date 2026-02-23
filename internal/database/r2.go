// r2.go — Cloudflare R2 (S3-compatible) storage client for PDF management.
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
//
// For local development, set R2_ENDPOINT to point to a local S3-compatible
// service like MinIO (e.g. http://localhost:9000). When R2_ENDPOINT is set,
// R2_ACCOUNT_ID is not required.
func InitR2() (*R2Client, error) {
	log.Println("Initializing S3-compatible storage client...")

	accountID := getenv("R2_ACCOUNT_ID", "")
	accessKeyID := getenv("R2_ACCESS_KEY_ID", "")
	secretAccessKey := getenv("R2_SECRET_ACCESS_KEY", "")
	bucketName := getenv("R2_BUCKET_NAME", "notery-pdfs")
	customEndpoint := getenv("R2_ENDPOINT", "") // e.g. http://localhost:9000 for MinIO

	// Validate required configuration
	if accessKeyID == "" || secretAccessKey == "" {
		// If no credentials at all, check if we should use MinIO defaults
		if customEndpoint == "" {
			return nil, fmt.Errorf("R2/S3 configuration incomplete: set R2_ACCESS_KEY_ID + R2_SECRET_ACCESS_KEY (and R2_ACCOUNT_ID for Cloudflare R2, or R2_ENDPOINT for MinIO/local S3)")
		}
	}

	// Determine the endpoint URL
	var endpointURL string
	var signingRegion string
	if customEndpoint != "" {
		// Local S3-compatible service (e.g. MinIO)
		endpointURL = customEndpoint
		signingRegion = "us-east-1"
		log.Printf("Using custom S3 endpoint: %s (dev mode)", customEndpoint)
	} else if accountID != "" {
		// Cloudflare R2
		endpointURL = "https://" + accountID + ".r2.cloudflarestorage.com"
		signingRegion = "auto"
	} else {
		return nil, fmt.Errorf("R2 configuration incomplete: set R2_ACCOUNT_ID (for Cloudflare R2) or R2_ENDPOINT (for local S3)")
	}

	customResolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               endpointURL,
				SigningRegion:     signingRegion,
				HostnameImmutable: true,
			}, nil
		},
	)

	// Load AWS config with credentials and custom endpoint.
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithEndpointResolverWithOptions(customResolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKeyID,
			secretAccessKey,
			"", // session token not used
		)),
		config.WithRegion(signingRegion),
	)
	if err != nil {
		log.Printf("Failed to load S3 config: %v", err)
		return nil, err
	}

	// Initialize the S3 client with path-style addressing (required for MinIO).
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})
	presignClient := s3.NewPresignClient(client)

	log.Println("S3-compatible storage client initialized successfully.")

	return &R2Client{
		S3Client:      client,
		PresignClient: presignClient,
		BucketName:    bucketName,
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
