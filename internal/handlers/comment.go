// Package handlers/comment.go contains HTTP handlers for the Reddit-style comment system.
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
//
// DESIGN:
//
//	Comments form a tree rooted at notes. The tree is fetched in one query per note
//	and assembled in memory (O(n) time+space). Top-level roots are paginated; all
//	descendants of the current page's roots are included up to MaxTreeDepth.
//
//	Ranking uses Wilson score lower bound with NO time decay — quality always wins.
//	Vote counts and Wilson scores are updated atomically inside a DB transaction
//	with SELECT … FOR UPDATE to prevent race conditions on concurrent votes.
//
// TREE ALGORITHM:
//
//  1. Fetch all comments for the note (hard cap: maxCommentsPerNote).
//  2. Batch-fetch the current user's votes and all commenters' usernames.
//  3. Build a map[commentID] → *CommentResponse.
//  4. Wire parent→children pointers.
//  5. Sort each depth level by the requested sort order.
//  6. Truncate beyond MaxTreeDepth, setting has_more_replies.
//  7. Paginate top-level roots and return.
package handlers

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
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
// Top-level comments are paginated; all descendants (up to MaxTreeDepth) are included.
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
	if note.Status != models.StatusApproved {
		c.JSON(http.StatusForbidden, gin.H{"error": "Comments are not available for this note"})
		return
	}

	// Count total top-level comments (for pagination metadata)
	var totalTopLevel int64
	app.DB.Model(&models.Comment{}).
		Where("note_id = ? AND parent_id IS NULL", noteID).
		Count(&totalTopLevel)

	// Count total comments to detect truncation
	var totalComments int64
	app.DB.Model(&models.Comment{}).
		Where("note_id = ?", noteID).
		Count(&totalComments)

	// Fetch all comments for this note (hard cap for safety)
	var comments []models.Comment
	if err := app.DB.Where("note_id = ?", noteID).
		Order("created_at ASC").
		Limit(maxCommentsPerNote).
		Find(&comments).Error; err != nil {
		commentLog.Log("LIST", "db error", "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
		return
	}

	// Batch-fetch usernames for all commenters
	userMap := app.fetchCommentUsernames(comments)

	// Batch-fetch current user's votes (if authenticated)
	var userVotes map[uint]int8
	if userID, authenticated := helpers.TryGetUserID(c); authenticated {
		userVotes = app.fetchUserCommentVotes(userID, comments)
	}

	// Build tree, sort, and truncate at max depth
	roots := buildCommentTree(comments, userMap, userVotes, sortOrder, maxDepth)

	// Paginate top-level
	start := pag.Offset
	end := start + pag.Limit
	if start > len(roots) {
		start = len(roots)
	}
	if end > len(roots) {
		end = len(roots)
	}

	// Detect truncation: total comments in DB exceed the hard cap we fetched
	truncated := totalComments > int64(maxCommentsPerNote)

	commentLog.Log("LIST", "served", "note_id", noteID, "total_top", totalTopLevel,
		"page", pag.Page, "sort", string(sortOrder), "tree_nodes", len(comments), "truncated", truncated)

	c.JSON(http.StatusOK, gin.H{
		"comments":  roots[start:end],
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

	// Validate body length
	if utf8.RuneCountInString(req.Body) > models.MaxCommentBodyLength {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Comment body exceeds maximum length",
			"max":   models.MaxCommentBodyLength,
		})
		return
	}

	// Verify note exists and is approved
	var note models.Note
	if err := app.DB.First(&note, noteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	if note.Status != models.StatusApproved {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot comment on this note"})
		return
	}

	// Determine depth from parent (if replying)
	depth := 0
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

	if err := app.DB.Create(&comment).Error; err != nil {
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

// GetComment returns a single comment with its reply subtree.
// Used for "Continue this thread →" deep-linking.
//
// Query params: max_depth (default 10, max 20), sort (default best)
//
// Route: GET /api/v1/comments/:comment_id
func (app *App) GetComment(c *gin.Context) {
	commentID, ok := parseCommentID(c)
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

	// FIX #1: Enforce note visibility — must be approved (same as GetNoteComments).
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

	// FIX #8: Only fetch the subtree rooted at target instead of all note comments.
	// We fetch the target comment + all descendants (depth > target.depth whose
	// ancestor chain leads here). Since we don't have a path column, we still
	// need to fetch note-level comments but limit to the depth window we care about.
	relativeMaxDepth := target.Depth + maxDepth
	var subtreeComments []models.Comment
	if err := app.DB.Where("note_id = ? AND depth >= ? AND depth <= ?",
		target.NoteID, target.Depth, relativeMaxDepth).
		Order("created_at ASC").
		Limit(maxCommentsPerNote).
		Find(&subtreeComments).Error; err != nil {
		commentLog.Log("GET", "db error", "comment_id", commentID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
		return
	}

	userMap := app.fetchCommentUsernames(subtreeComments)

	var userVotes map[uint]int8
	if userID, authenticated := helpers.TryGetUserID(c); authenticated {
		userVotes = app.fetchUserCommentVotes(userID, subtreeComments)
	}

	// Build tree from the depth-filtered set, then extract subtree rooted at target
	responseMap := buildResponseMap(subtreeComments, userMap, userVotes)
	wireChildren(responseMap)
	truncateDepth(responseMap, relativeMaxDepth)

	resp, exists := responseMap[uint(commentID)]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
		return
	}

	sortTree([]*CommentResponse{resp}, sortOrder)

	commentLog.Log("GET", "served", "comment_id", commentID, "note_id", target.NoteID)
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
	commentID, ok := parseCommentID(c)
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
	commentID, ok := parseCommentID(c)
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

	// Ownership or admin check (with defensive type assertion)
	isOwner := comment.UserID == userID
	isGlobalAdmin := false
	isSubnoteryAdmin := false
	if adminType, exists := c.Get("admin_type"); exists {
		if v, ok := adminType.(bool); ok {
			isGlobalAdmin = v
		}
	}

	// Subnotery admins may only delete comments on notes within their subnoteries.
	if !isGlobalAdmin && !isOwner {
		// Look up note to check subnotery scope
		var note models.Note
		if err := app.DB.Select("id, subnotery_id").First(&note, comment.NoteID).Error; err == nil {
			var adminCount int64
			app.DB.Table("user_admins").
				Where("user_id = ? AND subnotery_id = ?", userID, note.SubnoteryID).
				Count(&adminCount)
			if adminCount > 0 {
				isSubnoteryAdmin = true
			}
		}
	}

	if !isOwner && !isGlobalAdmin && !isSubnoteryAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "You can only delete your own comments"})
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
	commentID, ok := parseCommentID(c)
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

	err := app.DB.Transaction(func(tx *gorm.DB) error {
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
					if err := tx.Model(&comment).Update("upvotes", gorm.Expr("GREATEST(upvotes - 1, 0)")).Error; err != nil {
						return err
					}
				} else {
					if err := tx.Model(&comment).Update("downvotes", gorm.Expr("GREATEST(downvotes - 1, 0)")).Error; err != nil {
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
						"downvotes": gorm.Expr("GREATEST(downvotes - 1, 0)"),
					}).Error; err != nil {
						return err
					}
				} else {
					// Switching from +1 to -1
					if err := tx.Model(&comment).Updates(map[string]interface{}{
						"upvotes":   gorm.Expr("GREATEST(upvotes - 1, 0)"),
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
	commentID, ok := parseCommentID(c)
	if !ok {
		return
	}
	userID := helpers.GetUserID(c)

	err := app.DB.Transaction(func(tx *gorm.DB) error {
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
			if err := tx.Model(&comment).Update("upvotes", gorm.Expr("GREATEST(upvotes - 1, 0)")).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&comment).Update("downvotes", gorm.Expr("GREATEST(downvotes - 1, 0)")).Error; err != nil {
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

// ===== TREE-BUILDING INTERNALS =====

// buildCommentTree transforms a flat slice of comments into a sorted, depth-limited tree.
func buildCommentTree(
	comments []models.Comment,
	userMap map[uint64]string,
	userVotes map[uint]int8,
	sortOrder models.CommentSortOrder,
	maxDepth int,
) []*CommentResponse {
	responseMap := buildResponseMap(comments, userMap, userVotes)
	wireChildren(responseMap)

	// Collect roots (top-level comments)
	var roots []*CommentResponse
	for _, c := range comments {
		if c.ParentID == nil {
			if resp, ok := responseMap[c.ID]; ok {
				roots = append(roots, resp)
			}
		}
	}

	// Sort entire tree recursively
	sortTree(roots, sortOrder)

	// Truncate beyond max depth
	truncateDepth(responseMap, maxDepth)

	return roots
}

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
		if c.IsDeleted {
			resp.Body = "[deleted]"
			resp.Username = "[deleted]"
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
	case models.SortBest:
		sort.SliceStable(nodes, func(i, j int) bool {
			return nodes[i].Score > nodes[j].Score
		})
	case models.SortNew:
		sort.SliceStable(nodes, func(i, j int) bool {
			return nodes[i].CreatedAt.After(nodes[j].CreatedAt)
		})
	case models.SortOld:
		sort.SliceStable(nodes, func(i, j int) bool {
			return nodes[i].CreatedAt.Before(nodes[j].CreatedAt)
		})
	case models.SortTop:
		sort.SliceStable(nodes, func(i, j int) bool {
			netI := nodes[i].Upvotes - nodes[i].Downvotes
			netJ := nodes[j].Upvotes - nodes[j].Downvotes
			return netI > netJ
		})
	case models.SortControversial:
		sort.SliceStable(nodes, func(i, j int) bool {
			cI := models.ControversyScore(nodes[i].Upvotes, nodes[i].Downvotes)
			cJ := models.ControversyScore(nodes[j].Upvotes, nodes[j].Downvotes)
			return cI > cJ
		})
	}
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

// ===== URL PARAMETER HELPERS =====

// parseCommentID extracts and validates the "comment_id" URL parameter.
func parseCommentID(c *gin.Context) (uint64, bool) {
	id, ok := helpers.ParseUintParam(c, "comment_id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid comment ID"})
		return 0, false
	}
	return id, true
}
