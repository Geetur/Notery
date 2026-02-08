// Package handlers/feed.go contains the HTTP handlers for feed operations
package handlers

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// feedLog is the shared logger for feed operations
var feedLog = helpers.FeedLog

const (
	// Redis key prefixes
	globalHotKey    = "feed:hot:global"
	subnoteryHotKey = "feed:hot:subnotery:"

	// copied from Reddit's algorithm
	// Reddit's magic number for time decay (seconds in 12 hours)
	hotDecaySeconds = 45000.0

	// Epoch for hotness calculation (Jan 1, 2025)
	hotEpoch = 1735689600
)

// CalculateHotness computes the Reddit-style hot score for a note.
// Formula: sign(score) * log10(max(|score|, 1)) + (created_at - epoch) / decay
func CalculateHotness(upvotes, downvotes uint64, createdAt time.Time) float64 {
	score := int64(upvotes) - int64(downvotes)
	order := math.Log10(math.Max(math.Abs(float64(score)), 1))
	sign := 0.0
	if score > 0 {
		sign = 1.0
	} else if score < 0 {
		sign = -1.0
	}
	seconds := float64(createdAt.Unix() - hotEpoch)
	return sign*order + seconds/hotDecaySeconds
}

// UpdateNoteHotness recalculates and stores a note's hotness score in Redis.
// Call this after any vote change.
func (app *App) UpdateNoteHotness(ctx context.Context, note *models.Note) error {
	hotness := CalculateHotness(note.Upvotes, note.Downvotes, note.CreatedAt)
	feedLog.Log("HOTNESS", "calculated", "note_id", note.ID, "score", fmt.Sprintf("%.4f", hotness), "upvotes", note.Upvotes, "downvotes", note.Downvotes)

	// Update in database
	if err := app.DB.Model(note).Update("hotness", hotness).Error; err != nil {
		feedLog.Log("HOTNESS", "db update failed", "note_id", note.ID, "error", err)
		return err
	}

	// Only approved notes go in the feed
	if note.Status != models.StatusApproved {
		feedLog.Log("HOTNESS", "skipped feed (not approved)", "note_id", note.ID, "status", note.Status)
		return nil
	}

	noteID := strconv.FormatUint(uint64(note.ID), 10)

	// Update global hot feed
	if err := app.RDB.ZAdd(ctx, globalHotKey, redis.Z{
		Score:  hotness,
		Member: noteID,
	}).Err(); err != nil {
		feedLog.Log("HOTNESS", "redis global update failed", "note_id", note.ID, "error", err)
		return err
	}

	// Update subnotery hot feed
	subnoteryKey := subnoteryHotKey + strconv.FormatUint(uint64(note.SubnoteryID), 10)
	if err := app.RDB.ZAdd(ctx, subnoteryKey, redis.Z{
		Score:  hotness,
		Member: noteID,
	}).Err(); err != nil {
		feedLog.Log("HOTNESS", "redis subnotery update failed", "note_id", note.ID, "subnotery_id", note.SubnoteryID, "error", err)
		return err
	}

	feedLog.Log("HOTNESS", "updated successfully", "note_id", note.ID, "subnotery_id", note.SubnoteryID)
	return nil
}

// AddNoteToFeed adds an approved note to the hot feeds.
// Call this when a note is approved.
func (app *App) AddNoteToFeed(ctx context.Context, note *models.Note) error {
	if note.Status != models.StatusApproved {
		return nil
	}
	return app.UpdateNoteHotness(ctx, note)
}

// RemoveNoteFromFeed removes a note from all hot feeds.
// Call this when a note is rejected or deleted.
func (app *App) RemoveNoteFromFeed(ctx context.Context, note *models.Note) error {
	feedLog.Log("REMOVE", "removing from feeds", "note_id", note.ID, "subnotery_id", note.SubnoteryID)
	noteID := strconv.FormatUint(uint64(note.ID), 10)

	// Remove from global feed
	if err := app.RDB.ZRem(ctx, globalHotKey, noteID).Err(); err != nil {
		feedLog.Log("REMOVE", "global feed removal failed", "note_id", note.ID, "error", err)
		return err
	}

	// Remove from subnotery feed
	subnoteryKey := subnoteryHotKey + strconv.FormatUint(uint64(note.SubnoteryID), 10)
	if err := app.RDB.ZRem(ctx, subnoteryKey, noteID).Err(); err != nil {
		feedLog.Log("REMOVE", "subnotery feed removal failed", "note_id", note.ID, "error", err)
		return err
	}

	feedLog.Log("REMOVE", "removed successfully", "note_id", note.ID)
	return nil
}

