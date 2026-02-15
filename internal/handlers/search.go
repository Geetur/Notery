// search.go — Full-text search across notes, subnoteries, users, and comments.
//
// Provides a Reddit-style unified search endpoint where the caller can toggle
// between result types via the "type" query parameter.
//
// Search types:
//   - notes (default): Meilisearch-backed full-text search on approved notes.
//   - subnoteries:     Database ILIKE search on subnotery names.
//   - users:           Database ILIKE search on username / display_name.
//   - comments:        Database ILIKE search on comment body (approved notes only).
package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/meilisearch/meilisearch-go"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// searchLog is the domain-specific logger for search operations.
var searchLog = helpers.SearchLog

// SearchType enumerates the allowed search categories.
type SearchType string

const (
	SearchNotes       SearchType = "notes"
	SearchSubnoteries SearchType = "subnoteries"
	SearchUsers       SearchType = "users"
	SearchComments    SearchType = "comments"
)

// validSearchType returns true if the given type is recognized.
func validSearchType(t SearchType) bool {
	switch t {
	case SearchNotes, SearchSubnoteries, SearchUsers, SearchComments:
		return true
	}
	return false
}

// Search handles the unified search endpoint.
//
// Query params:
//   - q:    search query (required, min 1 char after trimming)
//   - type: notes | subnoteries | users | comments (default: notes)
//   - page: page number (default 1)
//   - limit: results per page (default 25, max 100)
//
// Route: GET /api/v1/search
func (app *App) SearchAll(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Search query is required"})
		return
	}

	searchType := SearchType(c.DefaultQuery("type", string(SearchNotes)))
	if !validSearchType(searchType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":       "Invalid search type",
			"valid_types": []string{"notes", "subnoteries", "users", "comments"},
		})
		return
	}

	pag := helpers.ParsePagination(c)

	searchLog.Log("SEARCH", "processing", "query", query, "type", string(searchType), "page", pag.Page)

	switch searchType {
	case SearchNotes:
		app.searchNotes(c, query, pag)
	case SearchSubnoteries:
		app.searchSubnoteries(c, query, pag)
	case SearchUsers:
		app.searchUsers(c, query, pag)
	case SearchComments:
		app.searchComments(c, query, pag)
	}
}

// searchNotes queries approved notes via Meilisearch.
func (app *App) searchNotes(c *gin.Context, query string, pag helpers.Pagination) {
	if app.Search == nil || app.SearchIndex == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Search is not configured"})
		return
	}

	index := app.Search.Index(app.SearchIndex)
	results, err := index.Search(query, &meilisearch.SearchRequest{
		Offset: int64(pag.Offset),
		Limit:  int64(pag.Limit),
	})
	if err != nil {
		searchLog.Log("SEARCH", "meilisearch error", "query", query, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Search failed"})
		return
	}

	searchLog.Log("SEARCH", "notes results", "query", query, "hits", results.EstimatedTotalHits)
	c.JSON(http.StatusOK, gin.H{
		"type":    "notes",
		"results": results.Hits,
		"total":   results.EstimatedTotalHits,
		"page":    pag.Page,
		"limit":   pag.Limit,
	})
}

// searchSubnoteries queries subnotery names via database ILIKE.
func (app *App) searchSubnoteries(c *gin.Context, query string, pag helpers.Pagination) {
	pattern := "%" + query + "%"

	var total int64
	app.DB.Model(&models.Subnotery{}).Where("name ILIKE ?", pattern).Count(&total)

	var results []models.Subnotery
	if err := app.DB.Where("name ILIKE ?", pattern).
		Offset(pag.Offset).Limit(pag.Limit).
		Order("name ASC").
		Find(&results).Error; err != nil {
		searchLog.Log("SEARCH", "subnotery db error", "query", query, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Search failed"})
		return
	}

	searchLog.Log("SEARCH", "subnotery results", "query", query, "count", len(results))
	c.JSON(http.StatusOK, gin.H{
		"type":    "subnoteries",
		"results": results,
		"total":   total,
		"page":    pag.Page,
		"limit":   pag.Limit,
	})
}

// searchUsers queries users by username or display name via database ILIKE.
// Only returns public profile data — never leaks email or hash.
func (app *App) searchUsers(c *gin.Context, query string, pag helpers.Pagination) {
	pattern := "%" + query + "%"

	var total int64
	app.DB.Model(&models.User{}).
		Where("username ILIKE ? OR display_name ILIKE ?", pattern, pattern).
		Count(&total)

	var users []models.User
	if err := app.DB.Where("username ILIKE ? OR display_name ILIKE ?", pattern, pattern).
		Offset(pag.Offset).Limit(pag.Limit).
		Order("username ASC").
		Find(&users).Error; err != nil {
		searchLog.Log("SEARCH", "user db error", "query", query, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Search failed"})
		return
	}

	// Map to public profiles to avoid leaking sensitive data.
	profiles := make([]map[string]interface{}, len(users))
	for i, u := range users {
		profiles[i] = u.PublicProfile()
	}

	searchLog.Log("SEARCH", "user results", "query", query, "count", len(profiles))
	c.JSON(http.StatusOK, gin.H{
		"type":    "users",
		"results": profiles,
		"total":   total,
		"page":    pag.Page,
		"limit":   pag.Limit,
	})
}

// searchComments queries comment bodies via database ILIKE.
// Only searches comments on approved notes. Returns comment metadata, not full trees.
func (app *App) searchComments(c *gin.Context, query string, pag helpers.Pagination) {
	pattern := "%" + query + "%"

	var total int64
	app.DB.Model(&models.Comment{}).
		Joins("JOIN notes ON notes.id = comments.note_id").
		Where("notes.status = ? AND comments.is_deleted = ? AND comments.body ILIKE ?",
			models.StatusApproved, false, pattern).
		Count(&total)

	var comments []models.Comment
	if err := app.DB.
		Joins("JOIN notes ON notes.id = comments.note_id").
		Where("notes.status = ? AND comments.is_deleted = ? AND comments.body ILIKE ?",
			models.StatusApproved, false, pattern).
		Offset(pag.Offset).Limit(pag.Limit).
		Order("comments.created_at DESC").
		Find(&comments).Error; err != nil {
		searchLog.Log("SEARCH", "comment db error", "query", query, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Search failed"})
		return
	}

	// Fetch usernames for results.
	userMap := app.fetchCommentUsernames(comments)

	type CommentResult struct {
		ID        uint   `json:"id"`
		NoteID    uint   `json:"note_id"`
		UserID    uint64 `json:"user_id"`
		Username  string `json:"username"`
		Body      string `json:"body"`
		Depth     int    `json:"depth"`
		Upvotes   int64  `json:"upvotes"`
		Downvotes int64  `json:"downvotes"`
		CreatedAt string `json:"created_at"`
	}

	results := make([]CommentResult, len(comments))
	for i, cm := range comments {
		username := "User"
		if name, ok := userMap[cm.UserID]; ok {
			username = name
		}
		results[i] = CommentResult{
			ID:        cm.ID,
			NoteID:    cm.NoteID,
			UserID:    cm.UserID,
			Username:  username,
			Body:      cm.Body,
			Depth:     cm.Depth,
			Upvotes:   cm.Upvotes,
			Downvotes: cm.Downvotes,
			CreatedAt: cm.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	searchLog.Log("SEARCH", "comment results", "query", query, "count", len(results))
	c.JSON(http.StatusOK, gin.H{
		"type":    "comments",
		"results": results,
		"total":   total,
		"page":    pag.Page,
		"limit":   pag.Limit,
	})
}
