// Package handlers provides HTTP request handlers for the Notery API.
package handlers

import (
	"log"
	"net/http"

	"github.com/Geetur/Notery/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SubnoteryHandler handles subnotery management HTTP requests.
type SubnoteryHandler struct {
	DB *gorm.DB
}

// CreateSubnoteryHandler returns a new SubnoteryHandler with the given database connection.
func CreateSubnoteryHandler(db *gorm.DB) *SubnoteryHandler {
	return &SubnoteryHandler{DB: db}
}

// AddAdminToSubnotery grants admin privileges to a user for a specific subnotery.
func (h *SubnoteryHandler) AddAdminToSubnotery(c *gin.Context) {
	log.Println("Adding admin to subnotery...")

	subnoteryID := c.Param("subnotery_id")
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	// binding JSON request
	log.Println("Binding JSON request...")
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Println("Failed to bind JSON:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	log.Println("JSON request bound successfully:", req.Email)

	// looking up subnotery in database
	log.Println("Looking up subnotery in database...")
	var subnotery models.Subnotery
	if err := h.DB.Where("id = ?", subnoteryID).First(&subnotery).Error; err != nil {
		log.Println("Subnotery not found:", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Subnotery not found"})
		return
	}
	log.Println("Subnotery found:", subnotery.ID)

	// look up user by email
	log.Println("Looking up user by email...")
	var user models.User
	if err := h.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		log.Println("User not found:", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	log.Println("User found:", user.ID)

	// add user as admin to subnotery
	if err := h.DB.Model(&subnotery).Association("Admins").Append(&user); err != nil {
		log.Println("Failed to add admin to subnotery:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add admin to subnotery"})
		return
	}
	log.Println("Admin added to subnotery successfully:", user.ID)

	c.JSON(http.StatusOK, gin.H{"message": "Admin added to subnotery successfully"})

}
