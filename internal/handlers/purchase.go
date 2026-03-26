// purchase.go — HTTP handlers for the purchase flow (checkout, orders, history).
// This file manages note purchases (checkout), order lifecycle, and purchase history.
//
// PURCHASE FLOW (with Order state machine):
// ------------------------------------------
// 1. User adds approved notes to cart (cart handler)
// 2. User initiates checkout with an idempotency key
// 3. System creates an Order (pending) with OrderItems
// 4. System validates all items (approved, has PDF, not already purchased)
// 5a. If payment provider configured: creates Stripe PaymentIntent, returns client_secret
//
//	→ Webhook confirms payment → transitions Order to "paid" → creates Purchases
//
// 5b. If no payment provider (dev mode) or free order: auto-fulfils immediately
// 6. Cart is cleared after fulfilment
//
// RECONCILIATION:
// ---------------
// If a webhook is delayed, users can call POST /orders/:order_id/confirm
// to manually check the PaymentIntent status and trigger fulfilment.
package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
	"github.com/Geetur/Notery/internal/payment"
)

// purchaseLog is the shared logger for purchase operations
var purchaseLog = helpers.PurchaseLog

// CheckoutCart processes the user's cart through the Order state machine.
//
// When a payment provider is configured (Stripe), this creates a PaymentIntent
// and returns the client_secret for the frontend to confirm payment via Stripe.js.
// Fulfilment happens asynchronously when the webhook confirms payment.
//
// When no payment provider is configured (development) or the total is $0,
// the order is auto-fulfilled immediately.
//
// Request body (optional): { "idempotency_key": "uuid-here" }
//
// Route: POST /api/v1/checkout
func (app *App) CheckoutCart(c *gin.Context) {
	start := time.Now()
	ctx := c.Request.Context()

	userID := helpers.GetUserID(c)
	purchaseLog.Log("CHECKOUT", "initiated", "user_id", userID)

	// Accept optional idempotency key
	var body struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	_ = c.ShouldBindJSON(&body)

	// If idempotency key provided, check for existing order
	if body.IdempotencyKey != "" {
		var existing models.Order
		if err := app.DB.Where("idempotency_key = ? AND user_id = ?", body.IdempotencyKey, userID).
			Preload("Items").First(&existing).Error; err == nil {
			purchaseLog.Log("CHECKOUT", "idempotent hit", "user_id", userID, "order_id", existing.ID, "status", existing.Status)

			// Non-pending states: return final status immediately
			if existing.Status != models.OrderPending {
				c.JSON(http.StatusOK, gin.H{
					"order_id":    existing.ID,
					"status":      existing.Status,
					"total_cents": existing.TotalCents,
					"idempotent":  true,
				})
				return
			}

			// Pending with PaymentIntent: re-fetch client_secret
			if existing.PaymentIntentID != "" && app.Payment != nil {
				pi, piErr := app.Payment.RetrievePaymentIntent(ctx, existing.PaymentIntentID)
				if piErr == nil {
					c.JSON(http.StatusOK, gin.H{
						"order_id":          existing.ID,
						"status":            existing.Status,
						"total_cents":       existing.TotalCents,
						"client_secret":     pi.ClientSecret,
						"payment_intent_id": pi.PaymentIntentID,
						"idempotent":        true,
					})
					return
				}
				purchaseLog.Log("CHECKOUT", "PI retrieval failed, retrying creation", "order_id", existing.ID, "error", piErr)
			}

			// Pending without PI (or PI retrieval failed): retry payment for existing order.
			// This handles the case where a previous PI creation had a transient Stripe failure.
			if app.Payment != nil && existing.TotalCents > 0 {
				piResult, piErr := app.Payment.CreatePaymentIntent(ctx, payment.CreateIntentParams{
					OrderID:        existing.ID,
					AmountCents:    existing.TotalCents,
					Currency:       existing.Currency,
					IdempotencyKey: body.IdempotencyKey,
					Metadata: map[string]string{
						"order_id": strconv.FormatUint(uint64(existing.ID), 10),
						"user_id":  strconv.FormatUint(userID, 10),
					},
				})
				if piErr != nil {
					purchaseLog.Log("CHECKOUT", "PI retry failed", "order_id", existing.ID, "error", piErr)
					c.JSON(http.StatusBadGateway, gin.H{
						"error":     "Failed to initiate payment — please retry",
						"order_id":  existing.ID,
						"retryable": true,
					})
					return
				}
				app.DB.Model(&existing).Update("payment_intent_id", piResult.PaymentIntentID)
				c.JSON(http.StatusOK, gin.H{
					"order_id":          existing.ID,
					"status":            existing.Status,
					"total_cents":       existing.TotalCents,
					"client_secret":     piResult.ClientSecret,
					"payment_intent_id": piResult.PaymentIntentID,
					"idempotent":        true,
				})
				return
			}

			// Pending free order or no payment provider: retry auto-fulfil
			if err := app.fulfilOrder(&existing); err != nil {
				purchaseLog.Log("CHECKOUT", "auto-fulfil retry failed", "order_id", existing.ID, "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Checkout failed"})
				return
			}
			app.clearCartItems(ctx, userID, existing.Items)
			c.JSON(http.StatusOK, gin.H{
				"order_id":        existing.ID,
				"status":          existing.Status,
				"purchased_count": len(existing.Items),
				"total_cents":     existing.TotalCents,
				"idempotent":      true,
			})
			return
		}
	}

	// Fetch cart items from Redis
	cartKey := helpers.CartKey(userID)
	cartItems, err := app.RDB.SMembers(ctx, cartKey).Result()
	if err != nil {
		purchaseLog.Log("CHECKOUT", "redis error", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch cart"})
		return
	}

	if len(cartItems) == 0 {
		purchaseLog.Log("CHECKOUT", "empty cart", "user_id", userID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cart is empty"})
		return
	}

	purchaseLog.Log("CHECKOUT", "processing cart", "user_id", userID, "item_count", len(cartItems))

	// ---------- Phase 1: Validate & build order ----------
	var orderItems []models.OrderItem
	var warnings []string
	var totalCents int64

	for _, itemIDStr := range cartItems {
		noteID, parseErr := strconv.ParseUint(itemIDStr, 10, 64)
		if parseErr != nil {
			warnings = append(warnings, "Invalid item ID: "+itemIDStr)
			continue
		}

		var note models.Note
		if err := app.DB.First(&note, noteID).Error; err != nil {
			warnings = append(warnings, "Note not found: "+itemIDStr)
			continue
		}
		if note.Status != models.StatusApproved {
			warnings = append(warnings, "Note not available: "+note.Title)
			continue
		}
		if !note.HasPDF {
			warnings = append(warnings, "Note has no content: "+note.Title)
			continue
		}

		var existing models.Purchase
		if err := app.DB.Where("user_id = ? AND note_id = ?", userID, noteID).First(&existing).Error; err == nil {
			warnings = append(warnings, "Already purchased: "+note.Title)
			continue
		}

		orderItems = append(orderItems, models.OrderItem{
			NoteID:     note.ID,
			PriceCents: note.Price,
		})
		totalCents += note.Price
	}

	if len(orderItems) == 0 {
		purchaseLog.Log("CHECKOUT", "no valid items", "user_id", userID, "warnings", len(warnings))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "No items could be purchased",
			"warnings": warnings,
		})
		return
	}

	// ---------- Phase 2: Create order ----------
	var order models.Order

	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		order = models.Order{
			UserID:         userID,
			Status:         models.OrderPending,
			TotalCents:     totalCents,
			Currency:       "usd",
			IdempotencyKey: body.IdempotencyKey,
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}

		for i := range orderItems {
			orderItems[i].OrderID = order.ID
		}
		if err := tx.Create(&orderItems).Error; err != nil {
			return err
		}

		return nil
	}); err != nil {
		purchaseLog.Log("CHECKOUT", "order creation failed", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Checkout failed"})
		return
	}

	order.Items = orderItems

	// ---------- Phase 3: Payment ----------
	if totalCents == 0 || app.Payment == nil {
		// Free order or no payment provider configured: auto-fulfil immediately.
		reason := "free order"
		if app.Payment == nil {
			reason = "no payment provider (dev mode)"
		}
		purchaseLog.Log("CHECKOUT", "auto-fulfilling", "user_id", userID, "order_id", order.ID, "reason", reason)

		if err := app.fulfilOrder(&order); err != nil {
			purchaseLog.Log("CHECKOUT", "auto-fulfil failed", "user_id", userID, "order_id", order.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Checkout failed"})
			return
		}
		app.clearCartItems(ctx, userID, orderItems)

		response := gin.H{
			"order_id":        order.ID,
			"status":          order.Status,
			"purchased_count": len(orderItems),
			"total_cents":     totalCents,
		}
		if len(warnings) > 0 {
			response["warnings"] = warnings
		}

		duration := time.Since(start)
		purchaseLog.Log("CHECKOUT", "completed (auto-fulfilled)", "user_id", userID, "order_id", order.ID, "items", len(orderItems), "total_cents", totalCents, "duration_ms", duration.Milliseconds())
		c.JSON(http.StatusOK, response)
		return
	}

	// Payment provider configured: create PaymentIntent
	purchaseLog.Log("CHECKOUT", "creating payment intent", "user_id", userID, "order_id", order.ID, "total_cents", totalCents)

	piResult, err := app.Payment.CreatePaymentIntent(ctx, payment.CreateIntentParams{
		OrderID:        order.ID,
		AmountCents:    totalCents,
		Currency:       "usd",
		IdempotencyKey: body.IdempotencyKey,
		Metadata: map[string]string{
			"order_id": strconv.FormatUint(uint64(order.ID), 10),
			"user_id":  strconv.FormatUint(userID, 10),
		},
	})
	if err != nil {
		purchaseLog.Log("CHECKOUT", "payment intent failed (retryable)", "user_id", userID, "order_id", order.ID, "error", err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":     "Failed to initiate payment — please retry",
			"order_id":  order.ID,
			"retryable": true,
		})
		return
	}

	// Store PaymentIntentID on the order
	if err := app.DB.Model(&order).Update("payment_intent_id", piResult.PaymentIntentID).Error; err != nil {
		purchaseLog.Log("CHECKOUT", "failed to store payment intent ID", "user_id", userID, "order_id", order.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Checkout failed"})
		return
	}

	response := gin.H{
		"order_id":          order.ID,
		"status":            order.Status,
		"total_cents":       totalCents,
		"client_secret":     piResult.ClientSecret,
		"payment_intent_id": piResult.PaymentIntentID,
	}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}

	duration := time.Since(start)
	purchaseLog.Log("CHECKOUT", "payment intent created", "user_id", userID, "order_id", order.ID, "pi_id", piResult.PaymentIntentID, "duration_ms", duration.Milliseconds())
	c.JSON(http.StatusOK, response)
}

