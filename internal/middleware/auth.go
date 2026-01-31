// Package middleware/auth.go contains middleware for user authentication
package middleware

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
)

// RequireAuth is a middleware function that checks for a valid JWT token in the Authorization header
// RequireAuth interacts with no other functions or methods.
// RequireAuth interacts with no models.
func RequireAuth(c *gin.Context) {
	// Middleware to require authentication via JWT token
	log.Println("Authenticating request...")

	log.Println("Extracting Authorization header...")
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		log.Println("No Authorization header provided")
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
		return
	}
	log.Println("Authorization header found")
	// Extract the token from the "Bearer <token>" format
	log.Println("Extracting token from Authorization header...")
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		log.Println("Invalid Authorization header format")
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization header format"})
		return
	}
	log.Println("Token extracted from Authorization header")

	// Parse and validate the token
	log.Println("loading JWT secret from environment...")
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found (ok):", err)
	}

	log.Println("Parsing and validating JWT token...")
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Ensure the signing method is HMAC
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		// Return the secret key
		secretKey := []byte(os.Getenv("JWT_SECRET"))
		return secretKey, nil
	})

	claims, ok := token.Claims.(jwt.MapClaims)
	if ok && token.Valid {
		userID, exists := claims["user_id"]
		if !exists {
			log.Println("JWT token missing user_id claim")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}
		userIDStr := fmt.Sprint(userID)
		if userIDStr == "" {
			log.Println("JWT token has empty user_id claim")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}
		userIDUint, err := strconv.ParseUint(userIDStr, 10, 64)
		if err != nil {
			log.Println("Invalid user_id claim in JWT token:", userIDStr)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}
		c.Set("user_id", userIDUint)
		log.Println("JWT token is valid, user authenticated:", userIDStr)
		c.Next()
	} else {
		log.Println("Invalid JWT token:", err)
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		return
	}
}

// OptionalAuth is a middleware that extracts user info from JWT if present,
// but allows the request to proceed even without authentication.
// Use this for endpoints that work for both logged-in and anonymous users.
func OptionalAuth(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		// No auth header, proceed as anonymous
		c.Next()
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		// Invalid format, proceed as anonymous
		c.Next()
		return
	}

	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found (ok):", err)
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil || !token.Valid {
		// Invalid token, proceed as anonymous
		c.Next()
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		c.Next()
		return
	}

	userID, exists := claims["user_id"]
	if !exists {
		c.Next()
		return
	}

	userIDStr := fmt.Sprint(userID)
	userIDUint, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		c.Next()
		return
	}

	c.Set("user_id", userIDUint)
	c.Next()
}
