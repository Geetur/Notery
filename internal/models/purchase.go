// Package models/purchase.go contains the Purchase model definition
package models

import "time"

// Purchase represents a user's purchase of a note.
// This is the key model that controls access to PDF content.
//
// DESIGN RATIONALE:
// -----------------
// Instead of giving users a downloadable file, we track their purchases.
// When a user wants to view a note's PDF, we check this table first.
// This allows:
// 1. Concurrent updates - if the note creator updates the PDF, buyers see the new version
// 2. No file sharing - users can't share a downloaded file
// 3. Access revocation - if needed, purchases can be invalidated
// 4. Analytics - we can track viewing patterns, most popular notes, etc.
//
// ACCESS CONTROL FLOW:
// --------------------
// 1. User requests to view a note's PDF
// 2. System checks if user has a Purchase record for that note
// 3. If yes, proxy the PDF content from R2 to the user
// 4. If no, return 403 Forbidden
//
// SPECIAL ACCESS:
// ---------------
// - Note creators can always view their own notes (handled in handler)
// - Admins of the note's subnotery can view pending notes (handled in handler)
// - Global admins can view all notes (handled in handler)
type Purchase struct {
	// ID is the primary key
	ID uint `json:"id" gorm:"primaryKey"`

	// UserID is the user who made the purchase
	UserID uint `json:"user_id" gorm:"index;not null"`

	// NoteID is the note that was purchased
	NoteID uint `json:"note_id" gorm:"index;not null"`

	// PricePaid records what the user paid at time of purchase
	// This is important because note prices can change over time
	PricePaid float64 `json:"price_paid"`

	// PurchasedAt records when the purchase was made
	PurchasedAt time.Time `json:"purchased_at"`

	// ----- Optional: Future Enhancement Fields -----
	// These are commented out but show potential expansion:

	// TransactionID could link to a payment processor transaction
	// TransactionID string `json:"transaction_id" gorm:"index"`

	// ExpiresAt could enable time-limited access (subscription model)
	// ExpiresAt *time.Time `json:"expires_at"`

	// Refunded could track if this purchase was refunded
	// Refunded bool `json:"refunded" gorm:"default:false"`
}

// UserNotePurchase is a unique constraint helper.
// We use a composite unique index on (user_id, note_id) to prevent duplicate purchases.
// This is defined in the migration rather than as a struct.

// TableName overrides the default table name for GORM
func (Purchase) TableName() string {
	return "purchases"
}
