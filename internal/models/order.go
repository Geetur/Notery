// order.go — Order and OrderItem models with state machine transitions.
// Orders track a payment session; OrderItems are the individual notes being purchased.
// Purchase records are created only once an Order transitions to OrderPaid.
package models

import "time"

// OrderStatus represents the lifecycle state of an order.
type OrderStatus string

const (
	// OrderPending means the order has been created but payment has not been confirmed.
	OrderPending OrderStatus = "pending"
	// OrderPaid means payment was successfully processed.
	OrderPaid OrderStatus = "paid"
	// OrderFulfilled means all Purchase records have been created and the buyer has access.
	OrderFulfilled OrderStatus = "fulfilled"
	// OrderFailed means payment processing failed.
	OrderFailed OrderStatus = "failed"
	// OrderRefunded means the order was refunded after payment.
	OrderRefunded OrderStatus = "refunded"
)

// Order represents a single checkout session for one or more notes.
type Order struct {
	ID uint `json:"id" gorm:"primaryKey"`

	// UserID is the buyer.
	UserID uint64 `json:"user_id" gorm:"uniqueIndex:idx_order_user_idempotency;index;not null"`

	// Status tracks the order through its lifecycle.
	Status OrderStatus `json:"status" gorm:"index;not null;default:'pending'"`

	// TotalCents is the sum of all item prices at time of checkout, in cents.
	TotalCents int64 `json:"total_cents"`

	// Currency is the ISO 4217 currency code used for this order (e.g., "usd").
	// Stored at order creation to allow webhook/reconciliation verification.
	Currency string `json:"currency" gorm:"size:3;not null;default:'usd'"`

	// IdempotencyKey prevents duplicate orders from retried requests.
	// Clients should generate a UUID and include it with checkout requests.
	// Scoped per-user so two different users can never collide.
	IdempotencyKey string `json:"idempotency_key" gorm:"uniqueIndex:idx_order_user_idempotency;size:64"`

	// PaymentIntentID stores the external payment processor reference (e.g. Stripe pi_xxx).
	PaymentIntentID string `json:"payment_intent_id,omitempty" gorm:"index"`

	// ChargeID is the Stripe Charge ID from the successful payment.
	// Used as source_transaction when creating transfers to connected accounts,
	// ensuring Stripe holds the transfer until the charge's funds are available.
	ChargeID string `json:"charge_id,omitempty" gorm:"size:255"`

	// PaidAt records when payment was confirmed by the payment provider.
	PaidAt *time.Time `json:"paid_at,omitempty"`

	// FailedAt records when payment processing failed.
	FailedAt *time.Time `json:"failed_at,omitempty"`

	// FailureReason describes why payment failed (from payment provider or internal error).
	FailureReason string `json:"failure_reason,omitempty" gorm:"size:512"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Items are loaded via preload when needed.
	Items []OrderItem `json:"items,omitempty" gorm:"foreignKey:OrderID"`
}

// OrderItem is a single line item within an order.
type OrderItem struct {
	ID      uint   `json:"id" gorm:"primaryKey"`
	OrderID uint   `json:"order_id" gorm:"index;not null"`
	NoteID  uint   `json:"note_id" gorm:"index;not null"`
	// PriceCents records the note price at time of order creation.
	PriceCents int64 `json:"price_cents"`
}

// IsValidTransition returns true if the given order state transition is allowed.
// The valid state machine is:
//
//	pending   → paid | failed
//	paid      → fulfilled | refunded
//	fulfilled → refunded
//	failed    → (terminal)
//	refunded  → (terminal)
func IsValidTransition(from, to OrderStatus) bool {
	switch from {
	case OrderPending:
		return to == OrderPaid || to == OrderFailed
	case OrderPaid:
		return to == OrderFulfilled || to == OrderRefunded
	case OrderFulfilled:
		return to == OrderRefunded
	default:
		return false
	}
}
