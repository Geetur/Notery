// Package models/order.go contains the Order and OrderItem models.
// Orders track a payment session, while OrderItems are the individual notes being purchased.
// Purchase records are only created once an Order transitions to OrderPaid.
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

	// IdempotencyKey prevents duplicate orders from retried requests.
	// Clients should generate a UUID and include it with checkout requests.
	// Scoped per-user so two different users can never collide.
	IdempotencyKey string `json:"idempotency_key" gorm:"uniqueIndex:idx_order_user_idempotency;size:64"`

	// ----- Future Payment Integration -----
	// PaymentIntentID stores the external payment processor reference (e.g. Stripe pi_xxx).
	PaymentIntentID string `json:"payment_intent_id,omitempty" gorm:"index"`

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
