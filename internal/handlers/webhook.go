// Package handlers/webhook.go contains the Stripe webhook handler.
// This endpoint is PUBLIC (no JWT auth) — security is via Stripe signature verification.
//
// WEBHOOK FLOW:
// 1. Stripe sends POST /api/v1/webhooks/stripe with signed payload
// 2. Handler verifies signature using the webhook secret
// 3. Parses event type and dispatches:
//   - payment_intent.succeeded → fulfil order (create Purchases, clear cart)
//   - payment_intent.payment_failed → mark order as failed
//   - payment_intent.canceled → mark order as failed/canceled
//   - Other events → acknowledge with 200 OK
//
// IDEMPOTENCY:
// The handler checks order state before transitioning. If the order is
// already in the target state (e.g., already fulfilled), it acknowledges
// silently without re-processing.
//
// ERROR HANDLING:
// Returns 500 on transient failures so Stripe will retry the webhook.
// Returns 200 on permanent/expected conditions (already processed, unsupported events).
package handlers

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
	"github.com/Geetur/Notery/internal/payment"
)

// webhookLog is the domain-specific logger for webhook operations.
var webhookLog = helpers.WebhookLog

// maxWebhookBodySize is the maximum allowed body size for webhook requests (64 KB).
const maxWebhookBodySize = 65536

