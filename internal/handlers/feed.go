// Package handlers/feed.go contains the HTTP handlers for feed operations
package handlers

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/Geetur/Notery/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

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

// FeedHandler handles feed HTTP requests backed by Redis ZSETs.
type FeedHandler struct {
	RDB *redis.Client
	DB  *gorm.DB
}

// CreateFeedHandler initializes a new FeedHandler with the given Redis client and database
// CreateFeedHandler interacts with no other handler methods.
// CreateFeedHandler interacts with Redis and the database.
func CreateFeedHandler(rdb *redis.Client, db *gorm.DB) *FeedHandler {
	return &FeedHandler{RDB: rdb, DB: db}
}

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
// UpdateNoteHotness interacts with Redis and the database to update hotness scores.
// UpdateNoteHotness interacts with no other handler methods.
func (handler *FeedHandler) UpdateNoteHotness(ctx context.Context, note *models.Note) error {
	hotness := CalculateHotness(note.Upvotes, note.Downvotes, note.CreatedAt)

	// Update in database
	if err := handler.DB.Model(note).Update("hotness", hotness).Error; err != nil {
		return err
	}

	// Only approved notes go in the feed
	if note.Status != "Approved" {
		return nil
	}

	noteID := strconv.FormatUint(uint64(note.ID), 10)

	// Update global hot feed
	if err := handler.RDB.ZAdd(ctx, globalHotKey, redis.Z{
		Score:  hotness,
		Member: noteID,
	}).Err(); err != nil {
		return err
	}

	// Update subnotery hot feed
	subnoteryKey := subnoteryHotKey + strconv.FormatUint(uint64(note.SubnoteryID), 10)
	if err := handler.RDB.ZAdd(ctx, subnoteryKey, redis.Z{
		Score:  hotness,
		Member: noteID,
	}).Err(); err != nil {
		return err
	}

	return nil
}

// AddNoteToFeed adds an approved note to the hot feeds.
// Call this when a note is approved.
// AddNoteToFeed interacts with Redis and the database to add the note to feeds.
// AddNoteToFeed interacts with no other handler methods.
func (handler *FeedHandler) AddNoteToFeed(ctx context.Context, note *models.Note) error {
	if note.Status != "Approved" {
		return nil
	}
	return handler.UpdateNoteHotness(ctx, note)
}

// RemoveNoteFromFeed removes a note from all hot feeds.
// Call this when a note is rejected or deleted.
// RemoveNoteFromFeed interacts with Redis to remove the note from feeds.
// RemoveNoteFromFeed interacts with no other handler methods.
func (handler *FeedHandler) RemoveNoteFromFeed(ctx context.Context, note *models.Note) error {
	noteID := strconv.FormatUint(uint64(note.ID), 10)

	// Remove from global feed
	if err := handler.RDB.ZRem(ctx, globalHotKey, noteID).Err(); err != nil {
		return err
	}

	// Remove from subnotery feed
	subnoteryKey := subnoteryHotKey + strconv.FormatUint(uint64(note.SubnoteryID), 10)
	return handler.RDB.ZRem(ctx, subnoteryKey, noteID).Err()
}

// GetHotFeed returns the hot feed for a user (personalized) or globally (public).
// For logged-in users: subscribed subnoteries first, then global.
// For anonymous users: just global hot notes.
// GetHotFeed interacts with Redis to retrieve hot feed note IDs.
// GetHotFeed interacts with the database to fetch full note data.
// GetHotFeed interacts with no other handler methods.
func (handler *FeedHandler) GetHotFeed(c *gin.Context) {
	ctx := c.Request.Context()

	// Parse pagination params
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "25"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}
	offset := (page - 1) * limit

	// Check if user is authenticated (optional auth)
	userID, authenticated := c.Get("user_id")

	var noteIDs []string

	if authenticated {
		log.Println("Fetching personalized hot feed for user:", userID)
		noteIDs = handler.getPersonalizedFeed(ctx, userID.(uint64), offset, limit)
	} else {
		log.Println("Fetching global hot feed for anonymous user")
		noteIDs = handler.getGlobalFeed(ctx, offset, limit)
	}

	// Fetch full note data from database
	notes := handler.fetchNotes(noteIDs)

	c.JSON(http.StatusOK, gin.H{
		"notes": notes,
		"page":  page,
		"limit": limit,
	})
}

