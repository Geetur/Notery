// Package middleware/auth.go contains middleware for user authentication.
package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/Geetur/Notery/internal/helpers"
)

// mwLog is the domain-specific logger for middleware operations.
var mwLog = helpers.MiddlewareLog

// RequireAuth returns a middleware that validates the JWT token in the Authorization header.
// The jwtSecret is captured once when the middleware is created, avoiding per-request env lookups.
func RequireAuth(jwtSecret string) gin.HandlerFunc {
	secretKey := []byte(jwtSecret)

	return func(c *gin.Context) {
		mwLog.Log("AUTH", "Authenticating request")

		// Extract Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			mwLog.Log("AUTH", "No Authorization header provided")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			return
		}

		// Extract token from "Bearer <token>" format
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			mwLog.Log("AUTH", "Invalid Authorization header format")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization header format"})
			return
		}

		// Parse and validate the token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return secretKey, nil
		})

		if err != nil || !token.Valid {
			mwLog.Log("AUTH", "Invalid JWT token", "error", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			mwLog.Log("AUTH", "Failed to parse JWT claims")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}

		userID, exists := claims["user_id"]
		if !exists {
			mwLog.Log("AUTH", "JWT token missing user_id claim")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}
		userIDStr := fmt.Sprint(userID)
		if userIDStr == "" {
			mwLog.Log("AUTH", "JWT token has empty user_id claim")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}
		userIDUint, parseErr := strconv.ParseUint(userIDStr, 10, 64)
		if parseErr != nil {
			mwLog.Log("AUTH", "Invalid user_id claim in JWT", "value", userIDStr)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}
		c.Set("user_id", userIDUint)
		mwLog.Log("AUTH", "User authenticated", "userID", userIDUint)
		c.Next()
	}
}

// OptionalAuth returns a middleware that extracts user info from JWT if present,
// but allows the request to proceed even without authentication.
// Use this for endpoints that work for both logged-in and anonymous users.
func OptionalAuth(jwtSecret string) gin.HandlerFunc {
	secretKey := []byte(jwtSecret)

	return func(c *gin.Context) {
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

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return secretKey, nil
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
		userIDUint, parseErr := strconv.ParseUint(userIDStr, 10, 64)
		if parseErr != nil {
			c.Next()
			return
		}

		c.Set("user_id", userIDUint)
		c.Next()
	}
}