// GetHotFeed returns the hot feed for a user (personalized) or globally (public).
// For logged-in users: subscribed subnoteries first, then global.
// For anonymous users: just global hot notes.
func (app *App) GetHotFeed(c *gin.Context) {
	start := time.Now()
	ctx := c.Request.Context()

	// Parse pagination using helpers
	pag := helpers.ParsePagination(c)

	// Check if user is authenticated (optional auth)
	userID, authenticated := helpers.TryGetUserID(c)

	var noteIDs []string

	if authenticated {
		feedLog.Log("GET_FEED", "fetching personalized", "user_id", userID, "page", pag.Page, "limit", pag.Limit)
		noteIDs = app.getPersonalizedFeed(ctx, userID, pag.Offset, pag.Limit)
	} else {
		feedLog.Log("GET_FEED", "fetching global (anonymous)", "page", pag.Page, "limit", pag.Limit)
		noteIDs = app.getGlobalFeed(ctx, pag.Offset, pag.Limit)
	}

	// Fetch full note data from database
	notes := app.fetchNotes(noteIDs)

	duration := time.Since(start)
	feedLog.Log("GET_FEED", "served", "count", len(notes), "page", pag.Page, "duration_ms", duration.Milliseconds())

	c.JSON(http.StatusOK, gin.H{
		"notes": notes,
		"page":  pag.Page,
		"limit": pag.Limit,
	})
}

// getPersonalizedFeed returns note IDs from user's subscribed subnoteries merged with global.
func (app *App) getPersonalizedFeed(ctx context.Context, userID uint64, offset, limit int) []string {
	// Get user's subscribed subnoteries
	var subnoteryIDs []uint
	app.DB.Table("user_memberships").
		Where("user_id = ?", userID).
		Pluck("subnotery_id", &subnoteryIDs)

	if len(subnoteryIDs) == 0 {
		feedLog.Log("PERSONALIZED", "no subscriptions, falling back to global", "user_id", userID)
		return app.getGlobalFeed(ctx, offset, limit)
	}

	feedLog.Log("PERSONALIZED", "building union feed", "user_id", userID, "subscriptions", len(subnoteryIDs))

	// Build list of keys to union (subscribed subnoteries + global)
	keys := make([]string, 0, len(subnoteryIDs)+1)
	for _, subID := range subnoteryIDs {
		keys = append(keys, subnoteryHotKey+strconv.FormatUint(uint64(subID), 10))
	}
	keys = append(keys, globalHotKey)

	// Create a temporary union key for this request
	unionKey := fmt.Sprintf("feed:union:%d:%d", userID, time.Now().UnixNano())

	// Union all feeds with weights (subscribed get slight boost)
	weights := make([]float64, len(keys))
	for i := range weights {
		if i < len(keys)-1 {
			weights[i] = 1.1 // Slight boost for subscribed subnoteries
		} else {
			weights[i] = 1.0 // Global feed
		}
	}

	// Perform weighted union
	app.RDB.ZUnionStore(ctx, unionKey, &redis.ZStore{
		Keys:      keys,
		Weights:   weights,
		Aggregate: "MAX",
	})

	// Set expiry on union key (cleanup)
	app.RDB.Expire(ctx, unionKey, 10*time.Second)

	// Get the hot notes from the union
	noteIDs, err := app.RDB.ZRevRange(ctx, unionKey, int64(offset), int64(offset+limit-1)).Result()
	if err != nil {
		feedLog.Log("PERSONALIZED", "redis fetch failed", "user_id", userID, "error", err)
		return []string{}
	}

	feedLog.Log("PERSONALIZED", "fetched", "user_id", userID, "count", len(noteIDs))
	return noteIDs
}

