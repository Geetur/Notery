// comment.go — HTTP handlers for the Reddit-style threaded comment system.
//
// ENDPOINTS:
//
//	GET    /notes/:id/comments          List threaded comments for a note (paginated roots, full tree)
//	POST   /notes/:id/comments          Create a new comment or reply
//	GET    /comments/:comment_id        Get a single comment with its reply subtree
//	PUT    /comments/:comment_id        Edit own comment
//	DELETE /comments/:comment_id        Soft-delete own comment (admin: any comment)
//	POST   /comments/:comment_id/vote   Vote on a comment (+1 upvote, -1 downvote, toggle)
//	DELETE /comments/:comment_id/vote   Remove vote from a comment
//	GET    /me/comments                  List own comments (flat, paginated)
//
// DESIGN:
//
//	Comments form a tree rooted at notes. Listing uses a two-phase read model:
//	Phase 1 fetches only root comments (parent_id IS NULL) with DB-level pagination/sort.
//	Phase 2 fetches descendants for those roots only, using materialized-path queries
//	when available, with automatic fallback to parent-chain filtering.
//	Total returned nodes are capped at MaxNodesPerRequest (500) per request.
//
//	Subtree fetches (GetComment) use materialized paths for exact descendant queries,
//	eliminating false positives from the legacy depth-range approach.
//
//	Ranking uses Wilson score lower bound with NO time decay — quality always wins.
//	Vote counts and Wilson scores are updated atomically inside a DB transaction
//	with SELECT … FOR UPDATE to prevent race conditions on concurrent votes.
//
// TREE ALGORITHM:
//
//  1. Phase 1: Fetch paginated roots with DB-level sort (score/created_at).
//  2. Phase 2: Fetch descendants for those roots (path LIKE or parent-chain filter).
//  3. Batch-fetch the current user's votes and all commenters' usernames.
//  4. Build a map[commentID] → *CommentResponse.
//  5. Wire parent→children pointers.
//  6. Sort children recursively by the requested sort order.
//  7. Truncate beyond MaxTreeDepth, setting has_more_replies.
package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// commentLog is the domain-specific logger for comment operations.
var commentLog = helpers.CommentLog

// maxCommentsPerNote is the hard cap on comments fetched per note in one request.
// Prevents unbounded memory use. For a notes marketplace this is very generous.
// If a note exceeds this, the response includes a truncation warning.
const maxCommentsPerNote = 2000

// ----- RESPONSE TYPES -----

// CommentResponse is the JSON representation of a comment in API responses.
// It includes computed fields (user_vote, is_edited, children) not stored directly.
type CommentResponse struct {
	ID        uint               `json:"id"`
	NoteID    uint               `json:"note_id"`
	UserID    uint64             `json:"user_id"`
	Username  string             `json:"username"`
	ParentID  *uint              `json:"parent_id"`
	Body      string             `json:"body"`
	Upvotes   int64              `json:"upvotes"`
	Downvotes int64              `json:"downvotes"`
	Score     float64            `json:"score"`
	Depth     int                `json:"depth"`
	IsDeleted bool               `json:"is_deleted"`
	IsEdited  bool               `json:"is_edited"`
	CreatedAt time.Time          `json:"created_at"`
	UserVote  int8               `json:"user_vote"`
	Children  []*CommentResponse `json:"children"`
	HasMore   bool               `json:"has_more_replies,omitempty"`
}

// ----- GET /notes/:id/comments -----

