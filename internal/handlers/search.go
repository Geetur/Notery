// Package handlers/search.go contains the HTTP handler for full-text search.
//
// ARCHITECTURE:
//
//	Notes are indexed in Meilisearch when they are approved (see note.go ApproveNote).
//	This handler exposes the Meilisearch index to frontend users with pagination,
//	filtering, and sorting.
//
//	Only approved notes appear in search results (enforced at index time — only
//	approved notes are ever indexed).
//
// QUERY PARAMS:
//
//	q        — search query (required)
//	page     — page number (default 1)
//	limit    — results per page (default 25, max 100)
//	sort     — sort field: "relevance" (default), "price_asc", "price_desc", "newest", "hottest"
//	min_price — minimum price filter in cents (optional)
//	max_price — maximum price filter in cents (optional)
//	subnotery — filter by subnotery name (optional)
package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/meilisearch/meilisearch-go"

	"github.com/Geetur/Notery/internal/helpers"
)

// searchLog is the domain-specific logger for search operations.
var searchLog = helpers.NewLogger("SEARCH")

// SearchNotes performs a full-text search across approved notes via Meilisearch.
//
// Route: GET /api/v1/search
func (app *App) SearchNotes(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Search query 'q' is required"})
		return
	}

	if app.Search == nil || app.SearchIndex == "" {
		searchLog.Log("SEARCH", "Meilisearch not configured")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Search is not available"})
		return
	}

	pg := helpers.ParsePagination(c)
	searchLog.Log("SEARCH", "Processing search request", "query", query, "page", pg.Page, "limit", pg.Limit)

	// Build search request
	searchReq := &meilisearch.SearchRequest{
		Limit:  int64(pg.Limit),
		Offset: int64(pg.Offset),
	}

	// Apply sort
	sort := c.DefaultQuery("sort", "relevance")
	switch sort {
	case "price_asc":
		searchReq.Sort = []string{"price:asc"}
	case "price_desc":
		searchReq.Sort = []string{"price:desc"}
	case "newest":
		searchReq.Sort = []string{"created_at:desc"}
	case "hottest":
		searchReq.Sort = []string{"hotness:desc"}
	case "relevance":
		// Default Meilisearch relevance ranking — no explicit sort
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error":         "Invalid sort parameter",
			"valid_options": []string{"relevance", "price_asc", "price_desc", "newest", "hottest"},
		})
		return
	}

	// Build filters
	var filters []string

	// Price range filters
	if minPrice := c.Query("min_price"); minPrice != "" {
		if v, err := strconv.ParseInt(minPrice, 10, 64); err == nil && v >= 0 {
			filters = append(filters, "price >= "+strconv.FormatInt(v, 10))
		}
	}
	if maxPrice := c.Query("max_price"); maxPrice != "" {
		if v, err := strconv.ParseInt(maxPrice, 10, 64); err == nil && v >= 0 {
			filters = append(filters, "price <= "+strconv.FormatInt(v, 10))
		}
	}

	// Subnotery filter (by subnotery_id)
	if subnoteryID := c.Query("subnotery_id"); subnoteryID != "" {
		if v, err := strconv.ParseUint(subnoteryID, 10, 64); err == nil && v > 0 {
			filters = append(filters, "subnotery_id = "+strconv.FormatUint(v, 10))
		}
	}

	// Combine filters with AND
	if len(filters) > 0 {
		combined := filters[0]
		for _, f := range filters[1:] {
			combined += " AND " + f
		}
		searchReq.Filter = combined
	}

	// Execute search
	index := app.Search.Index(app.SearchIndex)
	result, err := index.Search(query, searchReq)
	if err != nil {
		searchLog.Log("SEARCH", "Meilisearch query failed", "error", err, "query", query)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Search failed"})
		return
	}

	searchLog.Log("SEARCH", "Search completed", "query", query, "hits", result.EstimatedTotalHits, "returned", len(result.Hits))
	c.JSON(http.StatusOK, gin.H{
		"query":   query,
		"hits":    result.Hits,
		"total":   result.EstimatedTotalHits,
		"page":    pg.Page,
		"limit":   pg.Limit,
		"sort":    sort,
	})
}