// PurchaseSingleNote handles direct purchase of a single note (not from cart).
// Creates a single-item Order and either creates a PaymentIntent or auto-fulfils.
//
// Request body (optional): { "idempotency_key": "uuid-here" }
//
// Route: POST /api/v1/notes/:id/purchase
func (app *App) PurchaseSingleNote(c *gin.Context) {
	ctx := c.Request.Context()
	userID := helpers.GetUserID(c)
	noteID, ok := helpers.MustParseNoteID(c)
	if !ok {
		return
	}

	purchaseLog.Log("SINGLE", "processing", "user_id", userID, "note_id", noteID)

	var body struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	_ = c.ShouldBindJSON(&body)

	// Idempotency check
	if body.IdempotencyKey != "" {
		var existing models.Order
		if err := app.DB.Where("idempotency_key = ? AND user_id = ?", body.IdempotencyKey, userID).
			Preload("Items").First(&existing).Error; err == nil {
			purchaseLog.Log("SINGLE", "idempotent hit", "user_id", userID, "order_id", existing.ID, "status", existing.Status)

			// Non-pending states: return final status
			if existing.Status != models.OrderPending {
				c.JSON(http.StatusOK, gin.H{
					"order_id":   existing.ID,
					"status":     existing.Status,
					"idempotent": true,
				})
				return
			}

			// Pending with PaymentIntent: re-fetch client_secret
			if existing.PaymentIntentID != "" && app.Payment != nil {
				pi, piErr := app.Payment.RetrievePaymentIntent(ctx, existing.PaymentIntentID)
				if piErr == nil {
					c.JSON(http.StatusOK, gin.H{
						"order_id":          existing.ID,
						"status":            existing.Status,
						"total_cents":       existing.TotalCents,
						"client_secret":     pi.ClientSecret,
						"payment_intent_id": pi.PaymentIntentID,
						"idempotent":        true,
					})
					return
				}
				purchaseLog.Log("SINGLE", "PI retrieval failed, retrying creation", "order_id", existing.ID, "error", piErr)
			}

			// Pending without PI: retry payment creation
			if app.Payment != nil && existing.TotalCents > 0 {
				piResult, piErr := app.Payment.CreatePaymentIntent(ctx, payment.CreateIntentParams{
					OrderID:        existing.ID,
					AmountCents:    existing.TotalCents,
					Currency:       existing.Currency,
					IdempotencyKey: body.IdempotencyKey,
					Metadata: map[string]string{
						"order_id": strconv.FormatUint(uint64(existing.ID), 10),
						"user_id":  strconv.FormatUint(userID, 10),
						"note_id":  strconv.FormatUint(noteID, 10),
					},
				})
				if piErr != nil {
					purchaseLog.Log("SINGLE", "PI retry failed", "order_id", existing.ID, "error", piErr)
					c.JSON(http.StatusBadGateway, gin.H{
						"error":     "Failed to initiate payment — please retry",
						"order_id":  existing.ID,
						"retryable": true,
					})
					return
				}
				app.DB.Model(&existing).Update("payment_intent_id", piResult.PaymentIntentID)
				c.JSON(http.StatusOK, gin.H{
					"order_id":          existing.ID,
					"status":            existing.Status,
					"total_cents":       existing.TotalCents,
					"client_secret":     piResult.ClientSecret,
					"payment_intent_id": piResult.PaymentIntentID,
					"idempotent":        true,
				})
				return
			}

			// Pending free order or no payment provider: retry auto-fulfil
			if err := app.fulfilOrder(&existing); err != nil {
				purchaseLog.Log("SINGLE", "auto-fulfil retry failed", "order_id", existing.ID, "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete purchase"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"message":      "Purchase successful",
				"order_id":     existing.ID,
				"status":       existing.Status,
				"purchased_at": time.Now(),
				"idempotent":   true,
			})
			return
		}
	}

	var note models.Note
	if err := app.DB.First(&note, noteID).Error; err != nil {
		purchaseLog.Log("SINGLE", "note not found", "note_id", noteID, "error", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
		return
	}

	if note.Status != models.StatusApproved {
		purchaseLog.Log("SINGLE", "not approved", "note_id", noteID, "status", note.Status)
		c.JSON(http.StatusForbidden, gin.H{"error": "This note is not available for purchase"})
		return
	}
	if !note.HasPDF {
		purchaseLog.Log("SINGLE", "no PDF", "note_id", noteID)
		c.JSON(http.StatusForbidden, gin.H{"error": "This note has no content"})
		return
	}

	// Check if already purchased
	var existingPurchase models.Purchase
	if err := app.DB.Where("user_id = ? AND note_id = ?", userID, noteID).First(&existingPurchase).Error; err == nil {
		purchaseLog.Log("SINGLE", "already purchased", "user_id", userID, "note_id", noteID)
		c.JSON(http.StatusConflict, gin.H{
			"error":        "You have already purchased this note",
			"purchased_at": existingPurchase.PurchasedAt,
		})
		return
	}

	// Create the order
	var order models.Order
	item := models.OrderItem{
		NoteID:     note.ID,
		PriceCents: note.Price,
	}

	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		order = models.Order{
			UserID:         userID,
			Status:         models.OrderPending,
			TotalCents:     note.Price,
			Currency:       "usd",
			IdempotencyKey: body.IdempotencyKey,
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}

		item.OrderID = order.ID
		if err := tx.Create(&item).Error; err != nil {
			return err
		}

		return nil
	}); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			purchaseLog.Log("SINGLE", "duplicate order prevented", "user_id", userID, "note_id", noteID)
			c.JSON(http.StatusConflict, gin.H{"error": "Duplicate order (use a new idempotency key to retry)"})
			return
		}
		purchaseLog.Log("SINGLE", "order creation failed", "user_id", userID, "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create order"})
		return
	}

	order.Items = []models.OrderItem{item}

	// Payment or auto-fulfil
	if note.Price == 0 || app.Payment == nil {
		reason := "free note"
		if app.Payment == nil {
			reason = "no payment provider (dev mode)"
		}
		purchaseLog.Log("SINGLE", "auto-fulfilling", "user_id", userID, "order_id", order.ID, "reason", reason)

		if err := app.fulfilOrder(&order); err != nil {
			purchaseLog.Log("SINGLE", "auto-fulfil failed", "user_id", userID, "order_id", order.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete purchase"})
			return
		}

		purchaseLog.Log("SINGLE", "success (auto-fulfilled)", "user_id", userID, "note_id", noteID, "order_id", order.ID, "price_cents", note.Price)
		c.JSON(http.StatusOK, gin.H{
			"message":      "Purchase successful",
			"order_id":     order.ID,
			"status":       order.Status,
			"note_id":      note.ID,
			"note_title":   note.Title,
			"price_paid":   note.Price,
			"purchased_at": time.Now(),
		})
		return
	}

	// Create PaymentIntent via Stripe
	purchaseLog.Log("SINGLE", "creating payment intent", "user_id", userID, "order_id", order.ID, "amount_cents", note.Price)

	piResult, err := app.Payment.CreatePaymentIntent(ctx, payment.CreateIntentParams{
		OrderID:        order.ID,
		AmountCents:    note.Price,
		Currency:       "usd",
		IdempotencyKey: body.IdempotencyKey,
		Metadata: map[string]string{
			"order_id": strconv.FormatUint(uint64(order.ID), 10),
			"user_id":  strconv.FormatUint(userID, 10),
			"note_id":  strconv.FormatUint(noteID, 10),
		},
	})
	if err != nil {
		purchaseLog.Log("SINGLE", "payment intent failed (retryable)", "user_id", userID, "order_id", order.ID, "error", err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":     "Failed to initiate payment — please retry",
			"order_id":  order.ID,
			"retryable": true,
		})
		return
	}

	if err := app.DB.Model(&order).Update("payment_intent_id", piResult.PaymentIntentID).Error; err != nil {
		purchaseLog.Log("SINGLE", "failed to store payment intent ID", "user_id", userID, "order_id", order.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Purchase failed"})
		return
	}

	purchaseLog.Log("SINGLE", "payment intent created", "user_id", userID, "note_id", noteID, "order_id", order.ID, "pi_id", piResult.PaymentIntentID)
	c.JSON(http.StatusOK, gin.H{
		"order_id":          order.ID,
		"status":            order.Status,
		"note_id":           note.ID,
		"total_cents":       note.Price,
		"client_secret":     piResult.ClientSecret,
		"payment_intent_id": piResult.PaymentIntentID,
	})
}

