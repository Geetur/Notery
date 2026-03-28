// Package helpers provides shared utilities for HTTP handlers.
// This package reduces code duplication across handlers by centralizing
// common operations like parameter parsing, pagination, and logging.
//
// Domain-specific helpers are split into separate files:
//   - note.go: Note fetching and ID parsing
//   - cart.go: Cart Redis key builders
//   - subnotery.go: Subnotery fetching and ID parsing
//   - user.go: User fetching by ID and email
package helpers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ----- PARAMETER PARSING -----

// ParseUintParam extracts and parses a uint64 from a URL parameter.
// Returns the parsed value and true on success, or 0 and false on failure.
// Does NOT send an HTTP response - caller decides how to handle errors.
func ParseUintParam(c *gin.Context, param string) (uint64, bool) {
	str := c.Param(param)
	val, err := strconv.ParseUint(str, 10, 64)
	if err != nil {
		return 0, false
	}
	return val, true
}

// ----- AUTH CONTEXT -----

// GetUserID extracts the authenticated user's ID from the Gin context.
// Panics if user_id is not set (should only be called after auth middleware).
func GetUserID(c *gin.Context) uint64 {
	return c.MustGet("user_id").(uint64)
}

// TryGetUserID attempts to get the user ID, returning 0 and false if not authenticated.
// Use this for optional auth endpoints.
func TryGetUserID(c *gin.Context) (uint64, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	return userID.(uint64), true
}

// GetAdminType extracts the admin type flag from context (set by admin middleware).
// Returns true if global admin, false otherwise.
func GetAdminType(c *gin.Context) bool {
	return c.MustGet("admin_type").(bool)
}

// ----- PAGINATION -----

// Pagination holds parsed pagination parameters.
type Pagination struct {
	Page   int
	Limit  int
	Offset int
}

// DefaultPagination values
const (
	DefaultPage  = 1
	DefaultLimit = 25
	MaxLimit     = 100
	MaxPage      = 10000
)

// ParsePagination extracts and validates pagination params from query string.
// Defaults: page=1, limit=25. Max limit is 100.
func ParsePagination(c *gin.Context) Pagination {
	return ParsePaginationWithDefaults(c, DefaultLimit)
}

// ParsePaginationWithDefaults allows custom default limit.
func ParsePaginationWithDefaults(c *gin.Context, defaultLimit int) Pagination {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(defaultLimit)))

	if page < 1 {
		page = 1
	}
	if page > MaxPage {
		page = MaxPage
	}
	if limit < 1 || limit > MaxLimit {
		limit = defaultLimit
	}

	return Pagination{
		Page:   page,
		Limit:  limit,
		Offset: (page - 1) * limit,
	}
}

// ----- STRUCTURED LOGGING -----

// Logger provides domain-specific structured logging.
type Logger struct {
	Domain string // e.g., "CONTENT", "FEED", "PURCHASE"
}

// NewLogger creates a logger for a specific domain.
func NewLogger(domain string) *Logger {
	return &Logger{Domain: domain}
}

// Log outputs a structured log message.
// Format: [DOMAIN] [ACTION] message | key=value pairs
func (l *Logger) Log(action, msg string, fields ...interface{}) {
	if len(fields) == 0 {
		log.Printf("[%s] [%s] %s", l.Domain, action, msg)
		return
	}
	pairs := ""
	for i := 0; i < len(fields)-1; i += 2 {
		pairs += fmt.Sprintf(" %v=%v", fields[i], fields[i+1])
	}
	log.Printf("[%s] [%s] %s |%s", l.Domain, action, msg, pairs)
}

// Pre-configured loggers for each domain.
var (
	ContentLog      = NewLogger("CONTENT")
	FeedLog         = NewLogger("FEED")
	PurchaseLog     = NewLogger("PURCHASE")
	CartLog         = NewLogger("CART")
	NoteLog         = NewLogger("NOTE")
	AuthLog         = NewLogger("AUTH")
	SubnoteryLog    = NewLogger("SUBNOTERY")
	MiddlewareLog   = NewLogger("MIDDLEWARE")
	CommentLog      = NewLogger("COMMENT")
	PaymentLog      = NewLogger("PAYMENT")
	WebhookLog      = NewLogger("WEBHOOK")
	ProfileLog      = NewLogger("PROFILE")
	SearchLog       = NewLogger("SEARCH")
	NotificationLog = NewLogger("NOTIFICATION")
)

// ----- JSON BINDING -----

// BindJSON binds JSON request body to the given struct.
// Returns true on success, sends 400 response and returns false on failure.
// Usage: var req MyStruct; if !helpers.BindJSON(c, &req) { return }
func BindJSON(c *gin.Context, obj interface{}) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return false
	}
	return true
}
