// payout.go — PayoutRecord model: tracks per-item revenue split between Notery and creators.
//
// DESIGN:
//
//	Every paid note purchase generates a PayoutRecord to track the fee breakdown:
//	  - Flat fee: 25¢ per transaction
//	  - Marketplace fee: 15% of gross price
//	  - Creator payout: gross - flat fee - marketplace fee
//
//	If the creator has not connected a Stripe account (PayoutEnabled=false on User),
//	the record is created with Status="retained" and Notery keeps the full amount.
//	If connected, a Stripe Transfer is made to their Connected Account.
//
// STATUSES:
//   - pending:    Transfer initiated but not confirmed
//   - completed:  Transfer succeeded to creator's Stripe account
//   - failed:     Transfer failed (retry or manual resolution)
//   - retained:   Creator has no payout account; Notery keeps the funds
package models

import "time"

// PayoutStatus tracks the state of a creator payout.
type PayoutStatus string

const (
	PayoutPending   PayoutStatus = "pending"
	PayoutCompleted PayoutStatus = "completed"
	PayoutFailed    PayoutStatus = "failed"
	PayoutRetained  PayoutStatus = "retained"
)

// Fee constants for the Notery marketplace.
const (
	// FlatFeeCents is the per-transaction flat fee in cents (25¢).
	FlatFeeCents = 25
	// MarketplaceFeePercent is the marketplace fee percentage (15%).
	MarketplaceFeePercent = 15
)

// CalculatePayoutSplit computes the fee breakdown for a purchase.
// Returns (flatFee, marketplaceFee, creatorPayout) all in cents.
// If grossCents is 0 (free note), all values are 0.
func CalculatePayoutSplit(grossCents int64) (flatFee, marketplaceFee, creatorPayout int64) {
	if grossCents <= 0 {
		return 0, 0, 0
	}
	flatFee = FlatFeeCents
	marketplaceFee = grossCents * MarketplaceFeePercent / 100
	creatorPayout = grossCents - flatFee - marketplaceFee
	if creatorPayout < 0 {
		// Note price too low to cover fees — Notery absorbs the loss
		creatorPayout = 0
		// Notery gets what's left after flat fee
		marketplaceFee = grossCents - flatFee
		if marketplaceFee < 0 {
			marketplaceFee = 0
			flatFee = grossCents
		}
	}
	return flatFee, marketplaceFee, creatorPayout
}

// PayoutRecord tracks the revenue split for a single purchased note.
//
// Fields:
//   - OrderID, NoteID:          Links to the order and specific note
//   - CreatorID, BuyerID:       Seller and buyer
//   - GrossCents:               Full note price paid by buyer
//   - FlatFeeCents:             25¢ flat fee
//   - MarketplaceFeeCents:      15% of gross
//   - CreatorPayoutCents:       Amount transferred to creator (gross - fees)
//   - StripeTransferID:         Stripe Transfer object ID (empty if retained)
//   - Status:                   pending / completed / failed / retained
type PayoutRecord struct {
	ID                  uint         `json:"id" gorm:"primaryKey"`
	OrderID             uint         `json:"order_id" gorm:"index;not null"`
	NoteID              uint         `json:"note_id" gorm:"index;not null"`
	CreatorID           uint64       `json:"creator_id" gorm:"index;not null"`
	BuyerID             uint64       `json:"buyer_id" gorm:"not null"`
	GrossCents          int64        `json:"gross_cents" gorm:"not null"`
	FlatFeeCents        int64        `json:"flat_fee_cents" gorm:"not null"`
	MarketplaceFeeCents int64        `json:"marketplace_fee_cents" gorm:"not null"`
	CreatorPayoutCents  int64        `json:"creator_payout_cents" gorm:"not null"`
	StripeTransferID    string       `json:"stripe_transfer_id" gorm:"type:varchar(255);default:''"`
	Status              PayoutStatus `json:"status" gorm:"type:varchar(20);not null;default:'pending';index"`
	CreatedAt           time.Time    `json:"created_at"`
}
