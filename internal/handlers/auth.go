// Package handlers/auth.go contains the HTTP handlers for authentication operations
package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Geetur/Notery/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

// AuthHandler handles authentication-related HTTP requests.
type AuthHandler struct {
	DB *gorm.DB
}

// AuthRequest represents the JSON body for signup and login requests.
type AuthRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	// some other field for admin signup can be added later
}

// CreateAuthHandler returns a new AuthHandler with the given database connection.
func CreateAuthHandler(db *gorm.DB) *AuthHandler {
	return &AuthHandler{DB: db}
}


// Signup handles user authentication and JWT token generation
// Signup interacts with the User model and database to verify credentials.
// Signup interacts with no other handler methods.
func (handler *AuthHandler) Signup(c *gin.Context) {
	log.Println("trying to sign up user...")
	var authReq AuthRequest
	if err := c.ShouldBindJSON(&authReq); err != nil {
		log.Println("Failed to bind JSON:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	// build the user model for JSON input
	user := &models.User{Email: authReq.Email}
	// Internally hash and set the password
	if err := user.SetPassword(authReq.Password); err != nil {
		log.Println("Failed to set password:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}
	// Save the user to the database
	if result := handler.DB.Create(user); result.Error != nil {
		log.Println("Failed to create user in database:", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user (email already exists?)"})
		return
	}
	log.Println("successfully signed up user")
	c.JSON(http.StatusCreated, gin.H{"message": "User created successfully", "user_id": user.ID})
}

// Login handles user authentication and JWT token generation
// Login interacts with the User model and database to verify credentials.
// Login interacts with no other handler methods.
func (handler *AuthHandler) Login(c *gin.Context) {
	log.Println("trying to log in user...")
	var authReq AuthRequest

	if err := c.ShouldBindJSON(&authReq); err != nil {
		log.Println("Failed to bind JSON:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Find the user by email
	var user models.User
	log.Println("looking up user in database:", authReq.Email)
	if result := handler.DB.Where("email = ?", authReq.Email).First(&user); result.Error != nil {
		log.Println("User not found:", result.Error)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}
	log.Println("user found in database:", user.Email)

	// Check the password
	log.Println("verifying password for user:", authReq.Email)
	if !user.CheckPassword(authReq.Password) {
		log.Println("Invalid password for user:", authReq.Email)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}
	log.Println("password verified for user:", authReq.Email)

	// Generate JWT token
	log.Println("generating JWT token for user:", authReq.Email)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": fmt.Sprint(user.ID), // include user ID in token claims
		"exp":     time.Now().Add(time.Hour * 24).Unix(), // token expires in 24 hours
	})
	log.Println("JWT token generated.")
	
	// Load secret key from environment variable
	log.Println("loading JWT secret from environment...")
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found (ok):", err)
	}
	secretKey := []byte(os.Getenv("JWT_SECRET"))
	if len(secretKey) == 0 {
		log.Println("JWT_SECRET not set in environment")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "JWT secret not configured"})
		return
	}
	log.Println("JWT secret loaded from environment")

	// Sign the token with the secret key
	log.Println("trying to sign JWT token...")
	tokenString, err := token.SignedString(secretKey)
	if err != nil {
		log.Println("Failed to sign JWT token:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}
	log.Println("successfully generated JWT token for user:", authReq.Email)
	
	// Return the token in the response
	log.Println("successfully logged user in:", authReq.Email)
	c.JSON(http.StatusOK, gin.H{"token": tokenString})
}
