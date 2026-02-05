// Package handlers provides HTTP request handlers for the Notery API.
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/helpers"
)

// subnoteryLog is the domain-specific logger for subnotery operations.
var subnoteryLog = helpers.SubnoteryLog

// SubnoteryHandler handles subnotery management HTTP requests.
type SubnoteryHandler struct {
	DB *gorm.DB
}

// CreateSubnoteryHandler returns a new SubnoteryHandler with the given database connection.
func CreateSubnoteryHandler(db *gorm.DB) *SubnoteryHandler {
	return &SubnoteryHandler{DB: db}
}

// AddAdminToSubnotery grants admin privileges to a user for a specific subnotery.
// Requires the target user's email and the subnotery ID from URL params.
func (h *SubnoteryHandler) AddAdminToSubnotery(c *gin.Context) {
	subnoteryLog.Log("ADD_ADMIN", "Processing add admin request")

	// Parse subnotery ID from URL
	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		subnoteryLog.Log("ADD_ADMIN", "Invalid subnotery ID in URL")
		return
	}

	// Bind and validate request body
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if !helpers.BindJSON(c, &req) {
		subnoteryLog.Log("ADD_ADMIN", "Failed to bind JSON request")
		return
	}
	subnoteryLog.Log("ADD_ADMIN", "Request validated", "subnoteryID", subnoteryID, "email", req.Email)

	// Fetch subnotery from database
	subnotery, ok := helpers.FetchSubnotery(c, h.DB, subnoteryID)
	if !ok {
		subnoteryLog.Log("ADD_ADMIN", "Subnotery not found", "subnoteryID", subnoteryID)
		return
	}

	// Fetch user by email
	user, found := helpers.FetchUserByEmail(h.DB, req.Email)
	if !found {
		subnoteryLog.Log("ADD_ADMIN", "User not found", "email", req.Email)
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	subnoteryLog.Log("ADD_ADMIN", "User found", "userID", user.ID)

	// Add user as admin to subnotery
	if err := h.DB.Model(subnotery).Association("Admins").Append(user); err != nil {
		subnoteryLog.Log("ADD_ADMIN", "Failed to add admin", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add admin to subnotery"})
		return
	}

	subnoteryLog.Log("ADD_ADMIN", "Admin added successfully", "subnoteryID", subnoteryID, "userID", user.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Admin added to subnotery successfully"})
}

// JoinSubnotery adds the authenticated user as a member of the specified subnotery.
func (h *SubnoteryHandler) JoinSubnotery(c *gin.Context) {
	subnoteryLog.Log("JOIN", "Processing join subnotery request")

	// Parse subnotery ID from URL
	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		subnoteryLog.Log("JOIN", "Invalid subnotery ID in URL")
		return
	}

	// Fetch subnotery from database
	subnotery, ok := helpers.FetchSubnotery(c, h.DB, subnoteryID)
	if !ok {
		subnoteryLog.Log("JOIN", "Subnotery not found", "subnoteryID", subnoteryID)
		return
	}

	// Get authenticated user ID
	userID := helpers.GetUserID(c)
	subnoteryLog.Log("JOIN", "User identified", "userID", userID, "subnoteryID", subnoteryID)

	// Fetch user record
	user, ok := helpers.FetchUser(c, h.DB, userID)
	if !ok {
		subnoteryLog.Log("JOIN", "User not found", "userID", userID)
		return
	}

	// Add user as member to subnotery
	if err := h.DB.Model(subnotery).Association("Members").Append(user); err != nil {
		subnoteryLog.Log("JOIN", "Failed to add member", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to join subnotery"})
		return
	}

	subnoteryLog.Log("JOIN", "User joined successfully", "userID", userID, "subnoteryID", subnoteryID)
	c.JSON(http.StatusOK, gin.H{"message": "Joined subnotery successfully"})
}
