// Package handlers/auth.go contains the HTTP handlers for authentication operations.
package handlers

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// authLog is the domain-specific logger for authentication operations.
var authLog = helpers.AuthLog

// AuthRequest represents the JSON body for signup and login requests.
type AuthRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// Signup handles user registration and account creation.
func (app *App) Signup(c *gin.Context) {
	authLog.Log("SIGNUP", "Processing signup request")

	// Bind and validate request body
	var authReq AuthRequest
	if !helpers.BindJSON(c, &authReq) {
		authLog.Log("SIGNUP", "Failed to bind JSON request")
		return
	}
	authLog.Log("SIGNUP", "Request validated", "email", authReq.Email)

	// Build the user model and hash password
	user := &models.User{Email: authReq.Email}
	if err := user.SetPassword(authReq.Password); err != nil {
		authLog.Log("SIGNUP", "Failed to hash password", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	// Persist user to database
	if result := app.DB.Create(user); result.Error != nil {
		authLog.Log("SIGNUP", "Failed to create user in database", "error", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user (email already exists?)"})
		return
	}

	authLog.Log("SIGNUP", "User created successfully", "userID", user.ID, "email", authReq.Email)
	c.JSON(http.StatusCreated, gin.H{"message": "User created successfully", "user_id": user.ID})
}

// Login handles user authentication and JWT token generation.
func (app *App) Login(c *gin.Context) {
	authLog.Log("LOGIN", "Processing login request")

	// Bind and validate request body
	var authReq AuthRequest
	if !helpers.BindJSON(c, &authReq) {
		authLog.Log("LOGIN", "Failed to bind JSON request")
		return
	}
	authLog.Log("LOGIN", "Request validated", "email", authReq.Email)

	// Find user by email
	user, found := helpers.FetchUserByEmail(app.DB, authReq.Email)
	if !found {
		authLog.Log("LOGIN", "User not found", "email", authReq.Email)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// Verify password
	if !user.CheckPassword(authReq.Password) {
		authLog.Log("LOGIN", "Invalid password", "email", authReq.Email)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}
	authLog.Log("LOGIN", "Password verified", "userID", user.ID)

	// Generate JWT token with user claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": fmt.Sprint(user.ID),
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	})

	// Load JWT secret from environment
	if err := godotenv.Load(); err != nil {
		authLog.Log("LOGIN", "No .env file found (ok)")
	}
	secretKey := []byte(os.Getenv("JWT_SECRET"))
	if len(secretKey) == 0 {
		authLog.Log("LOGIN", "JWT_SECRET not configured")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "JWT secret not configured"})
		return
	}

	// Sign and return token
	tokenString, err := token.SignedString(secretKey)
	if err != nil {
		authLog.Log("LOGIN", "Failed to sign JWT", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	authLog.Log("LOGIN", "Login successful", "userID", user.ID, "email", authReq.Email)
	c.JSON(http.StatusOK, gin.H{"token": tokenString})
}
