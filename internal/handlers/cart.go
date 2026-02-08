// Package handlers/cart.go contains the HTTP handlers for cart operations.
package handlers

import (
	"net/http"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
	"github.com/gin-gonic/gin"
)

// cartLog is the domain-specific logger for cart operations.
var cartLog = helpers.CartLog

// CartRequest represents the expected structure of the cart addition request.
// ItemID will be stored in a Redis set keyed by user ID.
type CartRequest struct {
	ItemID string `json:"item_id" binding:"required"`
}

// AddToCart handles adding an item to the user's cart in Redis.
func (app *App) AddToCart(c *gin.Context) {
	cartLog.Log("ADD", "Processing add to cart request")

	// Bind and validate request body
	var cartReq CartRequest
	if !helpers.BindJSON(c, &cartReq) {
		cartLog.Log("ADD", "Failed to bind JSON request")
		return
	}

	// Extract authenticated user ID
	userID := helpers.GetUserID(c)
	cartLog.Log("ADD", "User identified", "userID", userID, "itemID", cartReq.ItemID)

	// Verify note exists and is approved before adding to cart
	var note models.Note
	if err := app.DB.First(&note, cartReq.ItemID).Error; err != nil {
		cartLog.Log("ADD", "Note not found", "itemID", cartReq.ItemID)
		c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
		return
	}
	if note.Status != models.StatusApproved {
		cartLog.Log("ADD", "Attempt to add non-approved note", "noteID", note.ID, "status", note.Status)
		c.JSON(http.StatusForbidden, gin.H{"error": "Only approved notes can be added to cart"})
		return
	}

	// Add item to user's cart set in Redis
	ctx := c.Request.Context()
	key := helpers.CartKey(userID)
	if err := app.RDB.SAdd(ctx, key, cartReq.ItemID).Err(); err != nil {
		cartLog.Log("ADD", "Redis SAdd failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add item to cart"})
		return
	}

	cartLog.Log("ADD", "Item added successfully", "userID", userID, "itemID", cartReq.ItemID)
	c.JSON(http.StatusOK, gin.H{"message": "Item added to cart successfully"})
}

// GetCart handles retrieving all items in the user's cart from Redis.
func (app *App) GetCart(c *gin.Context) {
	cartLog.Log("GET", "Processing get cart request")

	// Extract authenticated user ID
	userID := helpers.GetUserID(c)
	cartLog.Log("GET", "Fetching cart", "userID", userID)

	// Retrieve all items from user's cart set in Redis
	ctx := c.Request.Context()
	key := helpers.CartKey(userID)
	cartItems, err := app.RDB.SMembers(ctx, key).Result()
	if err != nil {
		cartLog.Log("GET", "Redis SMembers failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve cart"})
		return
	}

	cartLog.Log("GET", "Cart retrieved", "userID", userID, "itemCount", len(cartItems))
	c.JSON(http.StatusOK, gin.H{"cart": cartItems})
}

// RemoveFromCart handles removing an item from the user's cart in Redis.
func (app *App) RemoveFromCart(c *gin.Context) {
	cartLog.Log("REMOVE", "Processing remove from cart request")

	// Extract authenticated user ID and item ID from URL
	userID := helpers.GetUserID(c)
	itemID := c.Param("item_id")
	cartLog.Log("REMOVE", "Removing item", "userID", userID, "itemID", itemID)

	// Remove item from user's cart set in Redis
	ctx := c.Request.Context()
	key := helpers.CartKey(userID)
	if err := app.RDB.SRem(ctx, key, itemID).Err(); err != nil {
		cartLog.Log("REMOVE", "Redis SRem failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove item from cart"})
		return
	}

	cartLog.Log("REMOVE", "Item removed successfully", "userID", userID, "itemID", itemID)
	c.JSON(http.StatusOK, gin.H{"message": "Item removed from cart successfully"})
}