// getGlobalFeed returns note IDs from the global hot feed.
func (app *App) getGlobalFeed(ctx context.Context, offset, limit int) []string {
	noteIDs, err := app.RDB.ZRevRange(ctx, globalHotKey, int64(offset), int64(offset+limit-1)).Result()
	if err != nil {
		feedLog.Log("GLOBAL", "redis fetch failed", "error", err)
		return []string{}
	}
	feedLog.Log("GLOBAL", "fetched", "count", len(noteIDs))
	return noteIDs
}

// fetchNotes retrieves full note data from the database, preserving order.
func (app *App) fetchNotes(noteIDs []string) []models.Note {
	if len(noteIDs) == 0 {
		return []models.Note{}
	}

	// Convert string IDs to uints
	ids := make([]uint, 0, len(noteIDs))
	for _, idStr := range noteIDs {
		id, _ := strconv.ParseUint(idStr, 10, 64)
		ids = append(ids, uint(id))
	}

	// Fetch notes
	var notes []models.Note
	app.DB.Where("id IN ?", ids).Find(&notes)

	// Create a map for ordering
	noteMap := make(map[uint]models.Note)
	for _, note := range notes {
		noteMap[note.ID] = note
	}

	// Preserve Redis ordering
	ordered := make([]models.Note, 0, len(noteIDs))
	for _, id := range ids {
		if note, ok := noteMap[id]; ok {
			ordered = append(ordered, note)
		}
	}

	return ordered
}

// Upvote handles upvoting a note and updates hotness.
// Uses a DB transaction as the source of truth; Redis vote cache is updated afterwards.
func (app *App) Upvote(c *gin.Context) {
	ctx := c.Request.Context()
	noteID := c.Param("id")
	userID := helpers.GetUserID(c)

	feedLog.Log("UPVOTE", "processing", "user_id", userID, "note_id", noteID)

	var note models.Note
	if err := app.DB.First(&note, noteID).Error; err != nil {
		feedLog.Log("UPVOTE", "note not found", "note_id", noteID, "error", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
		return
	}

	noteIDUint := uint64(note.ID)

	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		// Lock-read existing vote
		var existing models.Vote
		err := tx.Where("user_id = ? AND note_id = ?", userID, noteIDUint).First(&existing).Error

		if err == nil {
			// Vote exists
			if existing.Direction == models.VoteUp {
				// Toggle off: remove upvote
				if err := tx.Delete(&existing).Error; err != nil {
					return err
				}
				if err := tx.Model(&note).Update("upvotes", gorm.Expr("upvotes - 1")).Error; err != nil {
					return err
				}
				note.Upvotes--
				feedLog.Log("UPVOTE", "toggled off", "user_id", userID, "note_id", noteID)
			} else {
				// Switch from down → up
				if err := tx.Model(&existing).Update("direction", models.VoteUp).Error; err != nil {
					return err
				}
				if err := tx.Model(&note).Update("downvotes", gorm.Expr("downvotes - 1")).Error; err != nil {
					return err
				}
				if err := tx.Model(&note).Update("upvotes", gorm.Expr("upvotes + 1")).Error; err != nil {
					return err
				}
				note.Downvotes--
				note.Upvotes++
				feedLog.Log("UPVOTE", "switched from down", "user_id", userID, "note_id", noteID)
			}
		} else {
			// No existing vote – create upvote
			vote := models.Vote{UserID: userID, NoteID: noteIDUint, Direction: models.VoteUp}
			if err := tx.Create(&vote).Error; err != nil {
				return err
			}
			if err := tx.Model(&note).Update("upvotes", gorm.Expr("upvotes + 1")).Error; err != nil {
				return err
			}
			note.Upvotes++
			feedLog.Log("UPVOTE", "added", "user_id", userID, "note_id", noteID)
		}
		return nil
	}); err != nil {
		feedLog.Log("UPVOTE", "transaction failed", "user_id", userID, "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process vote"})
		return
	}

	// Update Redis vote cache (best-effort)
	voteKey := fmt.Sprintf("votes:%s", noteID)
	userVoteKey := fmt.Sprintf("%d", userID)
	// Re-read the current vote state from DB to set cache correctly
	var currentVote models.Vote
	if err := app.DB.Where("user_id = ? AND note_id = ?", userID, noteIDUint).First(&currentVote).Error; err != nil {
		// Vote was removed (toggle-off)
		app.RDB.HDel(ctx, voteKey, userVoteKey)
	} else {
		app.RDB.HSet(ctx, voteKey, userVoteKey, string(currentVote.Direction))
	}

	// Recalculate hotness
	if err := app.UpdateNoteHotness(ctx, &note); err != nil {
		feedLog.Log("UPVOTE", "hotness update failed", "note_id", noteID, "error", err)
	}

	feedLog.Log("UPVOTE", "completed", "user_id", userID, "note_id", noteID, "upvotes", note.Upvotes, "downvotes", note.Downvotes)
	c.JSON(http.StatusOK, gin.H{
		"upvotes":   note.Upvotes,
		"downvotes": note.Downvotes,
		"hotness":   note.Hotness,
	})
}

