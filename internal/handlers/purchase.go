// Package handlers/purchase.go contains HTTP handlers for purchase operations.
// This file manages note purchases (checkout) and purchase history.
//
// PURCHASE FLOW (with Order state machine):
// ------------------------------------------
// 1. User adds approved notes to cart (cart handler)
// 2. User initiates checkout with an idempotency key
// 3. System creates an Order (pending) with OrderItems
// 4. System validates all items (approved, has PDF, not already purchased)
// 5. Order transitions: pending → paid → fulfilled
// 6. Purchase records are created during fulfillment
// 7. Cart is cleared
//
// PAYMENT INTEGRATION (FUTURE):
// -----------------------------
// When a real payment provider is integrated:
// - Step 4 creates a PaymentIntent externally
// - A webhook confirms payment → transitions Order to "paid"
// - Fulfillment runs on the "paid" event
// The current implementation auto-transitions for testing.
package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// purchaseLog is the shared logger for purchase operations
var purchaseLog = helpers.PurchaseLog

// CheckoutCart processes the user's cart through the Order state machine.
//
// Request body (optional): { "idempotency_key": "uuid-here" }
//
// Route: POST /api/v1/checkout
func (app *App) CheckoutCart(c *gin.Context) {
	start := time.Now()

	userID := helpers.GetUserID(c)
	purchaseLog.Log("CHECKOUT", "initiated", "user_id", userID)

	// Accept optional idempotency key
	var body struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	// Best-effort bind; missing key is fine
	_ = c.ShouldBindJSON(&body)

	// If idempotency key provided, check for existing order
	if body.IdempotencyKey != "" {
		var existing models.Order
		if err := app.DB.Where("idempotency_key = ? AND user_id = ?", body.IdempotencyKey, userID).
			Preload("Items").First(&existing).Error; err == nil {
			purchaseLog.Log("CHECKOUT", "idempotent hit", "user_id", userID, "order_id", existing.ID)
			c.JSON(http.StatusOK, gin.H{
				"order_id":    existing.ID,
				"status":      existing.Status,
				"total_cents": existing.TotalCents,
				"idempotent":  true,
			})
			return
		}
	}

	// Fetch cart items from Redis
	ctx := c.Request.Context()
	cartKey := "cart:" + strconv.FormatUint(userID, 10)

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
	var validNotes []models.Note
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

		// Check if already purchased
		var existing models.Purchase
		if err := app.DB.Where("user_id = ? AND note_id = ?", userID, noteID).First(&existing).Error; err == nil {
			warnings = append(warnings, "Already purchased: "+note.Title)
			continue
		}

		orderItems = append(orderItems, models.OrderItem{
			NoteID:     note.ID,
			PriceCents: note.Price,
		})
		validNotes = append(validNotes, note)
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

	// ---------- Phase 2: Create order + fulfil in one transaction ----------
	var order models.Order

	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		// Create Order (pending)
		order = models.Order{
			UserID:         userID,
			Status:         models.OrderPending,
			TotalCents:     totalCents,
			IdempotencyKey: body.IdempotencyKey,
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}

		// Create OrderItems
		for i := range orderItems {
			orderItems[i].OrderID = order.ID
		}
		if err := tx.Create(&orderItems).Error; err != nil {
			return err
		}

		// Transition: pending → paid (auto for now, webhook in production)
		if err := tx.Model(&order).Update("status", models.OrderPaid).Error; err != nil {
			return err
		}

		// Transition: paid → fulfilled — create Purchase records
		for _, note := range validNotes {
			purchase := models.Purchase{
				UserID:      uint(userID),
				NoteID:      note.ID,
				PricePaid:   note.Price,
				PurchasedAt: time.Now(),
			}
			if err := tx.Create(&purchase).Error; err != nil {
				return err
			}
		}

		if err := tx.Model(&order).Update("status", models.OrderFulfilled).Error; err != nil {
			return err
		}
		order.Status = models.OrderFulfilled

		return nil
	}); err != nil {
		purchaseLog.Log("CHECKOUT", "transaction failed", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Checkout failed"})
		return
	}

	// Clear successfully purchased items from cart
	for _, note := range validNotes {
		app.RDB.SRem(ctx, cartKey, strconv.FormatUint(uint64(note.ID), 10))
	}

	response := gin.H{
		"order_id":        order.ID,
		"status":          order.Status,
		"purchased_count": len(validNotes),
		"total_cents":     totalCents,
	}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}

	duration := time.Since(start)
	purchaseLog.Log("CHECKOUT", "completed", "user_id", userID, "order_id", order.ID, "items", len(validNotes), "total_cents", totalCents, "duration_ms", duration.Milliseconds())
	c.JSON(http.StatusOK, response)
}