// GetNoteComments returns the threaded comment tree for a note.
// Top-level comments are paginated; descendants are fetched only for the
// current page's roots, keeping cost proportional to what the user actually views.
//
// Two-phase read model:
//  1. Query only root comments (parent_id IS NULL) with DB-level pagination + sort.
//  2. Fetch descendants for exactly those root IDs within the depth window.
//  3. Assemble trees in memory only for the current page.
//
// Node budget: total returned nodes capped at MaxNodesPerRequest (500).
//
// Query params:
//
//	sort      — best (default), new, top, controversial, old
//	page      — page number (default 1)
//	limit     — page size (default 25, max 100)
//	max_depth — tree depth limit (default 10, max 20)
//
// Route: GET /api/v1/notes/:id/comments
func (app *App) GetNoteComments(c *gin.Context) {
	noteID, ok := helpers.MustParseNoteID(c)
	if !ok {
		return
	}

	// Parse sort order
	sortOrder := models.CommentSortOrder(c.DefaultQuery("sort", string(models.SortBest)))
	if !models.ValidSortOrder(sortOrder) {
		sortOrder = models.SortBest
	}

	pag := helpers.ParsePagination(c)

	maxDepth := models.MaxTreeDepth
	if d, err := strconv.Atoi(c.DefaultQuery("max_depth", strconv.Itoa(models.MaxTreeDepth))); err == nil && d > 0 && d <= 20 {
		maxDepth = d
	}

	// Verify note exists and is approved (comments only on published notes)
	var note models.Note
	if err := app.DB.First(&note, noteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	if note.Status == models.StatusApproved {
		// Anyone can view comments on approved notes
	} else if note.Status == models.StatusPending {
		// Only admins can view comments on pending notes
		userID, authenticated := helpers.TryGetUserID(c)
		if !authenticated || !app.isNoteAdmin(userID, note.SubnoteryID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Comments are not available for this note"})
			return
		}
	} else {
		c.JSON(http.StatusForbidden, gin.H{"error": "Comments are not available for this note"})
		return
	}

	// Count total top-level comments (for pagination metadata)
	var totalTopLevel int64
	if err := app.DB.Model(&models.Comment{}).
		Where("note_id = ? AND parent_id IS NULL", noteID).
		Count(&totalTopLevel).Error; err != nil {
		commentLog.Log("LIST", "count top-level error", "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count comments"})
		return
	}

	// ----- PHASE 1: Fetch only roots for this page with DB-level sort + pagination -----
	rootQuery := app.DB.Where("note_id = ? AND parent_id IS NULL", noteID)
	switch sortOrder {
	case models.SortBest, models.SortHot:
		rootQuery = rootQuery.Order("score DESC, created_at ASC, id ASC")
	case models.SortNew:
		rootQuery = rootQuery.Order("created_at DESC, id DESC")
	case models.SortOld:
		rootQuery = rootQuery.Order("created_at ASC, id ASC")
	case models.SortTop:
		rootQuery = rootQuery.Order("(upvotes - downvotes) DESC, created_at ASC, id ASC")
	case models.SortControversial:
		// Keep root ordering consistent with models.ControversyScore used for children.
		rootQuery = rootQuery.Order(
			"(upvotes + downvotes) * 1.0 / CASE WHEN ABS(upvotes - downvotes) < 1 THEN 1 ELSE ABS(upvotes - downvotes) END DESC, created_at ASC, id ASC",
		)
	default:
		rootQuery = rootQuery.Order("score DESC, created_at ASC, id ASC")
	}
	var roots []models.Comment
	if err := rootQuery.Offset(pag.Offset).Limit(pag.Limit).Find(&roots).Error; err != nil {
		commentLog.Log("LIST", "fetch roots error", "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
		return
	}

	if len(roots) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"comments":  []*CommentResponse{},
			"total":     totalTopLevel,
			"page":      pag.Page,
			"limit":     pag.Limit,
			"sort":      sortOrder,
			"truncated": false,
		})
		return
	}

	// ----- PHASE 2: Fetch descendants for this page's root IDs -----
	rootIDs := make([]uint, len(roots))
	for i, r := range roots {
		rootIDs[i] = r.ID
	}

	// Budget: MaxNodesPerRequest minus the roots we already have
	descendantBudget := models.MaxNodesPerRequest - len(roots)
	if descendantBudget < 0 {
		descendantBudget = 0
	}

	descendants, truncated, err := app.fetchDescendantsForRoots(noteID, rootIDs, maxDepth, descendantBudget)
	if err != nil {
		commentLog.Log("LIST", "fetch descendants error", "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
		return
	}

	// Merge roots + descendants
	allComments := make([]models.Comment, 0, len(roots)+len(descendants))
	allComments = append(allComments, roots...)
	allComments = append(allComments, descendants...)

	// Batch-fetch usernames for all commenters
	userMap := app.fetchCommentUsernames(allComments)

	// Batch-fetch current user's votes (if authenticated)
	var userVotes map[uint]int8
	if userID, authenticated := helpers.TryGetUserID(c); authenticated {
		userVotes = app.fetchUserCommentVotes(userID, allComments)
	}

	// Build response map + wire children + sort + truncate
	responseMap := buildResponseMap(allComments, userMap, userVotes)
	wireChildren(responseMap)
	truncateDepth(responseMap, maxDepth)

	// Collect root responses in DB-sorted order (already sorted by phase 1 query)
	rootResponses := make([]*CommentResponse, 0, len(roots))
	for _, r := range roots {
		if resp, ok := responseMap[r.ID]; ok {
			rootResponses = append(rootResponses, resp)
		}
	}

	// Sort children recursively (roots are already in DB sort order)
	for _, root := range rootResponses {
		sortTree(root.Children, sortOrder)
	}

	commentLog.Log("LIST", "served", "note_id", noteID, "total_top", totalTopLevel,
		"page", pag.Page, "sort", string(sortOrder),
		"roots", len(roots), "descendants", len(descendants), "truncated", truncated)

	c.JSON(http.StatusOK, gin.H{
		"comments":  rootResponses,
		"total":     totalTopLevel,
		"page":      pag.Page,
		"limit":     pag.Limit,
		"sort":      sortOrder,
		"truncated": truncated,
	})
}

// ----- POST /notes/:id/comments -----

