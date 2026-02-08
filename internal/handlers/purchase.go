// Package handlers/purchase.go contains HTTP handlers for purchase operations.
// This file manages note purchases (checkout) and purchase history.
//
// PURCHASE FLOW:
// --------------
// 1. User adds approved note to cart (existing cart handler)
// 2. User initiates checkout (this handler)
// 3. System validates cart items are still approved and available
// 4. System creates Purchase records for each item
// 5. Cart is cleared
// 6. User can now view purchased notes via content handler
//
// PAYMENT INTEGRATION (FUTURE):
// -----------------------------
// Currently, this is a simplified checkout that creates purchase records directly.
// In production, you would integrate with a payment provider (Stripe, etc.):
// 1. Create a payment intent with the total amount
// 2. Wait for webhook confirmation of successful payment
// 3. Only then create Purchase records
//
// The current implementation is suitable for testing and can be extended.
package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// purchaseLog is the shared logger for purchase operations
var purchaseLog = helpers.PurchaseLog

// CheckoutCart processes the user's cart and creates purchase records.
//
// This endpoint:
// 1. Fetches all items from user's Redis cart
// 2. Validates each item (note exists, is approved, not already purchased)
// 3. Creates Purchase records for valid items
// 4. Clears the cart on success
//
// In production, this would integrate with payment processing between steps 2-3.
//
// Response: JSON with list of successfully purchased note IDs
//
// Route: POST /api/v1/checkout
func (app *App) CheckoutCart(c *gin.Context) {
	start := time.Now()

	// Get authenticated user using helpers
	userID := helpers.GetUserID(c)
	purchaseLog.Log("CHECKOUT", "initiated", "user_id", userID)

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

	// Process each cart item
	var purchasedIDs []uint
	var errors []string
	var totalAmount float64

	for _, itemIDStr := range cartItems {
		noteID, parseErr := strconv.ParseUint(itemIDStr, 10, 64)
		if parseErr != nil {
			errors = append(errors, "Invalid item ID: "+itemIDStr)
			continue
		}

		// Fetch the note
		var note models.Note
		if err := app.DB.First(&note, noteID).Error; err != nil {
			errors = append(errors, "Note not found: "+itemIDStr)
			continue
		}

		// Validate note is approved
		if note.Status != "Approved" {
			errors = append(errors, "Note not available: "+note.Title)
			continue
		}

		// Validate note has PDF content
		if !note.HasPDF {
			errors = append(errors, "Note has no content: "+note.Title)
			continue
		}

		// Check if already purchased (prevent duplicate purchases)
		var existingPurchase models.Purchase
		if err := app.DB.Where("user_id = ? AND note_id = ?", userID, noteID).First(&existingPurchase).Error; err == nil {
			errors = append(errors, "Already purchased: "+note.Title)
			continue
		}

		// Create purchase record
		purchase := models.Purchase{
			UserID:      uint(userID),
			NoteID:      note.ID,
			PricePaid:   note.Price,
			PurchasedAt: time.Now(),
		}

		if err := app.DB.Create(&purchase).Error; err != nil {
			purchaseLog.Log("CHECKOUT", "db error creating purchase", "user_id", userID, "note_id", noteID, "error", err)
			errors = append(errors, "Failed to purchase: "+note.Title)
			continue
		}

		purchasedIDs = append(purchasedIDs, note.ID)
		totalAmount += note.Price
		purchaseLog.Log("CHECKOUT", "item purchased", "user_id", userID, "note_id", noteID, "price", note.Price)
	}

	// Clear the cart (only remove successfully purchased items)
	for _, purchasedID := range purchasedIDs {
		app.RDB.SRem(ctx, cartKey, strconv.FormatUint(uint64(purchasedID), 10))
	}

	// Build response
	response := gin.H{
		"purchased_count": len(purchasedIDs),
		"purchased_ids":   purchasedIDs,
		"total_amount":    totalAmount,
	}

	if len(errors) > 0 {
		response["warnings"] = errors
	}

	if len(purchasedIDs) == 0 {
		purchaseLog.Log("CHECKOUT", "failed - no items purchased", "user_id", userID, "warnings", len(errors))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "No items could be purchased",
			"warnings": errors,
		})
		return
	}

	duration := time.Since(start)
	purchaseLog.Log("CHECKOUT", "completed", "user_id", userID, "items", len(purchasedIDs), "total", fmt.Sprintf("%.2f", totalAmount), "duration_ms", duration.Milliseconds())
	c.JSON(http.StatusOK, response)
}

// PurchaseSingleNote handles direct purchase of a single note (not from cart).
//
// This is useful for "Buy Now" buttons that bypass the cart entirely.
//
// Route: POST /api/v1/notes/:id/purchase
func (app *App) PurchaseSingleNote(c *gin.Context) {
	// Get authenticated user and note ID using helpers
	userID := helpers.GetUserID(c)
	noteID, ok := helpers.MustParseNoteID(c)
	if !ok {
		return
	}

	purchaseLog.Log("SINGLE", "processing", "user_id", userID, "note_id", noteID)

	// Fetch the note
	var note models.Note
	if err := app.DB.First(&note, noteID).Error; err != nil {
		purchaseLog.Log("SINGLE", "note not found", "note_id", noteID, "error", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
		return
	}

	// Validate note is purchasable
	if note.Status != "Approved" {
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

	// Create purchase record
	purchase := models.Purchase{
		UserID:      uint(userID),
		NoteID:      note.ID,
		PricePaid:   note.Price,
		PurchasedAt: time.Now(),
	}

	if err := app.DB.Create(&purchase).Error; err != nil {
		purchaseLog.Log("SINGLE", "db error", "user_id", userID, "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete purchase"})
		return
	}

	purchaseLog.Log("SINGLE", "success", "user_id", userID, "note_id", noteID, "price", note.Price)
	c.JSON(http.StatusOK, gin.H{
		"message":      "Purchase successful",
		"note_id":      note.ID,
		"note_title":   note.Title,
		"price_paid":   note.Price,
		"purchased_at": purchase.PurchasedAt,
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
		PricePaid   float64   `json:"price_paid"`
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