// CheckoutSelected processes a purchase for selected cart items (not all).
// This allows users to "Buy Selected" items from their cart.
//
// Request body: { "item_ids": ["1","2"], "idempotency_key": "uuid-here" }
//
// Route: POST /api/v1/checkout/selected
func (app *App) CheckoutSelected(c *gin.Context) {
	ctx := c.Request.Context()
	userID := helpers.GetUserID(c)
	start := time.Now()

	var body struct {
		ItemIDs        []string `json:"item_ids" binding:"required"`
		IdempotencyKey string   `json:"idempotency_key"`
	}
	if !helpers.BindJSON(c, &body) {
		return
	}
	if len(body.ItemIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No items selected"})
		return
	}

	purchaseLog.Log("CHECKOUT_SELECTED", "processing", "user_id", userID, "item_count", len(body.ItemIDs))

	// Idempotency check
	if body.IdempotencyKey != "" {
		var existing models.Order
		if err := app.DB.Where("idempotency_key = ? AND user_id = ?", body.IdempotencyKey, userID).
			Preload("Items").First(&existing).Error; err == nil {
			if existing.Status != models.OrderPending {
				c.JSON(http.StatusOK, gin.H{"order_id": existing.ID, "status": existing.Status, "idempotent": true})
				return
			}
		}
	}

	// Verify selected items are in cart
	if app.RDB != nil {
		cartKey := helpers.CartKey(userID)
		cartMembers, err := app.RDB.SMembers(ctx, cartKey).Result()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read cart"})
			return
		}
		cartSet := make(map[string]bool, len(cartMembers))
		for _, m := range cartMembers {
			cartSet[m] = true
		}
		for _, id := range body.ItemIDs {
			if !cartSet[id] {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Item " + id + " is not in your cart"})
				return
			}
		}
	}

	// Validate each item
	var orderItems []models.OrderItem
	var warnings []string
	var totalCents int64

	for _, idStr := range body.ItemIDs {
		noteID, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			warnings = append(warnings, "Invalid item ID: "+idStr)
			continue
		}
		var note models.Note
		if err := app.DB.First(&note, noteID).Error; err != nil {
			warnings = append(warnings, "Note "+idStr+" not found")
			continue
		}
		if note.Status != models.StatusApproved {
			warnings = append(warnings, "Note "+idStr+" is not approved")
			continue
		}
		if !note.HasPDF {
			warnings = append(warnings, "Note "+idStr+" has no PDF")
			continue
		}
		// Check not already purchased
		var count int64
		app.DB.Model(&models.Purchase{}).Where("user_id = ? AND note_id = ?", userID, noteID).Count(&count)
		if count > 0 {
			warnings = append(warnings, "Note "+idStr+" already purchased")
			continue
		}

		orderItems = append(orderItems, models.OrderItem{
			NoteID:     uint(noteID),
			PriceCents: note.Price,
		})
		totalCents += note.Price
	}

	if len(orderItems) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No valid items to purchase", "warnings": warnings})
		return
	}

	// Create order
	order := models.Order{
		UserID:         userID,
		Status:         models.OrderPending,
		TotalCents:     totalCents,
		Currency:       "usd",
		IdempotencyKey: body.IdempotencyKey,
	}
	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		for i := range orderItems {
			orderItems[i].OrderID = order.ID
		}
		return tx.Create(&orderItems).Error
	}); err != nil {
		purchaseLog.Log("CHECKOUT_SELECTED", "failed to create order", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create order"})
		return
	}
	order.Items = orderItems

	// Auto-fulfil or payment intent (same as CheckoutCart)
	if totalCents == 0 || app.Payment == nil {
		if err := app.fulfilOrder(&order); err != nil {
			purchaseLog.Log("CHECKOUT_SELECTED", "fulfilment failed", "order_id", order.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Fulfilment failed"})
			return
		}
		// Remove purchased items from cart
		if app.RDB != nil {
			cartKey := helpers.CartKey(userID)
			for _, id := range body.ItemIDs {
				app.RDB.SRem(ctx, cartKey, id)
			}
		}
		response := gin.H{
			"order_id":        order.ID,
			"status":          order.Status,
			"purchased_count": len(orderItems),
			"total_cents":     totalCents,
		}
		if len(warnings) > 0 {
			response["warnings"] = warnings
		}
		duration := time.Since(start)
		purchaseLog.Log("CHECKOUT_SELECTED", "completed (auto-fulfilled)", "order_id", order.ID, "items", len(orderItems), "duration_ms", duration.Milliseconds())
		c.JSON(http.StatusOK, response)
		return
	}

	// Create payment intent
	piResult, err := app.Payment.CreatePaymentIntent(ctx, payment.CreateIntentParams{
		OrderID:        order.ID,
		AmountCents:    totalCents,
		Currency:       "usd",
		IdempotencyKey: body.IdempotencyKey,
		Metadata: map[string]string{
			"order_id": strconv.FormatUint(uint64(order.ID), 10),
			"user_id":  strconv.FormatUint(userID, 10),
		},
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to initiate payment — please retry", "order_id": order.ID, "retryable": true})
		return
	}
	app.DB.Model(&order).Update("payment_intent_id", piResult.PaymentIntentID)

	response := gin.H{
		"order_id":          order.ID,
		"status":            order.Status,
		"total_cents":       totalCents,
		"client_secret":     piResult.ClientSecret,
		"payment_intent_id": piResult.PaymentIntentID,
	}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}
	c.JSON(http.StatusOK, response)
}

// CheckPurchaseStatus checks if the user has purchased a specific note.
//
// Useful for frontend to show "Buy" vs "View" button.
//
// Route: GET /api/v1/notes/:id/purchased
func (app *App) CheckPurchaseStatus(c *gin.Context) {
	userID := helpers.GetUserID(c)
	noteID, ok := helpers.MustParseNoteID(c)
	if !ok {
		return
	}

	var purchase models.Purchase
	if err := app.DB.Where("user_id = ? AND note_id = ?", userID, noteID).First(&purchase).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"purchased": false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"purchased":    true,
		"purchased_at": purchase.PurchasedAt,
		"price_paid":   purchase.PricePaid,
	})
}