// Downvote handles downvoting a note and updates hotness.
// Uses a DB transaction as the source of truth; Redis vote cache is updated afterwards.
func (app *App) Downvote(c *gin.Context) {
	ctx := c.Request.Context()
	noteID := c.Param("id")
	userID := helpers.GetUserID(c)

	feedLog.Log("DOWNVOTE", "processing", "user_id", userID, "note_id", noteID)

	var note models.Note
	if err := app.DB.First(&note, noteID).Error; err != nil {
		feedLog.Log("DOWNVOTE", "note not found", "note_id", noteID, "error", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
		return
	}

	noteIDUint := uint64(note.ID)

	if err := app.DB.Transaction(func(tx *gorm.DB) error {
		var existing models.Vote
		err := tx.Where("user_id = ? AND note_id = ?", userID, noteIDUint).First(&existing).Error

		if err == nil {
			if existing.Direction == models.VoteDown {
				// Toggle off: remove downvote
				if err := tx.Delete(&existing).Error; err != nil {
					return err
				}
				if err := tx.Model(&note).Update("downvotes", gorm.Expr("downvotes - 1")).Error; err != nil {
					return err
				}
				note.Downvotes--
				feedLog.Log("DOWNVOTE", "toggled off", "user_id", userID, "note_id", noteID)
			} else {
				// Switch from up → down
				if err := tx.Model(&existing).Update("direction", models.VoteDown).Error; err != nil {
					return err
				}
				if err := tx.Model(&note).Update("upvotes", gorm.Expr("upvotes - 1")).Error; err != nil {
					return err
				}
				if err := tx.Model(&note).Update("downvotes", gorm.Expr("downvotes + 1")).Error; err != nil {
					return err
				}
				note.Upvotes--
				note.Downvotes++
				feedLog.Log("DOWNVOTE", "switched from up", "user_id", userID, "note_id", noteID)
			}
		} else {
			// No existing vote – create downvote
			vote := models.Vote{UserID: userID, NoteID: noteIDUint, Direction: models.VoteDown}
			if err := tx.Create(&vote).Error; err != nil {
				return err
			}
			if err := tx.Model(&note).Update("downvotes", gorm.Expr("downvotes + 1")).Error; err != nil {
				return err
			}
			note.Downvotes++
			feedLog.Log("DOWNVOTE", "added", "user_id", userID, "note_id", noteID)
		}
		return nil
	}); err != nil {
		feedLog.Log("DOWNVOTE", "transaction failed", "user_id", userID, "note_id", noteID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process vote"})
		return
	}

	// Update Redis vote cache (best-effort)
	voteKey := fmt.Sprintf("votes:%s", noteID)
	userVoteKey := fmt.Sprintf("%d", userID)
	var currentVote models.Vote
	if err := app.DB.Where("user_id = ? AND note_id = ?", userID, noteIDUint).First(&currentVote).Error; err != nil {
		app.RDB.HDel(ctx, voteKey, userVoteKey)
	} else {
		app.RDB.HSet(ctx, voteKey, userVoteKey, string(currentVote.Direction))
	}

	// Recalculate hotness
	if err := app.UpdateNoteHotness(ctx, &note); err != nil {
		feedLog.Log("DOWNVOTE", "hotness update failed", "note_id", noteID, "error", err)
	}

	feedLog.Log("DOWNVOTE", "completed", "user_id", userID, "note_id", noteID, "upvotes", note.Upvotes, "downvotes", note.Downvotes)
	c.JSON(http.StatusOK, gin.H{
		"upvotes":   note.Upvotes,
		"downvotes": note.Downvotes,
		"hotness":   note.Hotness,
	})
}
