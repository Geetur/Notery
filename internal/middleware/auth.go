// Package middleware/auth.go contains middleware for authentication
package middleware

import (
	"net/http"
	"log"
	"fmt"
	"strings"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
)

// RequireAuth is a middleware function that checks for a valid JWT token in the Authorization header
// RequireAuth interacts with no other functions or methods.
// RequireAuth interacts with no models.
func RequireAuth(c *gin.Context) {
	// Middleware to require authentication via JWT token
	log.Println("Authenticating request")

	log.Println("Extracting Authorization header")
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		log.Println("No Authorization header provided")
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
		return
	}
	log.Println("Authorization header found")
	// Extract the token from the "Bearer <token>" format
	log.Println("Extracting token from Authorization header")
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		log.Println("Invalid Authorization header format")
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization header format"})
		return
	}
	log.Println("Token extracted from Authorization header")

	// Parse and validate the token
	log.Println("loading JWT secret from environment")
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found (ok):", err)
	}

	log.Println("Parsing and validating JWT token")
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Ensure the signing method is HMAC
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		// Return the secret key
		secretKey := []byte(os.Getenv("JWT_SECRET"))
		return secretKey, nil
	})

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		c.Set("user_id", claims["user_id"])
		log.Println("JWT token is valid, user authenticated:", claims["user_id"])
		c.Next()
	} else {
		log.Println("Invalid JWT token:", err)
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		return
	}

}