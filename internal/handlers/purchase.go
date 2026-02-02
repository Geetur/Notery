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
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/models"
)

// PurchaseHandler handles purchase-related HTTP requests.
// It manages checkout flow and purchase history.
type PurchaseHandler struct {
	DB  *gorm.DB
	RDB *redis.Client
}

// CreatePurchaseHandler initializes a new PurchaseHandler with the given dependencies.
// CreatePurchaseHandler interacts with no other handler methods.
// CreatePurchaseHandler interacts with the database and Redis.
func CreatePurchaseHandler(db *gorm.DB, rdb *redis.Client) *PurchaseHandler {
	return &PurchaseHandler{
		DB:  db,
		RDB: rdb,
	}
}

// CheckoutCart processes the user's cart and creates purchase records.
// CheckoutCart interacts with Redis to get cart items.
// CheckoutCart interacts with the database to create purchases.
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
func (handler *PurchaseHandler) CheckoutCart(c *gin.Context) {
	log.Println("Processing checkout request...")

	// Get authenticated user
	userID := c.MustGet("user_id").(uint64)
	log.Printf("User %d initiating checkout", userID)

	// Fetch cart items from Redis
	ctx := c.Request.Context()
	cartKey := "cart:" + strconv.FormatUint(userID, 10)

	cartItems, err := handler.RDB.SMembers(ctx, cartKey).Result()
	if err != nil {
		log.Printf("Failed to fetch cart from Redis: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch cart"})
		return
	}

	if len(cartItems) == 0 {
		log.Println("Cart is empty")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cart is empty"})
		return
	}

	log.Printf("Processing %d cart items", len(cartItems))

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
		if err := handler.DB.First(&note, noteID).Error; err != nil {
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
		if err := handler.DB.Where("user_id = ? AND note_id = ?", userID, noteID).First(&existingPurchase).Error; err == nil {
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

		if err := handler.DB.Create(&purchase).Error; err != nil {
			log.Printf("Failed to create purchase for note %d: %v", noteID, err)
			errors = append(errors, "Failed to purchase: "+note.Title)
			continue
		}

		purchasedIDs = append(purchasedIDs, note.ID)
		totalAmount += note.Price
		log.Printf("Successfully purchased note %d for user %d", noteID, userID)
	}

	// Clear the cart (only remove successfully purchased items)
	for _, purchasedID := range purchasedIDs {
		handler.RDB.SRem(ctx, cartKey, strconv.FormatUint(uint64(purchasedID), 10))
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
		log.Println("No items were purchased")
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "No items could be purchased",
			"warnings": errors,
		})
		return
	}

	log.Printf("Checkout complete: %d items purchased, total: %.2f", len(purchasedIDs), totalAmount)
	c.JSON(http.StatusOK, response)
}

// PurchaseSingleNote handles direct purchase of a single note (not from cart).
// PurchaseSingleNote interacts with the database to create a purchase.
// PurchaseSingleNote does not interact with any other handler methods.
//
// This is useful for "Buy Now" buttons that bypass the cart entirely.
//
// Route: POST /api/v1/notes/:id/purchase
func (handler *PurchaseHandler) PurchaseSingleNote(c *gin.Context) {
	log.Println("Processing single note purchase...")

	// Get authenticated user
	userID := c.MustGet("user_id").(uint64)

	// Parse note ID
	noteIDStr := c.Param("id")
	noteID, err := strconv.ParseUint(noteIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid note ID"})
		return
	}

	log.Printf("User %d attempting to purchase note %d", userID, noteID)

	// Fetch the note
	var note models.Note
	if err := handler.DB.First(&note, noteID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
		return
	}

	// Validate note is purchasable
	if note.Status != "Approved" {
		c.JSON(http.StatusForbidden, gin.H{"error": "This note is not available for purchase"})
		return
	}

	if !note.HasPDF {
		c.JSON(http.StatusForbidden, gin.H{"error": "This note has no content"})
		return
	}

	// Check if already purchased
	var existingPurchase models.Purchase
	if err := handler.DB.Where("user_id = ? AND note_id = ?", userID, noteID).First(&existingPurchase).Error; err == nil {
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

	if err := handler.DB.Create(&purchase).Error; err != nil {
		log.Printf("Failed to create purchase: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete purchase"})
		return
	}

	log.Printf("User %d successfully purchased note %d", userID, noteID)
	c.JSON(http.StatusOK, gin.H{
		"message":      "Purchase successful",
		"note_id":      note.ID,
		"note_title":   note.Title,
		"price_paid":   note.Price,
		"purchased_at": purchase.PurchasedAt,
	})
}

// CheckPurchaseStatus checks if the user has purchased a specific note.
// CheckPurchaseStatus interacts with the database to check purchase existence.
// CheckPurchaseStatus does not interact with any other handler methods.
//
// Useful for frontend to show "Buy" vs "View" button.
//
// Route: GET /api/v1/notes/:id/purchased
func (handler *PurchaseHandler) CheckPurchaseStatus(c *gin.Context) {
	userID := c.MustGet("user_id").(uint64)

	noteIDStr := c.Param("id")
	noteID, err := strconv.ParseUint(noteIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid note ID"})
		return
	}

	var purchase models.Purchase
	if err := handler.DB.Where("user_id = ? AND note_id = ?", userID, noteID).First(&purchase).Error; err != nil {
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
// GetPurchaseHistory interacts with the database to fetch purchases.
// GetPurchaseHistory does not interact with any other handler methods.
//
// Route: GET /api/v1/me/purchases/history
func (handler *PurchaseHandler) GetPurchaseHistory(c *gin.Context) {
	userID := c.MustGet("user_id").(uint64)

	// Parse pagination params
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	// Count total purchases
	var total int64
	handler.DB.Model(&models.Purchase{}).Where("user_id = ?", userID).Count(&total)

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

	err := handler.DB.Table("purchases").
		Select("purchases.id as purchase_id, purchases.note_id, notes.title as note_title, notes.author as note_author, purchases.price_paid, purchases.purchased_at, notes.has_pdf").
		Joins("JOIN notes ON notes.id = purchases.note_id").
		Where("purchases.user_id = ?", userID).
		Order("purchases.purchased_at DESC").
		Offset(offset).
		Limit(limit).
		Scan(&purchases).Error

	if err != nil {
		log.Printf("Failed to fetch purchase history: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch purchase history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"purchases": purchases,
		"page":      page,
		"limit":     limit,
		"total":     total,
	})
}
