// auth.go — JWT authentication middleware (required and optional variants).
//
// MIDDLEWARE:
//
//	RequireAuth   Validates JWT, extracts user_id, sets context. Rejects 401 on failure.
//	OptionalAuth  Same JWT parsing, but allows unauthenticated requests through.
//
// DESIGN:
//
//	Both middlewares share extractBearerToken() and parseJWTUserID() helpers to avoid
//	duplicating JWT parsing logic. The JWT secret is captured once at middleware
//	creation time (closure), so config changes require a restart.
//
//	JWT claims must contain a "user_id" field (set during token issuance in auth.go).
//	The parsed user_id is stored in the Gin context for downstream handlers via
//	c.Set("user_id", userID).
package middleware

import (
	"errors"
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

// errNoBearer is returned when the Authorization header is missing the "Bearer " prefix.
var errNoBearer = errors.New("missing or invalid Bearer prefix")

// extractBearerToken pulls the raw token string from an Authorization header.
// Returns errNoBearer if the header is empty or has the wrong format.
func extractBearerToken(authHeader string) (string, error) {
	if authHeader == "" {
		return "", errNoBearer
	}
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		return "", errNoBearer
	}
	return tokenString, nil
}

// parseJWTUserID validates a JWT string and extracts the user_id claim.
// Returns the user ID or an error describing the failure.
func parseJWTUserID(tokenString string, secretKey []byte) (uint64, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secretKey, nil
	})
	if err != nil || !token.Valid {
		return 0, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, errors.New("invalid claims")
	}

	raw, exists := claims["user_id"]
	if !exists {
		return 0, errors.New("missing user_id claim")
	}

	userIDStr := fmt.Sprint(raw)
	if userIDStr == "" {
		return 0, errors.New("empty user_id claim")
	}

	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid user_id value %q: %w", userIDStr, err)
	}
	return userID, nil
}

// RequireAuth returns middleware that validates the JWT in the Authorization header.
// Also supports a ?token= query parameter as fallback for endpoints that need
// browser-native access (e.g., PDF viewer in an iframe).
// Requests without a valid token are rejected with 401.
// The jwtSecret is captured once when the middleware is created.
func RequireAuth(jwtSecret string) gin.HandlerFunc {
	secretKey := []byte(jwtSecret)

	return func(c *gin.Context) {
		mwLog.Log("AUTH", "Authenticating request")

		tokenString, err := extractBearerToken(c.GetHeader("Authorization"))
		if err != nil {
			// Fallback: check query parameter (used by PDF viewer)
			if qToken := c.Query("token"); qToken != "" {
				tokenString = qToken
				err = nil
			}
		}
		if err != nil {
			mwLog.Log("AUTH", "No valid Authorization header")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			return
		}

		userID, err := parseJWTUserID(tokenString, secretKey)
		if err != nil {
			mwLog.Log("AUTH", "JWT validation failed", "error", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}

		c.Set("user_id", userID)
		mwLog.Log("AUTH", "User authenticated", "userID", userID)
		c.Next()
	}
}

// OptionalAuth returns middleware that extracts user info from JWT if present,
// but allows the request to proceed even without authentication.
// Use this for endpoints that personalize responses for logged-in users.
func OptionalAuth(jwtSecret string) gin.HandlerFunc {
	secretKey := []byte(jwtSecret)

	return func(c *gin.Context) {
		tokenString, err := extractBearerToken(c.GetHeader("Authorization"))
		if err != nil {
			c.Next()
			return
		}

		userID, err := parseJWTUserID(tokenString, secretKey)
		if err != nil {
			c.Next()
			return
		}

		c.Set("user_id", userID)
		c.Next()
	}
}
