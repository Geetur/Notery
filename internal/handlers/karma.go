// Package handlers/karma.go contains HTTP handlers for the karma/reputation system.
//
// ARCHITECTURE:
//
//	Karma is a display-only reputation metric for users, calculated from:
//	  - Note karma: sum of (upvotes - downvotes) across all the user's approved notes
//	  - Comment karma: sum of (upvotes - downvotes) across all the user's comments
//
//	Karma is recalculated periodically and stored on the user record for fast access.
//	The recalculation endpoint is called internally (e.g., after votes) or by an admin.
//
//	The stored values on User.NoteKarma and User.CommentKarma are the cached karma values.
//	RecalculateKarma recomputes from the authoritative sources (notes and comments tables).
//
// DESIGN DECISIONS:
//
//   - Karma is eventually consistent, not real-time. This avoids locking on every vote.
//   - Only approved notes contribute to note karma (prevents gaming via self-approval).
//   - Deleted comments still contribute to comment karma (matches Reddit behavior).
//   - Negative karma is possible (user's content is consistently downvoted).
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// karmaLog is the domain-specific logger for karma operations.
var karmaLog = helpers.NewLogger("KARMA")

// GetMyKarma returns the authenticated user's karma breakdown.
//
// Route: GET /api/v1/me/karma
func (app *App) GetMyKarma(c *gin.Context) {
	userID := helpers.GetUserID(c)
	karmaLog.Log("GET", "Processing karma request", "userID", userID)

	var user models.User
	if err := app.DB.Select("id", "note_karma", "comment_karma").First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"note_karma":    user.NoteKarma,
		"comment_karma": user.CommentKarma,
		"total_karma":   user.TotalKarma(),
	})
}

// RecalculateMyKarma recalculates the authenticated user's karma from the DB.
// This is intended to be called when the user views their karma page, ensuring
// reasonably fresh values without a background job.
//
// Route: POST /api/v1/me/karma/refresh
func (app *App) RecalculateMyKarma(c *gin.Context) {
	userID := helpers.GetUserID(c)
	karmaLog.Log("RECALCULATE", "Processing karma recalculation", "userID", userID)

	noteKarma, commentKarma, err := app.recalculateUserKarma(userID)
	if err != nil {
		karmaLog.Log("RECALCULATE", "Failed to recalculate karma", "error", err, "userID", userID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to recalculate karma"})
		return
	}

	karmaLog.Log("RECALCULATE", "Karma recalculated", "userID", userID, "noteKarma", noteKarma, "commentKarma", commentKarma)
	c.JSON(http.StatusOK, gin.H{
		"note_karma":    noteKarma,
		"comment_karma": commentKarma,
		"total_karma":   noteKarma + commentKarma,
	})
}

// recalculateUserKarma computes karma from notes and comments tables and updates the user record.
// Returns (noteKarma, commentKarma, error).
func (app *App) recalculateUserKarma(userID uint64) (int64, int64, error) {
	// Note karma: sum of (upvotes - downvotes) for user's approved notes
	var noteKarma int64
	err := app.DB.Model(&models.Note{}).
		Where("creator_id = ? AND status = ?", userID, models.StatusApproved).
		Select("COALESCE(SUM(upvotes) - SUM(downvotes), 0)").
		Scan(&noteKarma).Error
	if err != nil {
		return 0, 0, err
	}

	// Comment karma: sum of (upvotes - downvotes) for user's comments
	var commentKarma int64
	err = app.DB.Model(&models.Comment{}).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(upvotes) - SUM(downvotes), 0)").
		Scan(&commentKarma).Error
	if err != nil {
		return 0, 0, err
	}

	// Update user record with cached karma values
	if err := app.DB.Model(&models.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"note_karma":    noteKarma,
			"comment_karma": commentKarma,
		}).Error; err != nil {
		return 0, 0, err
	}

	return noteKarma, commentKarma, nil
}