// GetPurchaseHistory returns paginated purchase history for the user.
//
// Route: GET /api/v1/me/purchases/history
func (app *App) GetPurchaseHistory(c *gin.Context) {
	userID := helpers.GetUserID(c)

	// Parse pagination using helpers
	pag := helpers.ParsePaginationWithDefaults(c, 20)

	// Count total purchases
	var total int64
	app.DB.Model(&models.Purchase{}).Where("user_id = ?", userID).Count(&total)

	// Fetch purchases with note details
	type PurchaseWithNote struct {
		PurchaseID    uint      `json:"purchase_id"`
		NoteID        uint      `json:"note_id"`
		NoteTitle     string    `json:"note_title"`
		NoteAuthor    string    `json:"note_author"`
		PricePaid     int64     `json:"price_paid"`
		PurchasedAt   time.Time `json:"purchased_at"`
		HasPDF        bool      `json:"has_pdf"`
		SubnoteryID   uint      `json:"subnotery_id"`
		SubnoteryName string    `json:"subnotery_name"`
	}

	var purchases []PurchaseWithNote

	err := app.DB.Table("purchases").
		Select("purchases.id as purchase_id, purchases.note_id, notes.title as note_title, notes.author as note_author, purchases.price_paid, purchases.purchased_at, notes.has_pdf, notes.subnotery_id, subnoteries.name as subnotery_name").
		Joins("JOIN notes ON notes.id = purchases.note_id").
		Joins("LEFT JOIN subnoteries ON subnoteries.id = notes.subnotery_id").
		Where("purchases.user_id = ?", userID).
		Order("purchases.purchased_at DESC").
		Offset(pag.Offset).
		Limit(pag.Limit).
		Scan(&purchases).Error

	if err != nil {
		purchaseLog.Log("HISTORY", "db error", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch purchase history"})
		return
	}

	purchaseLog.Log("HISTORY", "fetched", "user_id", userID, "count", len(purchases), "total", total)

	// Ensure JSON [] (not null) for empty results.
	if purchases == nil {
		purchases = []PurchaseWithNote{}
	}

	c.JSON(http.StatusOK, gin.H{
		"purchases": purchases,
		"page":      pag.Page,
		"limit":     pag.Limit,
		"total":     total,
	})
}

// ----- SHARED ORDER LIFECYCLE HELPERS -----

// fulfilOrder transitions an order to fulfilled state, creating Purchase records.
// This is the shared fulfilment logic used by both auto-fulfil mode and webhook processing.
//
// CONCURRENCY SAFETY:
// Uses SELECT ... FOR UPDATE to lock the order row, preventing concurrent webhook
// deliveries from double-fulfilling. The status is re-checked under the lock.
//
// The method is idempotent: if the order is already fulfilled, it returns nil.
func (app *App) fulfilOrder(order *models.Order) error {
	// Fast path: already done (no lock needed)
	if order.Status == models.OrderFulfilled {
		return nil
	}

	// Can only fulfil from pending or paid state
	if order.Status != models.OrderPending && order.Status != models.OrderPaid {
		return fmt.Errorf("cannot fulfil order %d: invalid status %s", order.ID, order.Status)
	}

	// Ensure items are loaded
	if len(order.Items) == 0 {
		if err := app.DB.Where("order_id = ?", order.ID).Find(&order.Items).Error; err != nil {
			return fmt.Errorf("failed to load order items: %w", err)
		}
	}

	now := time.Now()
	return app.DB.Transaction(func(tx *gorm.DB) error {
		// Acquire row-level lock to prevent concurrent webhook races.
		// If two webhooks arrive simultaneously, the second blocks here
		// until the first commits, then sees the updated status and returns idempotently.
		var locked models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&locked, order.ID).Error; err != nil {
			return fmt.Errorf("failed to lock order %d: %w", order.ID, err)
		}

		// Re-check status under the lock — another goroutine may have already fulfilled.
		if locked.Status == models.OrderFulfilled {
			order.Status = models.OrderFulfilled
			return nil // idempotent
		}
		if locked.Status != models.OrderPending && locked.Status != models.OrderPaid {
			return fmt.Errorf("cannot fulfil order %d: status is %s (concurrent modification)", order.ID, locked.Status)
		}

		// Transition pending → paid if not already
		if locked.Status == models.OrderPending {
			if err := tx.Model(&locked).Updates(map[string]interface{}{
				"status":  models.OrderPaid,
				"paid_at": now,
			}).Error; err != nil {
				return err
			}
		}

		// Create Purchase records for each item
		for _, item := range order.Items {
			purchase := models.Purchase{
				UserID:      uint(order.UserID),
				NoteID:      item.NoteID,
				OrderID:     order.ID,
				PricePaid:   item.PriceCents,
				PurchasedAt: now,
			}
			if err := tx.Create(&purchase).Error; err != nil {
				return err
			}
		}

		// Transition to fulfilled
		if err := tx.Model(&locked).Update("status", models.OrderFulfilled).Error; err != nil {
			return err
		}

		order.Status = models.OrderFulfilled

		// Send purchase notifications to note owners (async, best-effort)
		for _, item := range order.Items {
			var note models.Note
			if err := tx.Select("id", "title", "creator_id").First(&note, item.NoteID).Error; err == nil {
				buyerName := ""
				var buyer models.User
				if err := tx.Select("username").First(&buyer, order.UserID).Error; err == nil {
					buyerName = buyer.Username
				}
				noteID := item.NoteID
				noteTitle := note.Title
				creatorID := note.CreatorID
				buyerID := order.UserID
				go app.SendPurchaseNotification(creatorID, buyerID, noteID, noteTitle, buyerName)
			}
		}

		// Process creator payouts (async, best-effort — records created in DB for audit)
		for _, item := range order.Items {
			if item.PriceCents <= 0 {
				continue // free notes: no payout
			}
			var note models.Note
			if err := tx.Select("id", "creator_id").First(&note, item.NoteID).Error; err != nil {
				continue
			}
			flatFee, mktFee, creatorAmt := models.CalculatePayoutSplit(item.PriceCents)
			record := models.PayoutRecord{
				OrderID:             order.ID,
				NoteID:              item.NoteID,
				CreatorID:           note.CreatorID,
				BuyerID:             order.UserID,
				GrossCents:          item.PriceCents,
				FlatFeeCents:        flatFee,
				MarketplaceFeeCents: mktFee,
				CreatorPayoutCents:  creatorAmt,
				Status:              models.PayoutRetained, // default: Notery keeps
			}

			// Check if creator has payout enabled
			var creator models.User
			if err := tx.Select("id", "stripe_account_id", "payout_enabled").
				First(&creator, note.CreatorID).Error; err == nil &&
				creator.PayoutEnabled && creator.StripeAccountID != "" && creatorAmt > 0 {
				record.Status = models.PayoutPending
			}

			if err := tx.Create(&record).Error; err != nil {
				purchaseLog.Log("PAYOUT", "Failed to create payout record", "noteID", item.NoteID, "error", err)
			}

			// Execute transfer asynchronously if creator is eligible
			if record.Status == models.PayoutPending && app.Payment != nil {
				recordID := record.ID
				acctID := creator.StripeAccountID
				amt := creatorAmt
				group := fmt.Sprintf("order_%d", order.ID)
				srcTxn := order.ChargeID
				go app.executePayoutTransfer(recordID, amt, "usd", acctID, group, srcTxn)
			}
		}

		return nil
	})
}