// CreateComment creates a new top-level comment or reply on a note.
//
// Request body:
//
//	{
//	  "body":      "Comment text (required, max 10000 chars)",
//	  "parent_id": 42  // optional — omit or null for top-level
//	}
//
// Route: POST /api/v1/notes/:id/comments
func (app *App) CreateComment(c *gin.Context) {
	noteID, ok := helpers.MustParseNoteID(c)
	if !ok {
		return
	}
	userID := helpers.GetUserID(c)

	var req struct {
		Body     string `json:"body" binding:"required"`
		ParentID *uint  `json:"parent_id"`
	}
	if !helpers.BindJSON(c, &req) {
		return
	}

	// Normalize: trim leading/trailing whitespace; reject whitespace-only bodies
	req.Body = strings.TrimSpace(req.Body)
	if req.Body == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Comment body must not be empty or whitespace-only"})
		return
	}

	// Validate body length
	if utf8.RuneCountInString(req.Body) > models.MaxCommentBodyLength {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Comment body exceeds maximum length",
			"max":   models.MaxCommentBodyLength,
		})
		return
	}

	// Verify note exists and is approved (or pending — admins can comment on pending)
	var note models.Note
	if err := app.DB.First(&note, noteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	if note.Status == models.StatusApproved {
		// Anyone can comment on approved notes — proceed
	} else if note.Status == models.StatusPending {
		// Only admins (global or subnotery-scoped) may comment on pending notes
		if !app.isNoteAdmin(userID, note.SubnoteryID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot comment on this note"})
			return
		}
	} else {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot comment on this note"})
		return
	}

	// Determine depth and parent path (if replying)
	depth := 0
	parentPath := "" // materialized path of parent, empty for top-level
	if req.ParentID != nil {
		var parent models.Comment
		if err := app.DB.First(&parent, *req.ParentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Parent comment not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		// Parent must belong to the same note
		if parent.NoteID != uint(noteID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Parent comment does not belong to this note"})
			return
		}
		// Cannot reply to a deleted comment
		if parent.IsDeleted {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot reply to a deleted comment"})
			return
		}
		depth = parent.Depth + 1
		parentPath = parent.Path

		// Enforce maximum write depth to prevent pathologically deep chains (DoS)
		if depth > models.MaxWriteDepth {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":     "Maximum reply depth exceeded",
				"max_depth": models.MaxWriteDepth,
			})
			return
		}
	}

	comment := models.Comment{
		NoteID:   uint(noteID),
		UserID:   userID,
		ParentID: req.ParentID,
		Body:     req.Body,
		Depth:    depth,
	}

	if err := app.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&comment).Error; err != nil {
			return err
		}

		// Compute and persist the materialized path now that we have the auto-generated ID.
		// Top-level: "/<id>/", reply: "<parentPath><id>/"
		if parentPath != "" {
			comment.Path = fmt.Sprintf("%s%d/", parentPath, comment.ID)
		} else {
			comment.Path = fmt.Sprintf("/%d/", comment.ID)
		}
		return tx.Model(&comment).Update("path", comment.Path).Error
	}); err != nil {
		commentLog.Log("CREATE", "db error", "note_id", noteID, "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create comment"})
		return
	}

	// Fetch username for response
	username := app.lookupUsername(userID)

	commentLog.Log("CREATE", "success", "comment_id", comment.ID, "note_id", noteID,
		"user_id", userID, "depth", depth, "parent_id", req.ParentID)

	c.JSON(http.StatusCreated, CommentResponse{
		ID:        comment.ID,
		NoteID:    comment.NoteID,
		UserID:    comment.UserID,
		Username:  username,
		ParentID:  comment.ParentID,
		Body:      comment.Body,
		Upvotes:   0,
		Downvotes: 0,
		Score:     0,
		Depth:     comment.Depth,
		IsDeleted: false,
		IsEdited:  false,
		CreatedAt: comment.CreatedAt,
		UserVote:  0,
		Children:  []*CommentResponse{},
	})
}

// ----- GET /comments/:comment_id -----

// GetComment returns a single comment with its exact reply subtree.
// Used for "Continue this thread →" deep-linking.
//
// Uses materialized path for exact descendant queries when available,
// with automatic fallback to legacy depth-range queries for unbackfilled data.
//
// Query params: max_depth (default 10, max 20), sort (default best)
//
// Route: GET /api/v1/comments/:comment_id
func (app *App) GetComment(c *gin.Context) {
	commentID, ok := helpers.MustParseCommentID(c)
	if !ok {
		return
	}

	sortOrder := models.CommentSortOrder(c.DefaultQuery("sort", string(models.SortBest)))
	if !models.ValidSortOrder(sortOrder) {
		sortOrder = models.SortBest
	}
	maxDepth := models.MaxTreeDepth
	if d, err := strconv.Atoi(c.DefaultQuery("max_depth", strconv.Itoa(models.MaxTreeDepth))); err == nil && d > 0 && d <= 20 {
		maxDepth = d
	}

	// Fetch the target comment
	var target models.Comment
	if err := app.DB.First(&target, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Enforce note visibility — must be approved (same as GetNoteComments).
	var note models.Note
	if err := app.DB.Select("id, status").First(&note, target.NoteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	if note.Status != models.StatusApproved {
		c.JSON(http.StatusForbidden, gin.H{"error": "Comments are not available for this note"})
		return
	}

	relativeMaxDepth := target.Depth + maxDepth
	budget := models.MaxNodesPerRequest

	subtreeComments, truncatedByBudget, usedPathQuery, err := app.fetchCommentSubtree(target, relativeMaxDepth, budget)
	if err != nil {
		commentLog.Log("GET", "subtree fetch error", "comment_id", commentID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
		return
	}

	userMap := app.fetchCommentUsernames(subtreeComments)

	var userVotes map[uint]int8
	if userID, authenticated := helpers.TryGetUserID(c); authenticated {
		userVotes = app.fetchUserCommentVotes(userID, subtreeComments)
	}

	// Build tree from the subtree set
	responseMap := buildResponseMap(subtreeComments, userMap, userVotes)
	wireChildren(responseMap)
	truncateDepth(responseMap, relativeMaxDepth)

	resp, exists := responseMap[uint(commentID)]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
		return
	}
	if truncatedByBudget {
		resp.HasMore = true
	}

	sortTree([]*CommentResponse{resp}, sortOrder)

	commentLog.Log("GET", "served", "comment_id", commentID, "note_id", target.NoteID,
		"nodes", len(subtreeComments), "path_query", usedPathQuery, "truncated", truncatedByBudget)
	c.JSON(http.StatusOK, resp)
}

// ----- PUT /comments/:comment_id -----

// EditComment edits the body of the user's own comment.
// If edited outside the EditGracePeriod (~3 min), sets EditedAt (shows "edited" indicator).
//
// Request body: { "body": "Updated text" }
//
// Route: PUT /api/v1/comments/:comment_id
func (app *App) EditComment(c *gin.Context) {
	commentID, ok := helpers.MustParseCommentID(c)
	if !ok {
		return
	}
	userID := helpers.GetUserID(c)

	var req struct {
		Body string `json:"body" binding:"required"`
	}
	if !helpers.BindJSON(c, &req) {
		return
	}

	// Normalize: trim leading/trailing whitespace; reject whitespace-only bodies
	req.Body = strings.TrimSpace(req.Body)
	if req.Body == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Comment body must not be empty or whitespace-only"})
		return
	}

	if utf8.RuneCountInString(req.Body) > models.MaxCommentBodyLength {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Comment body exceeds maximum length",
			"max":   models.MaxCommentBodyLength,
		})
		return
	}

	var comment models.Comment
	if err := app.DB.First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Ownership check
	if comment.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You can only edit your own comments"})
		return
	}
	if comment.IsDeleted {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot edit a deleted comment"})
		return
	}

	// Determine if edit should show "edited" indicator
	updates := map[string]interface{}{
		"body": req.Body,
	}
	if time.Since(comment.CreatedAt) > models.EditGracePeriod {
		now := time.Now()
		updates["edited_at"] = now
	}

	if err := app.DB.Model(&comment).Updates(updates).Error; err != nil {
		commentLog.Log("EDIT", "db error", "comment_id", commentID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to edit comment"})
		return
	}

	commentLog.Log("EDIT", "success", "comment_id", commentID, "user_id", userID)
	c.JSON(http.StatusOK, gin.H{
		"comment_id": comment.ID,
		"body":       req.Body,
		"is_edited":  comment.EditedAt != nil || time.Since(comment.CreatedAt) > models.EditGracePeriod,
	})
}

