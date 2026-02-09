// Package payment provides an abstraction layer over payment providers.
// The current implementation uses Stripe, but the interface allows swapping
// providers without changing handler logic.
//
// ARCHITECTURE:
// -------------
// Handlers depend on the payment.Service interface, not on Stripe directly.
// This gives us:
//   - Testability: mock the interface in unit tests
//   - Flexibility: swap providers (Stripe → Adyen) by implementing Service
//   - Graceful degradation: when Service is nil, handlers auto-fulfil (dev mode)
package payment

import (
	"context"
	"fmt"
)

// Service defines the contract for payment processing operations.
// All payment provider implementations must satisfy this interface.
type Service interface {
	// CreatePaymentIntent creates a new payment intent for the given order.
	// Returns a result containing the client secret for frontend confirmation.
	CreatePaymentIntent(ctx context.Context, params CreateIntentParams) (*IntentResult, error)

	// RetrievePaymentIntent fetches an existing payment intent by its ID.
	// Used for idempotent retries when a pending order already has a PaymentIntentID.
	RetrievePaymentIntent(ctx context.Context, paymentIntentID string) (*IntentResult, error)

	// VerifyWebhookSignature validates an incoming webhook payload against its signature.
	// Returns the parsed event on success or an error if verification fails.
	VerifyWebhookSignature(payload []byte, signature string) (*WebhookEvent, error)
}

// CreateIntentParams holds the parameters for creating a payment intent.
type CreateIntentParams struct {
	// OrderID is the internal order ID (stored in PaymentIntent metadata for reconciliation).
	OrderID uint

	// AmountCents is the total charge amount in the smallest currency unit (e.g., cents for USD).
	AmountCents int64

	// Currency is the ISO 4217 currency code (e.g., "usd").
	Currency string

	// IdempotencyKey prevents duplicate charges on retried requests.
	// Passed through to the payment provider's idempotency mechanism.
	IdempotencyKey string

	// CustomerEmail is used for payment receipt emails (optional).
	CustomerEmail string

	// Metadata is additional key-value data attached to the PaymentIntent.
	Metadata map[string]string
}

// IntentResult is returned after creating or retrieving a payment intent.
type IntentResult struct {
	// PaymentIntentID is the provider's unique reference (e.g., Stripe pi_xxx).
	PaymentIntentID string

	// ClientSecret is passed to the frontend for Stripe.js payment confirmation.
	ClientSecret string

	// Status is the current status of the PaymentIntent (e.g., "requires_payment_method").
	Status string

	// AmountCents is the payment amount in the smallest currency unit (populated from provider).
	AmountCents int64

	// Currency is the ISO 4217 currency code (e.g., "usd") from the provider.
	Currency string
}

// WebhookEventType categorises webhook events the system handles.
type WebhookEventType string

const (
	// EventPaymentSucceeded indicates payment was successfully processed.
	EventPaymentSucceeded WebhookEventType = "payment_intent.succeeded"

	// EventPaymentFailed indicates a payment attempt failed.
	EventPaymentFailed WebhookEventType = "payment_intent.payment_failed"
)

// WebhookEvent is the normalised representation of a payment webhook event.
type WebhookEvent struct {
	// Type categorises the event.
	Type WebhookEventType

	// PaymentIntentID is the provider's reference for the payment.
	PaymentIntentID string

	// AmountCents is the payment amount in the smallest currency unit.
	AmountCents int64

	// Currency is the ISO 4217 currency code (e.g., "usd").
	Currency string

	// FailureMessage describes why payment failed (populated only for failed events).
	FailureMessage string
}

// ErrUnsupportedEvent is returned when the system receives a webhook event type it doesn't handle.
// The webhook handler should acknowledge these with 200 OK to prevent retries.
var ErrUnsupportedEvent = fmt.Errorf("unsupported webhook event type")