// clearCartItems removes the specified order items from a user's cart in Redis (best-effort).
func (app *App) clearCartItems(ctx context.Context, userID uint64, items []models.OrderItem) {
	if app.RDB == nil {
		return
	}
	cartKey := helpers.CartKey(userID)
	for _, item := range items {
		app.RDB.SRem(ctx, cartKey, strconv.FormatUint(uint64(item.NoteID), 10))
	}
}

// executePayoutTransfer runs a Stripe Transfer for a payout record and updates its status.
// This is called asynchronously from fulfilOrder.
// sourceTransaction is the Charge ID from the buyer's payment; Stripe holds the
// transfer until the charge's funds are available.
func (app *App) executePayoutTransfer(recordID uint, amountCents int64, currency, destAccountID, transferGroup, sourceTransaction string) {
	ctx := context.Background()
	transferID, err := app.Payment.CreateTransfer(ctx, amountCents, currency, destAccountID, transferGroup, sourceTransaction)
	if err != nil {
		purchaseLog.Log("PAYOUT", "Transfer failed", "recordID", recordID, "error", err)
		app.DB.Model(&models.PayoutRecord{}).Where("id = ?", recordID).
			Update("status", models.PayoutFailed)
		return
	}
	app.DB.Model(&models.PayoutRecord{}).Where("id = ?", recordID).
		Updates(map[string]interface{}{
			"stripe_transfer_id": transferID,
			"status":             models.PayoutCompleted,
		})
	purchaseLog.Log("PAYOUT", "Transfer completed", "recordID", recordID, "transferID", transferID)
}