// ----- DELETE /comments/:comment_id -----

// DeleteComment soft-deletes a comment. The body is wiped but the tree structure
// is preserved so child replies remain readable (shown as "[deleted]").
// Users can delete their own comments; admins can delete any comment.
//
// Route: DELETE /api/v1/comments/:comment_id
func (app *App) DeleteComment(c *gin.Context) {
	commentID, ok := helpers.MustParseCommentID(c)
	if !ok {
		return
	}
	userID := helpers.GetUserID(c)

	var comment models.Comment
	if err := app.DB.First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if comment.IsDeleted {
		c.JSON(http.StatusOK, gin.H{"message": "Comment already deleted"})
		return
	}

	// Ownership or admin check
	isOwner := comment.UserID == userID
	isGlobalAdmin := false
	isSubnoteryAdmin := false
	if !isOwner {
		var err error
		isGlobalAdmin, isSubnoteryAdmin, err = app.resolveCommentDeletePrivileges(userID, comment.NoteID)
		if err != nil {
			commentLog.Log("DELETE", "role resolution error", "comment_id", commentID, "user_id", userID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to authorize delete"})
			return
		}
	}

	if !isOwner && !isGlobalAdmin && !isSubnoteryAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "You do not have permission to delete this comment"})
		return
	}

	// Soft-delete: clear body, mark as deleted
	if err := app.DB.Model(&comment).Updates(map[string]interface{}{
		"is_deleted": true,
		"body":       "",
	}).Error; err != nil {
		commentLog.Log("DELETE", "db error", "comment_id", commentID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete comment"})
		return
	}

	commentLog.Log("DELETE", "soft-deleted", "comment_id", commentID, "user_id", userID,
		"is_global_admin", isGlobalAdmin, "is_subnotery_admin", isSubnoteryAdmin)
	c.JSON(http.StatusOK, gin.H{"message": "Comment deleted"})
}

// ----- POST /comments/:comment_id/vote -----

