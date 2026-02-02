// Package models/note.go contains the Note model definition
package models

import "time"

// Note represents a note entity in the system.
// A note is a piece of content (PDF) that users can purchase and view.
//
// LIFECYCLE:
// 1. User creates note with metadata + uploads PDF
// 2. Note starts in "Pending" status, PDF stored in R2
// 3. Admin reviews note (can view PDF) and approves/rejects
// 4. Approved notes are searchable and purchasable
// 5. Users who purchase can view the PDF in-app
//
// STATUS VALUES:
// - "Pending": Awaiting admin approval
// - "Approved": Live and purchasable
// - "Rejected": Declined by admin (note and PDF deleted)
type Note struct {
	// ID is the primary key for all intents and purposes
	ID uint `json:"id" gorm:"primaryKey"`

	// CreatorID links this note to the user who created it
	// This allows creators to always view their own notes
	CreatorID uint64 `json:"creator_id" gorm:"index;not null"`

	// Title and Author are the main queryable fields for search
	Title  string `json:"title" gorm:"index"`
	Author string `json:"author" gorm:"index"`

	// Status tracks the approval state: "Pending", "Approved", or "Rejected"
	Status string `json:"status" gorm:"index"`

	// SubnoteryID links this note to its parent community
	SubnoteryID uint `json:"subnotery_id" gorm:"index;not null"`

	// Price is what users pay to access this note (in cents for precision)
	Price float64 `json:"price"`

	// ----- PDF Content Fields -----
	// HasPDF indicates whether a PDF has been uploaded for this note.
	// A note cannot be approved without a PDF.
	HasPDF bool `json:"has_pdf" gorm:"default:false"`

	// PDFSize stores the size of the PDF in bytes (for display purposes)
	PDFSize int64 `json:"pdf_size" gorm:"default:0"`

	// PDFUploadedAt tracks when the PDF was last uploaded/updated
	PDFUploadedAt *time.Time `json:"pdf_uploaded_at"`

	// ----- Voting/Hotness Fields -----
	// Upvotes and Downvotes track community engagement
	Upvotes   uint64 `json:"upvotes" gorm:"default:0"`
	Downvotes uint64 `json:"downvotes" gorm:"default:0"`

	// Hotness is the calculated Reddit-style hot score
	Hotness float64 `json:"hotness" gorm:"index"`

	// ----- Timestamps -----
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