// PurchaseSingleNote handles direct purchase of a single note (not from cart).
// Creates a single-item Order → fulfils it in one transaction.
//
// Request body (optional): { "idempotency_key": "uuid-here" }
//
// Route: POST /api/v1/notes/:id/purchase
func (app *App) PurchaseSingleNote(c *gin.Context) {
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
			First(&existing).Error; err == nil {
			purchaseLog.Log("SINGLE", "idempotent hit", "user_id", userID, "order_id", existing.ID)
			c.JSON(http.StatusOK, gin.H{
				"order_id":   existing.ID,
				"status":     existing.Status,
				"idempotent": true,
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

	var order models.Order

	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		order = models.Order{
			UserID:         userID,
			Status:         models.OrderPending,
			TotalCents:     note.Price,
			IdempotencyKey: body.IdempotencyKey,
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}

		item := models.OrderItem{
			OrderID:    order.ID,
			NoteID:     note.ID,
			PriceCents: note.Price,
		}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}

		// Auto-transition: pending → paid → fulfilled
		if err := tx.Model(&order).Update("status", models.OrderPaid).Error; err != nil {
			return err
		}

		purchase := models.Purchase{
			UserID:      uint(userID),
			NoteID:      note.ID,
			PricePaid:   note.Price,
			PurchasedAt: time.Now(),
		}
		if err := tx.Create(&purchase).Error; err != nil {
			return err
		}

		if err := tx.Model(&order).Update("status", models.OrderFulfilled).Error; err != nil {
			return err
		}
		order.Status = models.OrderFulfilled

		return nil
	}); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			purchaseLog.Log("SINGLE", "duplicate purchase prevented", "user_id", userID, "note_id", noteID)
			c.JSON(http.StatusConflict, gin.H{"error": "You have already purchased this note"})
			return
		}
		purchaseLog.Log("SINGLE", "transaction failed", "user_id", userID, "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete purchase"})
		return
	}

	purchaseLog.Log("SINGLE", "success", "user_id", userID, "note_id", noteID, "order_id", order.ID, "price_cents", note.Price)
	c.JSON(http.StatusOK, gin.H{
		"message":      "Purchase successful",
		"order_id":     order.ID,
		"note_id":      note.ID,
		"note_title":   note.Title,
		"price_paid":   note.Price,
		"purchased_at": time.Now(),
	})
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
		PurchaseID  uint      `json:"purchase_id"`
		NoteID      uint      `json:"note_id"`
		NoteTitle   string    `json:"note_title"`
		NoteAuthor  string    `json:"note_author"`
		PricePaid   int64     `json:"price_paid"`
		PurchasedAt time.Time `json:"purchased_at"`
		HasPDF      bool      `json:"has_pdf"`
	}

	var purchases []PurchaseWithNote

	err := app.DB.Table("purchases").
		Select("purchases.id as purchase_id, purchases.note_id, notes.title as note_title, notes.author as note_author, purchases.price_paid, purchases.purchased_at, notes.has_pdf").
		Joins("JOIN notes ON notes.id = purchases.note_id").
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
	c.JSON(http.StatusOK, gin.H{
		"purchases": purchases,
		"page":      pag.Page,
		"limit":     pag.Limit,
		"total":     total,
	})
}