// VoteComment handles voting on a comment. Reddit-style toggle behavior:
//   - POST {value: 1}  when no vote    → upvote
//   - POST {value: -1} when no vote    → downvote
//   - POST {value: 1}  when already +1 → toggle off (remove vote)
//   - POST {value: -1} when already -1 → toggle off (remove vote)
//   - POST {value: 1}  when already -1 → switch to upvote
//   - POST {value: -1} when already +1 → switch to downvote
//
// Vote counts and Wilson score are updated atomically inside a transaction
// with SELECT … FOR UPDATE to prevent race conditions.
//
// Route: POST /api/v1/comments/:comment_id/vote
func (app *App) VoteComment(c *gin.Context) {
	commentID, ok := helpers.MustParseCommentID(c)
	if !ok {
		return
	}
	userID := helpers.GetUserID(c)

	var req struct {
		Value int8 `json:"value" binding:"required"`
	}
	if !helpers.BindJSON(c, &req) {
		return
	}
	if req.Value != 1 && req.Value != -1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Value must be 1 (upvote) or -1 (downvote)"})
		return
	}

	var resultComment models.Comment
	var resultVote int8

	err := app.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		// Lock the comment row to prevent concurrent vote races on Wilson score
		var comment models.Comment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&comment, commentID).Error; err != nil {
			return err
		}

		if comment.IsDeleted {
			return errors.New("cannot vote on a deleted comment")
		}

		// Check for existing vote
		var existing models.CommentVote
		err := tx.Where("comment_id = ? AND user_id = ?", commentID, userID).First(&existing).Error

		if err == nil {
			// Existing vote found
			if existing.Value == req.Value {
				// Toggle off: remove vote
				if err := tx.Delete(&existing).Error; err != nil {
					return err
				}
				if req.Value == 1 {
					if err := tx.Model(&comment).Update("upvotes", gorm.Expr("CASE WHEN upvotes > 0 THEN upvotes - 1 ELSE 0 END")).Error; err != nil {
						return err
					}
				} else {
					if err := tx.Model(&comment).Update("downvotes", gorm.Expr("CASE WHEN downvotes > 0 THEN downvotes - 1 ELSE 0 END")).Error; err != nil {
						return err
					}
				}
				resultVote = 0
				commentLog.Log("VOTE", "toggled off", "comment_id", commentID, "user_id", userID, "was", req.Value)
			} else {
				// Switch vote direction
				if err := tx.Model(&existing).Update("value", req.Value).Error; err != nil {
					return err
				}
				if req.Value == 1 {
					// Switching from -1 to +1
					if err := tx.Model(&comment).Updates(map[string]interface{}{
						"upvotes":   gorm.Expr("upvotes + 1"),
						"downvotes": gorm.Expr("CASE WHEN downvotes > 0 THEN downvotes - 1 ELSE 0 END"),
					}).Error; err != nil {
						return err
					}
				} else {
					// Switching from +1 to -1
					if err := tx.Model(&comment).Updates(map[string]interface{}{
						"upvotes":   gorm.Expr("CASE WHEN upvotes > 0 THEN upvotes - 1 ELSE 0 END"),
						"downvotes": gorm.Expr("downvotes + 1"),
					}).Error; err != nil {
						return err
					}
				}
				resultVote = req.Value
				commentLog.Log("VOTE", "switched", "comment_id", commentID, "user_id", userID, "to", req.Value)
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// No existing vote — create new
			vote := models.CommentVote{
				CommentID: uint(commentID),
				UserID:    userID,
				Value:     req.Value,
			}
			if err := tx.Create(&vote).Error; err != nil {
				return err
			}
			if req.Value == 1 {
				if err := tx.Model(&comment).Update("upvotes", gorm.Expr("upvotes + 1")).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Model(&comment).Update("downvotes", gorm.Expr("downvotes + 1")).Error; err != nil {
					return err
				}
			}
			resultVote = req.Value
			commentLog.Log("VOTE", "created", "comment_id", commentID, "user_id", userID, "value", req.Value)
		} else {
			return err
		}

		// Re-fetch comment to get updated counts (Expr updates are DB-side)
		if err := tx.First(&comment, commentID).Error; err != nil {
			return err
		}

		// Recalculate Wilson score
		newScore := models.WilsonScore(comment.Upvotes, comment.Downvotes)
		if err := tx.Model(&comment).Update("score", newScore).Error; err != nil {
			return err
		}
		comment.Score = newScore
		resultComment = comment

		return nil
	})

	if err != nil {
		if err.Error() == "cannot vote on a deleted comment" {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
			return
		}
		commentLog.Log("VOTE", "transaction failed", "comment_id", commentID, "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process vote"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"comment_id": resultComment.ID,
		"upvotes":    resultComment.Upvotes,
		"downvotes":  resultComment.Downvotes,
		"score":      resultComment.Score,
		"user_vote":  resultVote,
	})
}

// ----- DELETE /comments/:comment_id/vote -----

// RemoveCommentVote explicitly removes the user's vote from a comment.
// Idempotent: returns 200 even if no vote existed.
//
// Route: DELETE /api/v1/comments/:comment_id/vote
func (app *App) RemoveCommentVote(c *gin.Context) {
	commentID, ok := helpers.MustParseCommentID(c)
	if !ok {
		return
	}
	userID := helpers.GetUserID(c)

	err := app.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		// Lock comment for atomic score update
		var comment models.Comment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&comment, commentID).Error; err != nil {
			return err
		}

		var existing models.CommentVote
		if err := tx.Where("comment_id = ? AND user_id = ?", commentID, userID).First(&existing).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil // No vote to remove — idempotent
			}
			return err
		}

		// Remove the vote and decrement the appropriate counter
		if err := tx.Delete(&existing).Error; err != nil {
			return err
		}
		if existing.Value == 1 {
			if err := tx.Model(&comment).Update("upvotes", gorm.Expr("CASE WHEN upvotes > 0 THEN upvotes - 1 ELSE 0 END")).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&comment).Update("downvotes", gorm.Expr("CASE WHEN downvotes > 0 THEN downvotes - 1 ELSE 0 END")).Error; err != nil {
				return err
			}
		}

		// Recalculate Wilson score
		if err := tx.First(&comment, commentID).Error; err != nil {
			return err
		}
		newScore := models.WilsonScore(comment.Upvotes, comment.Downvotes)
		return tx.Model(&comment).Update("score", newScore).Error
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
			return
		}
		commentLog.Log("UNVOTE", "transaction failed", "comment_id", commentID, "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove vote"})
		return
	}

	commentLog.Log("UNVOTE", "success", "comment_id", commentID, "user_id", userID)
	c.JSON(http.StatusOK, gin.H{"message": "Vote removed"})
}

// ----- GET /me/comments -----

