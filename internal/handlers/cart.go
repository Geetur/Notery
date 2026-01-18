// Package handlers/redis.go contains the HTTP handlers for cart operations
package handlers

import (
	"net/http"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type CartHandler struct {
	RDB *redis.Client
}

// CartRequest represents the expected structure of the cart addition request
// this will ultimately be binded as a hset in redis
// and userID will be used to lookup whatever info we need in postgres
type CartRequest struct {
	ItemID    string `json:"item_id" binding:"required"`
	// dont need a user ID here 
	// since it will be extracted from the JWT token in middleware
}

// CreateCartHandler initializes a new CartHandler with the given Redis client
func CreateCartHandler(rdb *redis.Client) *CartHandler {
	return &CartHandler{RDB: rdb}
}

// AddToCart handles adding an item to the user's cart in Redis
// AddToCart interacts with Redis to add the item.
// AddToCart interacts with no other handler methods.
func (handler *CartHandler) AddToCart(c *gin.Context) {
	log.Println("trying to add item to cart in Redis...")
	var cartReq CartRequest
	if err := c.ShouldBindJSON(&cartReq); err != nil {
		log.Println("Failed to bind JSON:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	log.Println("Extracting user ID from context...")
	userIDVal, exists := c.Get("user_id")
	if !exists {
		log.Println("User ID not found in context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	userID, ok := userIDVal.(string)
	if !ok {
		log.Println("User ID in context is not a string")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID format"})
		return
	}
	log.Println("User ID extracted from context:", userID)

	key := "cart:" + userID
	field := cartReq.ItemID
	ctx := c.Request.Context()
	err := handler.RDB.SAdd(ctx, key, field).Err()
	if err != nil {
		log.Println("Failed to add item to cart in Redis:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add item to cart"})
		return
	}
	log.Println("successfully added item to cart in Redis")

	c.JSON(http.StatusOK, gin.H{"message": "Item added to cart successfully"})
}

// GetCart handles retrieving all items in the user's cart from Redis
// GetCart interacts with Redis to retrieve the cart items.
// GetCart interacts with no other handler methods.
func (handler *CartHandler) GetCart(c *gin.Context) {
	// Use Redis SMembers to retrieve all items in the user's cart
	log.Println("extracting user ID from context for cart retrieval...")
	userIDVal, exists := c.Get("user_id")
	if !exists {
		log.Println("User ID not found in context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	userID, ok := userIDVal.(string)
	if !ok {
		log.Println("User ID in context is not a string")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID format"})
		return
	}
	log.Println("user ID extracted from context:", userID)

	log.Println("trying to retrieve cart from Redis...")
	key := "cart:" + userID
	ctx := c.Request.Context()
	cartItems, err := handler.RDB.SMembers(ctx, key).Result()
	if err != nil {
		log.Println("Failed to retrieve cart from Redis:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve cart"})
		return
	}
	log.Println("successfully retrieved cart from Redis")

	c.JSON(http.StatusOK, gin.H{"cart": cartItems})
} 

// RemoveFromCart handles removing an item from the user's cart in Redis
// RemoveFromCart interacts with Redis to remove the item.
// RemoveFromCart interacts with no other handler methods.
func (handler *CartHandler) RemoveFromCart(c *gin.Context) {
	// Use Redis SRem to remove an item from the user's cart
	userIDVal, exists := c.Get("user_id")
	if !exists {
		log.Println("User ID not found in context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	userID, ok := userIDVal.(string)
	if !ok {
		log.Println("User ID in context is not a string")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID format"})
		return
	}

	log.Println("trying to remove item from cart...")
	ctx := c.Request.Context()  
	itemID := c.Param("item_id")
	key := "cart:" + userID
	err := handler.RDB.SRem(ctx, key, itemID).Err()
	if err != nil {
		log.Println("Failed to remove item from cart in Redis:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove item from cart"})
		return
	}
	log.Println("successfully removed item from cart")
	c.JSON(http.StatusOK, gin.H{"message": "Item removed from cart successfully"})
}