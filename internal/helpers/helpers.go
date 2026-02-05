// Package helpers provides shared utilities for HTTP handlers.
// This package reduces code duplication across handlers by centralizing
// common operations like parameter parsing, pagination, and logging.
package helpers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/models"
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

// MustParseNoteID extracts the "id" parameter as a note ID.
// On failure, sends a 400 response and returns 0, false.
// Usage: noteID, ok := helpers.MustParseNoteID(c); if !ok { return }
func MustParseNoteID(c *gin.Context) (uint64, bool) {
	noteID, ok := ParseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid note ID"})
		return 0, false
	}
	return noteID, true
}

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
	if limit < 1 || limit > MaxLimit {
		limit = defaultLimit
	}

	return Pagination{
		Page:   page,
		Limit:  limit,
		Offset: (page - 1) * limit,
	}
}

// ----- NOTE FETCHING -----

// FetchNote retrieves a note by ID from the database.
// Returns the note and true on success.
// On failure, sends appropriate HTTP response (404 or 500) and returns nil, false.
func FetchNote(c *gin.Context, db *gorm.DB, noteID uint64) (*models.Note, bool) {
	var note models.Note
	if err := db.First(&note, noteID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch note"})
		}
		return nil, false
	}
	return &note, true
}

// MustFetchNote combines MustParseNoteID and FetchNote for the common pattern.
// Extracts note ID from "id" param, fetches from DB, handles all errors.
// Usage: note, ok := helpers.MustFetchNote(c, db); if !ok { return }
func MustFetchNote(c *gin.Context, db *gorm.DB) (*models.Note, bool) {
	noteID, ok := MustParseNoteID(c)
	if !ok {
		return nil, false
	}
	return FetchNote(c, db, noteID)
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

// Pre-configured loggers for each domain
var (
	ContentLog   = NewLogger("CONTENT")
	FeedLog      = NewLogger("FEED")
	PurchaseLog  = NewLogger("PURCHASE")
	CartLog      = NewLogger("CART")
	NoteLog      = NewLogger("NOTE")
	AuthLog      = NewLogger("AUTH")
	SubnoteryLog = NewLogger("SUBNOTERY")
	MiddlewareLog = NewLogger("MIDDLEWARE")
)

// ----- RESPONSE HELPERS -----

// RespondError sends a JSON error response with the given status code.
func RespondError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

// RespondOK sends a JSON response with status 200.
func RespondOK(c *gin.Context, data gin.H) {
	c.JSON(http.StatusOK, data)
}

// RespondCreated sends a JSON response with status 201.
func RespondCreated(c *gin.Context, data gin.H) {
	c.JSON(http.StatusCreated, data)
}

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

// ----- REDIS KEY BUILDERS -----

// CartKey returns the Redis key for a user's shopping cart.
func CartKey(userID uint64) string {
	return "cart:" + strconv.FormatUint(userID, 10)
}

// ----- SUBNOTERY HELPERS -----

// MustParseSubnoteryID extracts the "subnotery_id" parameter from URL.
// On failure, sends a 400 response and returns 0, false.
func MustParseSubnoteryID(c *gin.Context) (uint64, bool) {
	subnoteryID, ok := ParseUintParam(c, "subnotery_id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subnotery ID"})
		return 0, false
	}
	return subnoteryID, true
}

// FetchSubnotery retrieves a subnotery by ID from the database.
// Returns the subnotery and true on success.
// On failure, sends 404 or 500 response and returns nil, false.
func FetchSubnotery(c *gin.Context, db *gorm.DB, subnoteryID uint64) (*models.Subnotery, bool) {
	var subnotery models.Subnotery
	if err := db.First(&subnotery, subnoteryID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Subnotery not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subnotery"})
		}
		return nil, false
	}
	return &subnotery, true
}

// FetchSubnoteryByName retrieves a subnotery by name from the database.
// Returns the subnotery and true on success.
// On failure (not found), returns nil, false without sending a response.
// Caller decides how to handle missing subnotery (e.g., create it).
func FetchSubnoteryByName(db *gorm.DB, name string) (*models.Subnotery, bool) {
	var subnotery models.Subnotery
	if err := db.Where("name = ?", name).First(&subnotery).Error; err != nil {
		return nil, false
	}
	return &subnotery, true
}

// ----- USER HELPERS -----

// FetchUser retrieves a user by ID from the database.
// Returns the user and true on success.
// On failure, sends 404 or 500 response and returns nil, false.
func FetchUser(c *gin.Context, db *gorm.DB, userID uint64) (*models.User, bool) {
	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user"})
		}
		return nil, false
	}
	return &user, true
}

// FetchUserByEmail retrieves a user by email from the database.
// Returns the user and true on success, nil and false if not found.
func FetchUserByEmail(db *gorm.DB, email string) (*models.User, bool) {
	var user models.User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, false
	}
	return &user, true
}

// GetAdminType extracts the admin type flag from context (set by admin middleware).
// Returns true if global admin, false otherwise.
func GetAdminType(c *gin.Context) bool {
	return c.MustGet("admin_type").(bool)
}