// getPersonalizedFeed returns note IDs from user's subscribed subnoteries merged with global.
// getPersonalizedFeed interacts with Redis to retrieve personalized hot feed note IDs.
// getPersonalizedFeed interacts with no other handler methods.
func (handler *FeedHandler) getPersonalizedFeed(ctx context.Context, userID uint64, offset, limit int) []string {
	// Get user's subscribed subnoteries
	var subnoteryIDs []uint
	handler.DB.Table("user_memberships").
		Where("user_id = ?", userID).
		Pluck("subnotery_id", &subnoteryIDs)

	if len(subnoteryIDs) == 0 {
		// No subscriptions, just return global
		return handler.getGlobalFeed(ctx, offset, limit)
	}

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
	handler.RDB.ZUnionStore(ctx, unionKey, &redis.ZStore{
		Keys:      keys,
		Weights:   weights,
		Aggregate: "MAX",
	})

	// Set expiry on union key (cleanup)
	handler.RDB.Expire(ctx, unionKey, 10*time.Second)

	// Get the hot notes from the union
	noteIDs, err := handler.RDB.ZRevRange(ctx, unionKey, int64(offset), int64(offset+limit-1)).Result()
	if err != nil {
		log.Println("Failed to get personalized feed:", err)
		return []string{}
	}

	return noteIDs
}

// getGlobalFeed returns note IDs from the global hot feed.
// getGlobalFeed interacts with Redis to retrieve global hot feed note IDs.
// getGlobalFeed interacts with no other handler methods.
func (handler *FeedHandler) getGlobalFeed(ctx context.Context, offset, limit int) []string {
	noteIDs, err := handler.RDB.ZRevRange(ctx, globalHotKey, int64(offset), int64(offset+limit-1)).Result()
	if err != nil {
		log.Println("Failed to get global feed:", err)
		return []string{}
	}
	return noteIDs
}

// fetchNotes retrieves full note data from the database, preserving order.
func (handler *FeedHandler) fetchNotes(noteIDs []string) []models.Note {
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
	handler.DB.Where("id IN ?", ids).Find(&notes)

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
// Upvote interacts with Redis and the database to record the vote and update hotness.
// Upvote interacts with no other handler methods.
func (handler *FeedHandler) Upvote(c *gin.Context) {
	ctx := c.Request.Context()
	noteID := c.Param("id")
	userID := c.MustGet("user_id").(uint64)

	log.Println("User", userID, "upvoting note", noteID)

	var note models.Note
	if err := handler.DB.First(&note, noteID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
		return
	}

	// Check if user already voted (using Redis set)
	voteKey := fmt.Sprintf("votes:%s", noteID)
	userVoteKey := fmt.Sprintf("%d", userID)

	// Get current vote state
	currentVote, _ := handler.RDB.HGet(ctx, voteKey, userVoteKey).Result()

	if currentVote == "up" {
		// Remove upvote (toggle off)
		handler.RDB.HDel(ctx, voteKey, userVoteKey)
		handler.DB.Model(&note).Update("upvotes", gorm.Expr("upvotes - 1"))
		note.Upvotes--
	} else {
		if currentVote == "down" {
			// Switch from downvote to upvote
			handler.DB.Model(&note).Update("downvotes", gorm.Expr("downvotes - 1"))
			note.Downvotes--
		}
		// Add upvote
		handler.RDB.HSet(ctx, voteKey, userVoteKey, "up")
		handler.DB.Model(&note).Update("upvotes", gorm.Expr("upvotes + 1"))
		note.Upvotes++
	}

	// Recalculate hotness
	if err := handler.UpdateNoteHotness(ctx, &note); err != nil {
		log.Println("Failed to update hotness:", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"upvotes":   note.Upvotes,
		"downvotes": note.Downvotes,
		"hotness":   note.Hotness,
	})
}

// Downvote handles downvoting a note and updates hotness.
// Downvote interacts with Redis and the database to record the vote and update hotness.
// Downvote interacts with no other handler methods.
func (handler *FeedHandler) Downvote(c *gin.Context) {
	ctx := c.Request.Context()
	noteID := c.Param("id")
	userID := c.MustGet("user_id").(uint64)

	log.Println("User", userID, "downvoting note", noteID)

	var note models.Note
	if err := handler.DB.First(&note, noteID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
		return
	}

	voteKey := fmt.Sprintf("votes:%s", noteID)
	userVoteKey := fmt.Sprintf("%d", userID)

	currentVote, _ := handler.RDB.HGet(ctx, voteKey, userVoteKey).Result()

	if currentVote == "down" {
		// Remove downvote (toggle off)
		handler.RDB.HDel(ctx, voteKey, userVoteKey)
		handler.DB.Model(&note).Update("downvotes", gorm.Expr("downvotes - 1"))
		note.Downvotes--
	} else {
		if currentVote == "up" {
			// Switch from upvote to downvote
			handler.DB.Model(&note).Update("upvotes", gorm.Expr("upvotes - 1"))
			note.Upvotes--
		}
		// Add downvote
		handler.RDB.HSet(ctx, voteKey, userVoteKey, "down")
		handler.DB.Model(&note).Update("downvotes", gorm.Expr("downvotes + 1"))
		note.Downvotes++
	}

	// Recalculate hotness
	if err := handler.UpdateNoteHotness(ctx, &note); err != nil {
		log.Println("Failed to update hotness:", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"upvotes":   note.Upvotes,
		"downvotes": note.Downvotes,
		"hotness":   note.Hotness,
	})
}