// HandleStripeWebhook processes incoming Stripe webhook events.
// This endpoint is PUBLIC (no JWT auth). Security is enforced via Stripe-Signature verification.
//
// Route: POST /api/v1/webhooks/stripe
func (app *App) HandleStripeWebhook(c *gin.Context) {
	webhookLog.Log("RECEIVE", "webhook received")

	// Guard: payment provider must be configured
	if app.Payment == nil {
		webhookLog.Log("RECEIVE", "payment provider not configured")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Payment provider not configured"})
		return
	}

	// Read raw body (required for signature verification — must be read before any binding)
	payload, err := io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookBodySize))
	if err != nil {
		webhookLog.Log("RECEIVE", "failed to read body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// Verify webhook signature
	signature := c.GetHeader("Stripe-Signature")
	if signature == "" {
		webhookLog.Log("RECEIVE", "missing Stripe-Signature header")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing Stripe-Signature header"})
		return
	}

	event, err := app.Payment.VerifyWebhookSignature(payload, signature)
	if err != nil {
		if errors.Is(err, payment.ErrUnsupportedEvent) {
			// Acknowledge events we don't care about so Stripe stops retrying
			webhookLog.Log("RECEIVE", "unsupported event type (acknowledged)")
			c.JSON(http.StatusOK, gin.H{"received": true})
			return
		}
		webhookLog.Log("RECEIVE", "signature verification failed", "error", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid webhook signature"})
		return
	}

	webhookLog.Log("PROCESS", "verified event", "type", event.Type, "payment_intent_id", event.PaymentIntentID)

	switch event.Type {
	case payment.EventPaymentSucceeded:
		app.handlePaymentSucceeded(c, event)
	case payment.EventPaymentFailed:
		app.handlePaymentFailed(c, event)
	case payment.EventPaymentCanceled:
		app.handlePaymentCanceled(c, event)
	default:
		c.JSON(http.StatusOK, gin.H{"received": true})
	}
}

// handlePaymentSucceeded processes a successful payment.
// Transitions the Order from pending → paid → fulfilled and creates Purchase records.
func (app *App) handlePaymentSucceeded(c *gin.Context, event *payment.WebhookEvent) {
	webhookLog.Log("SUCCESS", "processing payment success", "pi_id", event.PaymentIntentID)

	// Find order by PaymentIntentID
	var order models.Order
	if err := app.DB.Where("payment_intent_id = ?", event.PaymentIntentID).
		Preload("Items").First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			webhookLog.Log("SUCCESS", "order not found for payment intent", "pi_id", event.PaymentIntentID)
			// Return 200 to prevent retries — this PI doesn't belong to us or was cleaned up
			c.JSON(http.StatusOK, gin.H{"received": true, "error": "order not found"})
			return
		}
		webhookLog.Log("SUCCESS", "db error looking up order", "pi_id", event.PaymentIntentID, "error", err)
		// Return 500 so Stripe retries
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Idempotency: if order is already paid or fulfilled, acknowledge silently
	if order.Status == models.OrderPaid || order.Status == models.OrderFulfilled {
		webhookLog.Log("SUCCESS", "order already processed (idempotent)", "order_id", order.ID, "status", order.Status)
		c.JSON(http.StatusOK, gin.H{"received": true, "already_processed": true})
		return
	}

	// Only pending orders can transition to paid
	if order.Status != models.OrderPending {
		webhookLog.Log("SUCCESS", "order in unexpected state", "order_id", order.ID, "status", order.Status)
		c.JSON(http.StatusOK, gin.H{"received": true, "error": "order not in pending state"})
		return
	}

	// Verify payment amount and currency match the order (defense against misconfiguration/bugs).
	// Return 200 so Stripe doesn't endlessly retry with a permanently wrong amount.
	if event.AmountCents != order.TotalCents {
		webhookLog.Log("SUCCESS", "CRITICAL: amount mismatch — order NOT fulfilled",
			"order_id", order.ID, "expected_cents", order.TotalCents,
			"webhook_cents", event.AmountCents, "currency", event.Currency)
		c.JSON(http.StatusOK, gin.H{"received": true, "error": "amount mismatch — manual review required"})
		return
	}
	if event.Currency != order.Currency {
		webhookLog.Log("SUCCESS", "CRITICAL: currency mismatch — order NOT fulfilled",
			"order_id", order.ID, "expected_currency", order.Currency,
			"webhook_currency", event.Currency)
		c.JSON(http.StatusOK, gin.H{"received": true, "error": "currency mismatch — manual review required"})
		return
	}

	// Fulfil the order
	if err := app.fulfilOrder(&order); err != nil {
		webhookLog.Log("SUCCESS", "fulfilment failed", "order_id", order.ID, "error", err)
		// Return 500 so Stripe retries
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Fulfilment failed"})
		return
	}

	// Clear cart items (best-effort)
	ctx := c.Request.Context()
	app.clearCartItems(ctx, order.UserID, order.Items)

	webhookLog.Log("SUCCESS", "order fulfilled", "order_id", order.ID, "items", len(order.Items), "total_cents", order.TotalCents)
	c.JSON(http.StatusOK, gin.H{"received": true, "order_id": order.ID, "status": "fulfilled"})
}

// handlePaymentFailed processes a failed payment.
// Transitions the Order to failed state with the failure reason.
func (app *App) handlePaymentFailed(c *gin.Context, event *payment.WebhookEvent) {
	webhookLog.Log("FAILED", "processing payment failure", "pi_id", event.PaymentIntentID,
		"reason", event.FailureMessage, "amount_cents", event.AmountCents, "currency", event.Currency)

	var order models.Order
	if err := app.DB.Where("payment_intent_id = ?", event.PaymentIntentID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			webhookLog.Log("FAILED", "order not found", "pi_id", event.PaymentIntentID)
			c.JSON(http.StatusOK, gin.H{"received": true, "error": "order not found"})
			return
		}
		webhookLog.Log("FAILED", "db error", "pi_id", event.PaymentIntentID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Log amount mismatch for investigation (don't block failure processing — we're not granting access)
	if event.AmountCents != order.TotalCents {
		webhookLog.Log("FAILED", "WARNING: amount mismatch on failed payment",
			"order_id", order.ID, "expected_cents", order.TotalCents, "webhook_cents", event.AmountCents)
	}

	// Idempotency: if already failed, acknowledge
	if order.Status == models.OrderFailed {
		webhookLog.Log("FAILED", "already failed (idempotent)", "order_id", order.ID)
		c.JSON(http.StatusOK, gin.H{"received": true, "already_processed": true})
		return
	}

	// Don't fail orders that are already fulfilled — fulfillment wins
	if order.Status == models.OrderFulfilled || order.Status == models.OrderPaid {
		webhookLog.Log("FAILED", "order already processed, ignoring failure", "order_id", order.ID, "status", order.Status)
		c.JSON(http.StatusOK, gin.H{"received": true, "ignored": true})
		return
	}

	now := time.Now()
	failureReason := event.FailureMessage
	if len(failureReason) > 512 {
		failureReason = failureReason[:512]
	}

	if err := app.DB.Model(&order).Updates(map[string]interface{}{
		"status":         models.OrderFailed,
		"failed_at":      now,
		"failure_reason": failureReason,
	}).Error; err != nil {
		webhookLog.Log("FAILED", "failed to update order", "order_id", order.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update order"})
		return
	}

	webhookLog.Log("FAILED", "order marked as failed", "order_id", order.ID, "reason", failureReason)
	c.JSON(http.StatusOK, gin.H{"received": true, "order_id": order.ID, "status": "failed"})
}

// handlePaymentCanceled processes a canceled payment intent.
// Transitions the Order to failed state. This prevents orders from being stuck
// in pending when the PaymentIntent is canceled via Stripe Dashboard or API.
func (app *App) handlePaymentCanceled(c *gin.Context, event *payment.WebhookEvent) {
	webhookLog.Log("CANCELED", "processing payment cancellation", "pi_id", event.PaymentIntentID,
		"reason", event.FailureMessage)

	var order models.Order
	if err := app.DB.Where("payment_intent_id = ?", event.PaymentIntentID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			webhookLog.Log("CANCELED", "order not found", "pi_id", event.PaymentIntentID)
			c.JSON(http.StatusOK, gin.H{"received": true, "error": "order not found"})
			return
		}
		webhookLog.Log("CANCELED", "db error", "pi_id", event.PaymentIntentID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Idempotency: if already failed, acknowledge
	if order.Status == models.OrderFailed {
		webhookLog.Log("CANCELED", "already failed (idempotent)", "order_id", order.ID)
		c.JSON(http.StatusOK, gin.H{"received": true, "already_processed": true})
		return
	}

	// Don't cancel orders that are already fulfilled — fulfillment wins
	if order.Status == models.OrderFulfilled || order.Status == models.OrderPaid {
		webhookLog.Log("CANCELED", "order already processed, ignoring cancellation", "order_id", order.ID, "status", order.Status)
		c.JSON(http.StatusOK, gin.H{"received": true, "ignored": true})
		return
	}

	now := time.Now()
	failureReason := event.FailureMessage
	if len(failureReason) > 512 {
		failureReason = failureReason[:512]
	}

	if err := app.DB.Model(&order).Updates(map[string]interface{}{
		"status":         models.OrderFailed,
		"failed_at":      now,
		"failure_reason": failureReason,
	}).Error; err != nil {
		webhookLog.Log("CANCELED", "failed to update order", "order_id", order.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update order"})
		return
	}

	webhookLog.Log("CANCELED", "order marked as failed (canceled)", "order_id", order.ID, "reason", failureReason)
	c.JSON(http.StatusOK, gin.H{"received": true, "order_id": order.ID, "status": "failed"})
}