// GetMyComments returns a flat paginated list of the authenticated user's comments.
// Soft-deleted comments are excluded. Each response includes the note title for context.
//
// Query params:
//
//	page  — page number (default 1)
//	limit — page size (default 25, max 100)
//
// Route: GET /api/v1/me/comments
func (app *App) GetMyComments(c *gin.Context) {
	userID := helpers.GetUserID(c)
	pag := helpers.ParsePagination(c)

	var total int64
	if err := app.DB.Model(&models.Comment{}).Where("user_id = ? AND is_deleted = false", userID).Count(&total).Error; err != nil {
		commentLog.Log("MY_COMMENTS", "count error", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count comments"})
		return
	}

	var comments []models.Comment
	if err := app.DB.Where("user_id = ? AND is_deleted = false", userID).
		Order("created_at DESC").
		Offset(pag.Offset).Limit(pag.Limit).
		Find(&comments).Error; err != nil {
		commentLog.Log("MY_COMMENTS", "fetch error", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
		return
	}

	// Batch-fetch note titles for context
	noteIDs := make(map[uint]struct{})
	for _, cm := range comments {
		noteIDs[cm.NoteID] = struct{}{}
	}
	noteTitles := make(map[uint]string)
	if len(noteIDs) > 0 {
		ids := make([]uint, 0, len(noteIDs))
		for id := range noteIDs {
			ids = append(ids, id)
		}
		var notes []models.Note
		app.DB.Select("id, title").Where("id IN ?", ids).Find(&notes)
		for _, n := range notes {
			noteTitles[uint(n.ID)] = n.Title
		}
	}

	type myCommentResponse struct {
		ID        uint      `json:"id"`
		NoteID    uint      `json:"note_id"`
		NoteTitle string    `json:"note_title"`
		Body      string    `json:"body"`
		Upvotes   int64     `json:"upvotes"`
		Downvotes int64     `json:"downvotes"`
		CreatedAt time.Time `json:"created_at"`
	}

	results := make([]myCommentResponse, 0, len(comments))
	for _, cm := range comments {
		results = append(results, myCommentResponse{
			ID:        cm.ID,
			NoteID:    cm.NoteID,
			NoteTitle: noteTitles[cm.NoteID],
			Body:      cm.Body,
			Upvotes:   cm.Upvotes,
			Downvotes: cm.Downvotes,
			CreatedAt: cm.CreatedAt,
		})
	}

	commentLog.Log("MY_COMMENTS", "success", "user_id", userID, "count", len(results), "total", total)
	c.JSON(http.StatusOK, gin.H{
		"comments": results,
		"total":    total,
		"page":     pag.Page,
		"limit":    pag.Limit,
	})
}

// ===== TREE-BUILDING INTERNALS =====

// buildResponseMap converts flat comments into a map of CommentResponse pointers.
func buildResponseMap(
	comments []models.Comment,
	userMap map[uint64]string,
	userVotes map[uint]int8,
) map[uint]*CommentResponse {
	m := make(map[uint]*CommentResponse, len(comments))
	for _, c := range comments {
		resp := &CommentResponse{
			ID:        c.ID,
			NoteID:    c.NoteID,
			UserID:    c.UserID,
			ParentID:  c.ParentID,
			Upvotes:   c.Upvotes,
			Downvotes: c.Downvotes,
			Score:     c.Score,
			Depth:     c.Depth,
			IsDeleted: c.IsDeleted,
			IsEdited:  c.EditedAt != nil,
			CreatedAt: c.CreatedAt,
			Children:  []*CommentResponse{},
		}

		// Redact deleted comments (Reddit shows "[deleted]")
		// Zero out UserID so the author's identity is fully hidden.
		if c.IsDeleted {
			resp.Body = "[deleted]"
			resp.Username = "[deleted]"
			resp.UserID = 0
		} else {
			resp.Body = c.Body
			if name, ok := userMap[c.UserID]; ok {
				resp.Username = name
			} else {
				resp.Username = "User " + strconv.FormatUint(c.UserID, 10)
			}
		}

		// Attach current user's vote
		if userVotes != nil {
			resp.UserVote = userVotes[c.ID]
		}

		m[c.ID] = resp
	}
	return m
}

// wireChildren links each child response to its parent's Children slice.
func wireChildren(m map[uint]*CommentResponse) {
	for _, resp := range m {
		if resp.ParentID != nil {
			if parent, ok := m[*resp.ParentID]; ok {
				parent.Children = append(parent.Children, resp)
			}
		}
	}
}

// truncateDepth removes children beyond maxDepth and sets HasMore flags.
func truncateDepth(m map[uint]*CommentResponse, maxDepth int) {
	for _, resp := range m {
		if resp.Depth >= maxDepth && len(resp.Children) > 0 {
			resp.HasMore = true
			resp.Children = []*CommentResponse{}
		}
	}
}

// sortTree recursively sorts children at each level by the specified order.
func sortTree(nodes []*CommentResponse, order models.CommentSortOrder) {
	sortSlice(nodes, order)
	for _, node := range nodes {
		if len(node.Children) > 0 {
			sortTree(node.Children, order)
		}
	}
}

// sortSlice sorts a single level of CommentResponse by the given order.
func sortSlice(nodes []*CommentResponse, order models.CommentSortOrder) {
	switch order {
	case models.SortBest, models.SortHot:
		sort.SliceStable(nodes, func(i, j int) bool {
			if nodes[i].Score == nodes[j].Score {
				return olderFirst(nodes[i], nodes[j])
			}
			return nodes[i].Score > nodes[j].Score
		})
	case models.SortNew:
		sort.SliceStable(nodes, func(i, j int) bool {
			return newerFirst(nodes[i], nodes[j])
		})
	case models.SortOld:
		sort.SliceStable(nodes, func(i, j int) bool {
			return olderFirst(nodes[i], nodes[j])
		})
	case models.SortTop:
		sort.SliceStable(nodes, func(i, j int) bool {
			netI := nodes[i].Upvotes - nodes[i].Downvotes
			netJ := nodes[j].Upvotes - nodes[j].Downvotes
			if netI == netJ {
				return olderFirst(nodes[i], nodes[j])
			}
			return netI > netJ
		})
	case models.SortControversial:
		sort.SliceStable(nodes, func(i, j int) bool {
			cI := models.ControversyScore(nodes[i].Upvotes, nodes[i].Downvotes)
			cJ := models.ControversyScore(nodes[j].Upvotes, nodes[j].Downvotes)
			if cI == cJ {
				return olderFirst(nodes[i], nodes[j])
			}
			return cI > cJ
		})
	}
}

func olderFirst(a, b *CommentResponse) bool {
	if a.CreatedAt.Equal(b.CreatedAt) {
		return a.ID < b.ID
	}
	return a.CreatedAt.Before(b.CreatedAt)
}

func newerFirst(a, b *CommentResponse) bool {
	if a.CreatedAt.Equal(b.CreatedAt) {
		return a.ID > b.ID
	}
	return a.CreatedAt.After(b.CreatedAt)
}

// ===== BATCH DATA HELPERS =====

// fetchCommentUsernames batch-fetches display names for all unique user IDs
// in the comment set. Returns a map[userID] → display name.
// FIX #4: Uses Username (public handle) instead of email to prevent identity leakage.
// FIX #5: Logs DB errors instead of silently ignoring them.
func (app *App) fetchCommentUsernames(comments []models.Comment) map[uint64]string {
	// Collect unique user IDs
	seen := make(map[uint64]struct{})
	var userIDs []uint64
	for _, c := range comments {
		if _, ok := seen[c.UserID]; !ok {
			seen[c.UserID] = struct{}{}
			userIDs = append(userIDs, c.UserID)
		}
	}

	if len(userIDs) == 0 {
		return nil
	}

	var users []models.User
	if err := app.DB.Where("id IN ?", userIDs).Select("id, username").Find(&users).Error; err != nil {
		commentLog.Log("HELPER", "failed to batch-fetch usernames", "error", err, "count", len(userIDs))
		// Return empty map — caller will use fallback "User <id>"
		return make(map[uint64]string)
	}

	m := make(map[uint64]string, len(users))
	for _, u := range users {
		m[uint64(u.ID)] = u.DisplayName()
	}
	return m
}

// fetchUserCommentVotes batch-fetches the current user's votes on all comments
// in the given set. Returns map[commentID] → vote value (+1, -1).
// FIX #5: Logs DB errors instead of silently ignoring them.
func (app *App) fetchUserCommentVotes(userID uint64, comments []models.Comment) map[uint]int8 {
	if len(comments) == 0 {
		return nil
	}

	commentIDs := make([]uint, 0, len(comments))
	for _, c := range comments {
		commentIDs = append(commentIDs, c.ID)
	}

	var votes []models.CommentVote
	if err := app.DB.Where("user_id = ? AND comment_id IN ?", userID, commentIDs).Find(&votes).Error; err != nil {
		commentLog.Log("HELPER", "failed to batch-fetch user votes", "error", err,
			"user_id", userID, "comments", len(commentIDs))
		return make(map[uint]int8)
	}

	m := make(map[uint]int8, len(votes))
	for _, v := range votes {
		m[v.CommentID] = v.Value
	}
	return m
}

// lookupUsername fetches a single user's public display name for response population.
// FIX #4: Uses Username instead of email.
func (app *App) lookupUsername(userID uint64) string {
	var user models.User
	if err := app.DB.Select("id, username").First(&user, userID).Error; err != nil {
		return "User " + strconv.FormatUint(userID, 10)
	}
	return user.DisplayName()
}

// ===== TWO-PHASE READ HELPERS =====

// hasCompletePathData checks whether all comments for a note have materialized paths.
// Path queries are only used when coverage is complete to avoid missing descendants.
func (app *App) hasCompletePathData(noteID uint64) (bool, error) {
	var missing int64
	err := app.DB.Model(&models.Comment{}).
		Where("note_id = ? AND (path = '' OR path IS NULL)", noteID).
		Count(&missing).Error
	if err != nil {
		return false, err
	}
	return missing == 0, nil
}

// fetchDescendantsForRoots fetches descendants for the selected root IDs with a hard budget.
// Returns a truncation flag if the response had to be clipped.
func (app *App) fetchDescendantsForRoots(
	noteID uint64,
	rootIDs []uint,
	maxDepth int,
	budget int,
) ([]models.Comment, bool, error) {
	if budget <= 0 || len(rootIDs) == 0 {
		return []models.Comment{}, false, nil
	}

	canUsePath, err := app.hasCompletePathData(noteID)
	if err != nil {
		return nil, false, err
	}

	if canUsePath {
		pathQuery := app.DB.Where("note_id = ? AND parent_id IS NOT NULL AND depth <= ?", noteID, maxDepth)
		pathConditions := app.DB.Where("1 = 0")
		for _, rid := range rootIDs {
			prefix := fmt.Sprintf("/%d/", rid)
			pathConditions = pathConditions.Or("path LIKE ?", prefix+"%")
		}

		var descendants []models.Comment
		if err := pathQuery.Where(pathConditions).
			Order("created_at ASC, id ASC").
			Limit(budget + 1).
			Find(&descendants).Error; err != nil {
			return nil, false, err
		}

		truncated := len(descendants) > budget
		if truncated {
			descendants = descendants[:budget]
		}
		return descendants, truncated, nil
	}

	// Legacy fallback for notes with partial/empty materialized paths.
	var allDescendants []models.Comment
	if err := app.DB.Where("note_id = ? AND parent_id IS NOT NULL AND depth <= ?", noteID, maxDepth).
		Order("created_at ASC, id ASC").
		Limit(maxCommentsPerNote + 1).
		Find(&allDescendants).Error; err != nil {
		return nil, false, err
	}

	hitHardCap := len(allDescendants) > maxCommentsPerNote
	if hitHardCap {
		allDescendants = allDescendants[:maxCommentsPerNote]
	}

	descendants := filterDescendantsOfRoots(allDescendants, rootIDs)
	truncated := hitHardCap
	if len(descendants) > budget {
		descendants = descendants[:budget]
		truncated = true
	}

	return descendants, truncated, nil
}

// fetchCommentSubtree fetches a target comment and its descendants within the depth window.
// Returns whether the result was truncated and whether path-based querying was used.
func (app *App) fetchCommentSubtree(
	target models.Comment,
	relativeMaxDepth int,
	budget int,
) ([]models.Comment, bool, bool, error) {
	canUsePath, err := app.hasCompletePathData(uint64(target.NoteID))
	if err != nil {
		return nil, false, false, err
	}

	if canUsePath && target.Path != "" {
		var subtree []models.Comment
		if err := app.DB.Where(
			"note_id = ? AND (id = ? OR (path LIKE ? AND depth <= ?))",
			target.NoteID, target.ID, target.Path+"%", relativeMaxDepth,
		).Order("created_at ASC, id ASC").Limit(budget + 1).Find(&subtree).Error; err != nil {
			return nil, false, false, err
		}

		truncated := len(subtree) > budget
		if truncated {
			subtree = subtree[:budget]
		}
		return subtree, truncated, true, nil
	}

	var depthComments []models.Comment
	if err := app.DB.Where("note_id = ? AND depth >= ? AND depth <= ?",
		target.NoteID, target.Depth, relativeMaxDepth).
		Order("created_at ASC, id ASC").
		Limit(maxCommentsPerNote + 1).
		Find(&depthComments).Error; err != nil {
		return nil, false, false, err
	}

	hitHardCap := len(depthComments) > maxCommentsPerNote
	if hitHardCap {
		depthComments = depthComments[:maxCommentsPerNote]
	}

	subtree := filterExactSubtree(depthComments, target.ID)
	truncated := hitHardCap
	if len(subtree) > budget {
		subtree = subtree[:budget]
		truncated = true
	}

	return subtree, truncated, false, nil
}

// resolveCommentDeletePrivileges determines global and scoped admin privileges for comment deletion.
func (app *App) resolveCommentDeletePrivileges(userID uint64, noteID uint) (bool, bool, error) {
	var user struct {
		IsGlobalAdmin bool
	}
	if err := app.DB.Model(&models.User{}).
		Select("is_global_admin").
		Where("id = ?", userID).
		Take(&user).Error; err != nil {
		return false, false, err
	}
	if user.IsGlobalAdmin {
		return true, false, nil
	}

	var note struct {
		SubnoteryID uint
	}
	if err := app.DB.Model(&models.Note{}).
		Select("subnotery_id").
		Where("id = ?", noteID).
		Take(&note).Error; err != nil {
		return false, false, err
	}

	var adminCount int64
	if err := app.DB.Table("user_admins").
		Where("user_id = ? AND subnotery_id = ?", userID, note.SubnoteryID).
		Count(&adminCount).Error; err != nil {
		return false, false, err
	}

	return false, adminCount > 0, nil
}

// filterDescendantsOfRoots filters a flat list of comments to only those that are
// descendants of the given root IDs. It walks parent chains in memory.
// Used as a fallback when materialized paths are not available.
func filterDescendantsOfRoots(comments []models.Comment, rootIDs []uint) []models.Comment {
	rootSet := make(map[uint]struct{}, len(rootIDs))
	for _, id := range rootIDs {
		rootSet[id] = struct{}{}
	}

	// Build parent lookup
	parentOf := make(map[uint]*uint, len(comments))
	for i := range comments {
		parentOf[comments[i].ID] = comments[i].ParentID
	}

	// Cache: tracks whether a comment is a descendant of one of the roots.
	cache := make(map[uint]bool, len(comments))

	var isDescendant func(id uint) bool
	isDescendant = func(id uint) bool {
		if v, ok := cache[id]; ok {
			return v
		}
		if _, isRoot := rootSet[id]; isRoot {
			cache[id] = true
			return true
		}
		pid, exists := parentOf[id]
		if !exists || pid == nil {
			cache[id] = false
			return false
		}
		result := isDescendant(*pid)
		cache[id] = result
		return result
	}

	var result []models.Comment
	for _, c := range comments {
		if isDescendant(c.ID) {
			result = append(result, c)
		}
	}
	return result
}

// filterExactSubtree filters a flat list of comments to only the target comment
// and its exact descendants. Used as a legacy fallback in GetComment when
// materialized paths are not available.
func filterExactSubtree(comments []models.Comment, targetID uint) []models.Comment {
	return filterDescendantsOfRoots(comments, []uint{targetID})
}

// isNoteAdmin checks whether the given user is a global admin or a scoped admin
// for the subnotery owning the note. Used for admin-only operations on pending notes.
func (app *App) isNoteAdmin(userID uint64, subnoteryID uint) bool {
	var user models.User
	if err := app.DB.Select("id, is_global_admin").First(&user, userID).Error; err != nil {
		return false
	}
	if user.IsGlobalAdmin {
		return true
	}
	var count int64
	app.DB.Table("user_admins").
		Where("user_id = ? AND subnotery_id = ?", userID, subnoteryID).
		Count(&count)
	return count > 0
}
