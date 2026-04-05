// search.go — Full-text search across notes, subnoteries, users, and comments.
//
// ENDPOINTS:
//
//	GET /search?q=&type=&page=&limit=      Unified multi-type search
//	POST /admin/resync-search-index        Admin-only Meilisearch resync
//
// DESIGN:
//
//	Provides a Reddit-style unified search endpoint where the caller toggles
//	between result types via the "type" query parameter.
//
//	Search types:
//	  - notes (default): Meilisearch-backed full-text search on approved notes.
//	                     Falls back to DB with pg_trgm fuzzy matching.
//	  - subnoteries:     Database ILIKE + pg_trgm search on subnotery names.
//	  - users:           Database ILIKE + pg_trgm search on username / display_name.
//	  - comments:        Database ILIKE + pg_trgm search on comment body (approved notes only).
//
//	Note search delegates to Meilisearch for relevance-ranked results when available.
//	If Meilisearch is down or not configured, all types fall back to PostgreSQL
//	with combined ILIKE (exact substring) + pg_trgm similarity (typo-tolerant)
//	matching. All responses are paginated with {type, results, total, page, limit}.
package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/meilisearch/meilisearch-go"
	"gorm.io/gorm/clause"

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

// SearchSort enumerates the allowed sort options for search results.
type SearchSort string

const (
	SortRelevance     SearchSort = "relevance"
	SortHot           SearchSort = "hot"
	SortNew           SearchSort = "new"
	SortTop           SearchSort = "top"
	SortComments      SearchSort = "comments"
	SortControversial SearchSort = "controversial"
)

// validSearchSort returns true if the given sort option is recognized.
func validSearchSort(s SearchSort) bool {
	switch s {
	case SortRelevance, SortHot, SortNew, SortTop, SortComments, SortControversial:
		return true
	}
	return false
}

// Search handles the unified search endpoint.
//
// Dispatches to type-specific search functions based on the "type" query param.
// All results follow the same paginated envelope: {type, results, total, page, limit}.
//
// DB: Depends on search type — see individual search functions.
// Technologies: PostgreSQL (GORM ILIKE), Meilisearch (full-text for notes).
// Helpers: helpers.ParsePagination.
//
// Query params:
//   - q:     search query (required, min 1 char after trimming)
//   - type:  notes | subnoteries | users | comments (default: notes)
//   - page:  page number (default 1)
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

	sort := SearchSort(c.DefaultQuery("sort", string(SortRelevance)))
	if !validSearchSort(sort) {
		sort = SortRelevance
	}

	searchLog.Log("SEARCH", "processing", "query", query, "type", string(searchType), "sort", string(sort), "page", pag.Page)

	switch searchType {
	case SearchNotes:
		app.searchNotes(c, query, pag, sort)
	case SearchSubnoteries:
		app.searchSubnoteries(c, query, pag, sort)
	case SearchUsers:
		app.searchUsers(c, query, pag, sort)
	case SearchComments:
		app.searchComments(c, query, pag, sort)
	}
}