// ----- ORDER STATUS & RECONCILIATION ENDPOINTS -----

// GetOrderStatus returns the current status of an order.
// The frontend polls this after payment to know when fulfillment is complete.
//
// Route: GET /api/v1/orders/:order_id
func (app *App) GetOrderStatus(c *gin.Context) {
	userID := helpers.GetUserID(c)

	orderID, err := strconv.ParseUint(c.Param("order_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID"})
		return
	}

	var order models.Order
	if err := app.DB.Preload("Items").First(&order, orderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch order"})
		return
	}

	// Users can only view their own orders
	if order.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
		return
	}

	response := gin.H{
		"order_id":    order.ID,
		"status":      order.Status,
		"total_cents": order.TotalCents,
		"items":       order.Items,
		"created_at":  order.CreatedAt,
	}
	if order.PaidAt != nil {
		response["paid_at"] = order.PaidAt
	}
	if order.FailedAt != nil {
		response["failed_at"] = order.FailedAt
		response["failure_reason"] = "Payment failed"
	}

	purchaseLog.Log("ORDER_STATUS", "fetched", "user_id", userID, "order_id", order.ID, "status", order.Status)
	c.JSON(http.StatusOK, response)
}

// ConfirmOrder checks the payment status with Stripe and reconciles if needed.
// Use this when the webhook hasn't arrived yet but the user believes they've paid.
//
// Route: POST /api/v1/orders/:order_id/confirm
func (app *App) ConfirmOrder(c *gin.Context) {
	if app.Payment == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payment provider not configured"})
		return
	}

	ctx := c.Request.Context()
	userID := helpers.GetUserID(c)

	orderID, err := strconv.ParseUint(c.Param("order_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID"})
		return
	}

	var order models.Order
	if err := app.DB.Preload("Items").First(&order, orderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch order"})
		return
	}

	// Users can only reconcile their own orders
	if order.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
		return
	}

	// Already processed
	if order.Status == models.OrderFulfilled || order.Status == models.OrderPaid {
		purchaseLog.Log("CONFIRM", "already processed", "order_id", order.ID, "status", order.Status)
		c.JSON(http.StatusOK, gin.H{"order_id": order.ID, "status": order.Status, "already_processed": true})
		return
	}

	if order.Status != models.OrderPending || order.PaymentIntentID == "" {
		purchaseLog.Log("CONFIRM", "cannot confirm", "order_id", order.ID, "status", order.Status, "has_pi", order.PaymentIntentID != "")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Order cannot be confirmed in its current state"})
		return
	}

	// Check with Stripe
	purchaseLog.Log("CONFIRM", "checking payment status", "order_id", order.ID, "pi_id", order.PaymentIntentID)

	result, err := app.Payment.RetrievePaymentIntent(ctx, order.PaymentIntentID)
	if err != nil {
		purchaseLog.Log("CONFIRM", "stripe retrieval failed", "order_id", order.ID, "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to check payment status"})
		return
	}

	purchaseLog.Log("CONFIRM", "stripe status", "order_id", order.ID, "payment_status", result.Status)

	switch result.Status {
	case "succeeded":
		// Verify amount matches order (defense against misconfiguration/bugs)
		if result.AmountCents != order.TotalCents {
			purchaseLog.Log("CONFIRM", "CRITICAL: amount mismatch",
				"order_id", order.ID, "expected_cents", order.TotalCents, "stripe_cents", result.AmountCents)
			c.JSON(http.StatusConflict, gin.H{"error": "Payment amount does not match order total — contact support"})
			return
		}
		if result.Currency != order.Currency {
			purchaseLog.Log("CONFIRM", "CRITICAL: currency mismatch",
				"order_id", order.ID, "expected_currency", order.Currency, "stripe_currency", result.Currency)
			c.JSON(http.StatusConflict, gin.H{"error": "Payment currency does not match order — contact support"})
			return
		}

		// Payment confirmed — fulfil order
		if result.ChargeID != "" {
			app.DB.Model(&order).Update("charge_id", result.ChargeID)
			order.ChargeID = result.ChargeID
		}
		if err := app.fulfilOrder(&order); err != nil {
			purchaseLog.Log("CONFIRM", "fulfilment failed", "order_id", order.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Fulfilment failed"})
			return
		}
		app.clearCartItems(ctx, userID, order.Items)
		purchaseLog.Log("CONFIRM", "reconciled and fulfilled", "order_id", order.ID)
		c.JSON(http.StatusOK, gin.H{"order_id": order.ID, "status": "fulfilled", "reconciled": true})

	case "canceled":
		now := time.Now()
		app.DB.Model(&order).Updates(map[string]interface{}{
			"status":         models.OrderFailed,
			"failed_at":      now,
			"failure_reason": "Payment was canceled",
		})
		purchaseLog.Log("CONFIRM", "payment was canceled", "order_id", order.ID)
		c.JSON(http.StatusOK, gin.H{"order_id": order.ID, "status": "failed", "reason": "Payment was canceled"})

	default:
		// Still processing (requires_payment_method, requires_confirmation, requires_action, processing)
		c.JSON(http.StatusOK, gin.H{"order_id": order.ID, "status": "pending", "payment_status": result.Status})
	}
}