// searchNotes queries approved notes via Meilisearch full-text search.
// For comment-count sorting, falls back to a database query since Meilisearch
// does not store comment counts.
//
// DB: Meilisearch (default) or PostgreSQL (comment-count fallback).
// Technologies: Meilisearch (offset/limit pagination), GORM.
func (app *App) searchNotes(c *gin.Context, query string, pag helpers.Pagination, sort SearchSort) {
	// Comment-count and controversial sorts require DB computation — fall back to DB search.
	if sort == SortComments || sort == SortControversial {
		app.searchNotesDB(c, query, pag, sort)
		return
	}

	if app.Search == nil || app.SearchIndex == "" {
		// Meilisearch unavailable — fall back to DB for all sorts.
		app.searchNotesDB(c, query, pag, sort)
		return
	}

	var meiliSort []string
	switch sort {
	case SortHot:
		meiliSort = []string{"hotness:desc"}
	case SortNew:
		meiliSort = []string{"created_at:desc"}
	case SortTop:
		meiliSort = []string{"upvotes:desc"}
	}

	index := app.Search.Index(app.SearchIndex)
	results, err := index.Search(query, &meilisearch.SearchRequest{
		Offset: int64(pag.Offset),
		Limit:  int64(pag.Limit),
		Sort:   meiliSort,
	})
	if err != nil {
		searchLog.Log("SEARCH", "meilisearch error, falling back to DB", "query", query, "error", err)
		app.searchNotesDB(c, query, pag, sort)
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

// searchNotesDB searches approved notes via PostgreSQL ILIKE + pg_trgm fuzzy
// matching as a fallback when Meilisearch is unavailable or when the sort
// requires DB computation (e.g., comment count).
//
// Uses a hybrid approach: ILIKE catches exact substring matches, and pg_trgm
// similarity() catches typo-tolerant matches (threshold 0.2). Results are
// ranked by trigram similarity when no explicit sort is provided.
//
// DB: COUNT + SELECT from notes WHERE status=Approved AND fuzzy match.
// Technologies: PostgreSQL (GORM ILIKE + pg_trgm similarity).
func (app *App) searchNotesDB(c *gin.Context, query string, pag helpers.Pagination, sort SearchSort) {
	pattern := "%" + query + "%"
	const threshold = 0.2

	whereClause := `status = ? AND (
		title ILIKE ? OR author ILIKE ? OR description ILIKE ? OR
		similarity(title, ?) > ? OR similarity(author, ?) > ? OR similarity(description, ?) > ?
	)`
	whereArgs := []interface{}{
		models.StatusApproved,
		pattern, pattern, pattern,
		query, threshold, query, threshold, query, threshold,
	}

	var total int64
	app.DB.Model(&models.Note{}).
		Where(whereClause, whereArgs...).
		Count(&total)

	q := app.DB.Where(whereClause, whereArgs...)

	switch sort {
	case SortComments:
		q = q.Select("notes.*, (SELECT COUNT(*) FROM comments WHERE comments.note_id = notes.id AND comments.is_deleted = false) as comment_count").
			Order("comment_count DESC")
	case SortHot:
		q = q.Order("hotness DESC")
	case SortNew:
		q = q.Order("created_at DESC")
	case SortTop:
		q = q.Order("(upvotes - downvotes) DESC")
	case SortControversial:
		q = q.Order("(upvotes + downvotes) * 1.0 / CASE WHEN ABS(CAST(upvotes AS INTEGER) - CAST(downvotes AS INTEGER)) < 1 THEN 1 ELSE ABS(CAST(upvotes AS INTEGER) - CAST(downvotes AS INTEGER)) END DESC, created_at DESC")
	default:
		q = q.Clauses(clause.OrderBy{
			Expression: clause.Expr{
				SQL:  "GREATEST(similarity(title, ?), similarity(author, ?), similarity(description, ?)) DESC, created_at DESC",
				Vars: []interface{}{query, query, query},
			},
		})
	}

	var notes []models.Note
	if err := q.Offset(pag.Offset).Limit(pag.Limit).Find(&notes).Error; err != nil {
		searchLog.Log("SEARCH", "notes db error", "query", query, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Search failed"})
		return
	}

	// Populate subnotery names for display
	app.populateSubnoteryNames(notes)

	// Populate comment counts
	app.populateCommentCounts(notes)

	searchLog.Log("SEARCH", "notes db results", "query", query, "count", len(notes))
	c.JSON(http.StatusOK, gin.H{
		"type":    "notes",
		"results": notes,
		"total":   total,
		"page":    pag.Page,
		"limit":   pag.Limit,
	})
}

// searchSubnoteries queries subnotery names via database ILIKE + pg_trgm fuzzy matching.
//
// DB: COUNT + SELECT from subnoteries WHERE name matches. Paginated with OFFSET/LIMIT.
// Technologies: PostgreSQL (GORM ILIKE + pg_trgm similarity).
func (app *App) searchSubnoteries(c *gin.Context, query string, pag helpers.Pagination, sort SearchSort) {
	pattern := "%" + query + "%"
	const threshold = 0.2
	whereClause := "name ILIKE ? OR similarity(name, ?) > ?"
	whereArgs := []interface{}{pattern, query, threshold}

	var total int64
	app.DB.Model(&models.Subnotery{}).Where(whereClause, whereArgs...).Count(&total)

	q := app.DB.Where(whereClause, whereArgs...)

	switch sort {
	case SortNew:
		q = q.Order("created_at DESC")
	case SortHot, SortTop:
		// Sort by member count as popularity proxy
		q = q.Order("(SELECT COUNT(*) FROM subnotery_members WHERE subnotery_members.subnotery_id = subnoteries.id) DESC")
	default:
		q = q.Clauses(clause.OrderBy{
			Expression: clause.Expr{
				SQL:  "similarity(name, ?) DESC, name ASC",
				Vars: []interface{}{query},
			},
		})
	}

	var results []models.Subnotery
	if err := q.Offset(pag.Offset).Limit(pag.Limit).
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

// searchUsers queries users by username or display name via database ILIKE + pg_trgm.
// Only returns public profile data (via User.PublicProfile()) — never leaks email or hash.
//
// DB: COUNT + SELECT from users WHERE username/display_name matches. Paginated.
// Technologies: PostgreSQL (GORM ILIKE + pg_trgm similarity).
func (app *App) searchUsers(c *gin.Context, query string, pag helpers.Pagination, sort SearchSort) {
	pattern := "%" + query + "%"
	const threshold = 0.2
	whereClause := "username ILIKE ? OR display_name ILIKE ? OR similarity(username, ?) > ? OR similarity(display_name, ?) > ?"
	whereArgs := []interface{}{pattern, pattern, query, threshold, query, threshold}

	var total int64
	app.DB.Model(&models.User{}).
		Where(whereClause, whereArgs...).
		Count(&total)

	q := app.DB.Where(whereClause, whereArgs...)

	switch sort {
	case SortNew:
		q = q.Order("created_at DESC")
	case SortHot, SortTop:
		q = q.Order("created_at DESC")
	default:
		q = q.Clauses(clause.OrderBy{
			Expression: clause.Expr{
				SQL:  "GREATEST(similarity(username, ?), similarity(display_name, ?)) DESC, username ASC",
				Vars: []interface{}{query, query},
			},
		})
	}

	var users []models.User
	if err := q.Offset(pag.Offset).Limit(pag.Limit).
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

// searchComments queries comment bodies via database ILIKE + pg_trgm fuzzy matching.
// Only searches non-deleted comments on approved notes. Returns comment metadata
// with resolved usernames, not full threaded trees.
//
// DB: COUNT + SELECT from comments JOIN notes WHERE status=Approved AND body matches.
//
//	Fetches usernames via fetchCommentUsernames helper. Paginated.
//
// Technologies: PostgreSQL (GORM ILIKE + pg_trgm similarity + JOIN).
func (app *App) searchComments(c *gin.Context, query string, pag helpers.Pagination, sort SearchSort) {
	pattern := "%" + query + "%"
	const threshold = 0.2
	bodyWhere := "notes.status = ? AND comments.is_deleted = ? AND (comments.body ILIKE ? OR similarity(comments.body, ?) > ?)"
	bodyArgs := []interface{}{models.StatusApproved, false, pattern, query, threshold}

	var total int64
	app.DB.Model(&models.Comment{}).
		Joins("JOIN notes ON notes.id = comments.note_id").
		Where(bodyWhere, bodyArgs...).
		Count(&total)

	q := app.DB.
		Joins("JOIN notes ON notes.id = comments.note_id").
		Where(bodyWhere, bodyArgs...)

	switch sort {
	case SortHot, SortTop:
		q = q.Order("(comments.upvotes - comments.downvotes) DESC")
	case SortNew:
		q = q.Order("comments.created_at DESC")
	default:
		q = q.Clauses(clause.OrderBy{
			Expression: clause.Expr{
				SQL:  "similarity(comments.body, ?) DESC, comments.created_at DESC",
				Vars: []interface{}{query},
			},
		})
	}

	var comments []models.Comment
	if err := q.Offset(pag.Offset).Limit(pag.Limit).
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

// ResyncSearchIndex re-indexes all approved notes into Meilisearch.
// This is an admin-only endpoint used to recover from Meilisearch outages
// or to repair index drift after notes were approved while Meilisearch was down.
//
// Loads all approved notes from the database (with SubnoteryName populated),
// and batch-inserts them into Meilisearch. This is idempotent — existing
// documents with the same ID are updated in place.
//
// DB: SELECT all approved notes + subnotery names.
// Technologies: PostgreSQL (GORM), Meilisearch (AddDocuments batch).
//
// Route: POST /api/v1/admin/resync-search-index
func (app *App) ResyncSearchIndex(c *gin.Context) {
	if app.Search == nil || app.SearchIndex == "" {
		searchLog.Log("RESYNC", "Meilisearch not configured — cannot resync")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Meilisearch is not configured"})
		return
	}

	var notes []models.Note
	if err := app.DB.Where("status = ?", models.StatusApproved).Find(&notes).Error; err != nil {
		searchLog.Log("RESYNC", "Failed to load approved notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load notes for resync"})
		return
	}

	// Populate SubnoteryName for all notes
	app.populateSubnoteryNames(notes)

	if len(notes) == 0 {
		searchLog.Log("RESYNC", "No approved notes to index")
		c.JSON(http.StatusOK, gin.H{"message": "No approved notes to index", "indexed": 0})
		return
	}

	index := app.Search.Index(app.SearchIndex)
	_, err := index.AddDocuments(notes, &meilisearch.DocumentOptions{
		PrimaryKey: meilisearch.StringPtr("id"),
	})
	if err != nil {
		searchLog.Log("RESYNC", "Failed to index notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to index notes in Meilisearch"})
		return
	}

	searchLog.Log("RESYNC", "Search index resync complete", "indexed", len(notes))
	c.JSON(http.StatusOK, gin.H{"message": "Search index resync complete", "indexed": len(notes)})
}

// ResyncSearchIndexBackground re-indexes all approved notes into Meilisearch
// in the background. Called at startup to catch any notes approved while
// Meilisearch was down. This is a fire-and-forget operation.
func (app *App) ResyncSearchIndexBackground() {
	if app.Search == nil || app.SearchIndex == "" {
		searchLog.Log("RESYNC", "Meilisearch not configured — skipping startup resync")
		return
	}

	var notes []models.Note
	if err := app.DB.Where("status = ?", models.StatusApproved).Find(&notes).Error; err != nil {
		searchLog.Log("RESYNC", "Background resync failed to load notes", "error", err)
		return
	}

	app.populateSubnoteryNames(notes)

	if len(notes) == 0 {
		searchLog.Log("RESYNC", "No approved notes to resync at startup")
		return
	}

	index := app.Search.Index(app.SearchIndex)
	_, err := index.AddDocuments(notes, &meilisearch.DocumentOptions{
		PrimaryKey: meilisearch.StringPtr("id"),
	})
	if err != nil {
		searchLog.Log("RESYNC", "Background resync failed to index", "error", err)
		return
	}

	searchLog.Log("RESYNC", "Startup resync complete", "indexed", len(notes))
}