// ----- GET /me/purchases -----

// GetMyPurchases returns all notes purchased by the authenticated user.
// Used to populate the "My Purchased Notes" section in the account view.
//
// Response: { "purchases": [ { note fields + price_paid, purchased_at } ] }
//
// Route: GET /api/v1/me/purchases
func (app *App) GetMyPurchases(c *gin.Context) {
	userID := helpers.GetUserID(c)
	purchaseLog.Log("MY_PURCHASES", "fetching", "user_id", userID)

	type PurchasedNote struct {
		models.Note
		PricePaid   int64     `json:"price_paid"`
		PurchasedAt time.Time `json:"purchased_at"`
	}

	var purchasedNotes []PurchasedNote

	err := app.DB.Unscoped().Table("purchases").
		Select("notes.*, subnoteries.name as subnotery_name, purchases.price_paid, purchases.purchased_at").
		Joins("JOIN notes ON notes.id = purchases.note_id").
		Joins("LEFT JOIN subnoteries ON subnoteries.id = notes.subnotery_id").
		Where("purchases.user_id = ?", userID).
		Order("purchases.purchased_at DESC").
		Scan(&purchasedNotes).Error

	if err != nil {
		purchaseLog.Log("MY_PURCHASES", "database error", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch purchases"})
		return
	}

	purchaseLog.Log("MY_PURCHASES", "success", "user_id", userID, "count", len(purchasedNotes))

	// Populate subnotery names manually — gorm:"-" on Note.SubnoteryName
	// prevents Scan from mapping the LEFT JOIN column.
	if len(purchasedNotes) > 0 {
		notes := make([]models.Note, len(purchasedNotes))
		for i := range purchasedNotes {
			notes[i] = purchasedNotes[i].Note
		}
		app.populateSubnoteryNames(notes)
		for i := range purchasedNotes {
			purchasedNotes[i].Note = notes[i]
		}
	}

	// Ensure JSON [] (not null) for empty results.
	if purchasedNotes == nil {
		purchasedNotes = []PurchasedNote{}
	}

	c.JSON(http.StatusOK, gin.H{"purchases": purchasedNotes})
}
