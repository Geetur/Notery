// Package models/comment.go contains the Comment and CommentVote models,
// plus the Wilson score lower bound algorithm used for Reddit-style "Best" ranking.
//
// DESIGN:
// -------
// Comments form a tree rooted at notes. Top-level comments have ParentID == nil;
// replies have ParentID pointing to their parent comment (must be on the same note).
// Depth is precomputed: 0 for top-level, parent.Depth+1 for replies.
//
// RANKING:
// --------
// Wilson score lower bound is the same algorithm Reddit uses for "Best" sorting.
// It answers: "Given the votes observed, what is the WORST the true quality could be
// at 95% confidence?" This means comments with more votes are ranked more confidently —
// a comment with 1 up / 0 down ranks LOWER than one with 100 up / 10 down because
// we're less certain about the first comment.
//
// No time factor — quality always wins regardless of when a comment was posted.
//
// SOFT DELETE:
// ------------
// Deleted comments have IsDeleted set to true. The API replaces their body with
// "[deleted]" and omits the author, but preserves the tree structure so child
// replies remain readable. A background job can prune leaf-deleted comments.
package models

import (
	"math"
	"time"
)

// CommentSortOrder represents the available sort options for comments.
type CommentSortOrder string

const (
	// SortBest ranks by Wilson score lower bound (default, like Reddit's "Best").
	// High-confidence positive comments win.
	SortBest CommentSortOrder = "best"

	// SortNew ranks by creation time, newest first.
	SortNew CommentSortOrder = "new"

	// SortTop ranks by net score (upvotes − downvotes), highest first.
	SortTop CommentSortOrder = "top"

	// SortControversial ranks by total votes with net score closest to zero.
	// Comments with many votes but an evenly split opinion rank highest.
	SortControversial CommentSortOrder = "controversial"

	// SortOld ranks by creation time, oldest first.
	SortOld CommentSortOrder = "old"
)

// ValidSortOrder returns true if the given sort order is recognized.
func ValidSortOrder(s CommentSortOrder) bool {
	switch s {
	case SortBest, SortNew, SortTop, SortControversial, SortOld:
		return true
	}
	return false
}

// MaxCommentBodyLength is the maximum allowed length for a comment body (in runes).
// Reddit's limit is ~10 000 characters; we match that.
const MaxCommentBodyLength = 10000

// EditGracePeriod is the duration after creation during which edits don't show
// as "edited". Matches Reddit's ~3-minute grace window.
const EditGracePeriod = 3 * time.Minute

// MaxTreeDepth is the maximum depth of nested replies returned in a single response.
// Beyond this depth, children are omitted and the response includes a "has_more_replies"
// flag so the frontend can display "Continue this thread →".
// Reddit uses ~10 levels of nesting.
const MaxTreeDepth = 10

// MaxWriteDepth is the maximum allowed nesting depth when creating a reply.
// Prevents DoS via pathologically deep comment chains that bloat memory during
// recursive tree assembly. Separate from MaxTreeDepth (a read-time display limit).
const MaxWriteDepth = 15

// MaxNodesPerRequest is the absolute per-request budget for comment nodes returned.
// Even if depth and pagination would allow more, we cap total tree nodes to prevent
// unbounded memory use. The response includes continuation metadata when truncated.
const MaxNodesPerRequest = 500

// Comment represents a user's comment on a note.
//
// TREE STRUCTURE:
//   - ParentID == nil → top-level comment on the note
//   - ParentID != nil → reply to another comment (must be on the same note)
//   - Depth is precomputed: 0 for top-level, parent.Depth+1 for replies
//   - Path is a materialized path string for efficient subtree queries:
//     Top-level: "/42/"  Reply: "/42/51/63/"
//
// INDEXES (composite for query patterns):
//   - idx_comment_note:            note_id — fast lookup of all comments
//   - idx_comment_parent:          parent_id — fast child lookups
//   - idx_comment_note_parent:     (note_id, parent_id) — root + child partitions
//   - idx_comment_note_score:      (note_id, score) — paginated "best" sort on roots
//   - idx_comment_note_created:    (note_id, created_at) — "new" / "old" sort
//   - idx_comment_note_depth:      (note_id, depth) — bounded-depth traversals
//   - idx_comment_path:            path — materialized-path subtree queries (LIKE prefix)
//   - (user_id): find all comments by a user
type Comment struct {
	ID       uint   `json:"id" gorm:"primaryKey"`
	NoteID   uint   `json:"note_id" gorm:"index:idx_comment_note;index:idx_comment_note_parent;index:idx_comment_note_score;index:idx_comment_note_created;index:idx_comment_note_depth;not null"`
	UserID   uint64 `json:"user_id" gorm:"index;not null"`
	ParentID *uint  `json:"parent_id" gorm:"index:idx_comment_parent;index:idx_comment_note_parent"`

	// Body is the comment text. Max MaxCommentBodyLength runes.
	Body string `json:"body" gorm:"type:text;not null"`

	// Path is the materialized path for this comment in the tree.
	// Format: "/<root_id>/<child_id>/.../<this_id>/"
	// Top-level example: "/42/"   Reply example: "/42/51/63/"
	// Used for exact subtree queries: WHERE path LIKE '/42/%'
	// Nullable for backwards compatibility — empty means not yet backfilled.
	Path string `json:"path,omitempty" gorm:"type:text;default:'';index:idx_comment_path"`

	// Cached vote counts — updated atomically via gorm.Expr.
	// These are the DB-authoritative source of truth.
	Upvotes   int64 `json:"upvotes" gorm:"not null;default:0"`
	Downvotes int64 `json:"downvotes" gorm:"not null;default:0"`

	// Score is the Wilson score lower bound, precomputed for efficient sorting.
	// Recalculated atomically alongside vote count updates inside a transaction.
	Score float64 `json:"score" gorm:"not null;default:0;index:idx_comment_note_score,sort:desc"`

	// Depth tracks nesting level: 0 = top-level, parent.Depth+1 for replies.
	// Stored (not computed on read) for efficient depth-limited queries.
	Depth int `json:"depth" gorm:"not null;default:0;index:idx_comment_note_depth"`

	// IsDeleted marks a comment as soft-deleted.
	// Soft-deleted comments display "[deleted]" but maintain tree structure.
	IsDeleted bool `json:"is_deleted" gorm:"not null;default:false"`

	// EditedAt records when the comment was last edited outside the grace period.
	// nil means never edited (or edited within EditGracePeriod of creation).
	EditedAt *time.Time `json:"edited_at,omitempty"`

	CreatedAt time.Time `json:"created_at" gorm:"index:idx_comment_note_created"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CommentVote records a single user's vote on a comment.
// The composite unique index on (comment_id, user_id) ensures one vote per user per comment.
// Value is +1 (upvote) or -1 (downvote).
type CommentVote struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CommentID uint      `json:"comment_id" gorm:"uniqueIndex:idx_comment_vote_user;index;not null"`
	UserID    uint64    `json:"user_id" gorm:"uniqueIndex:idx_comment_vote_user;not null"`
	Value     int8      `json:"value" gorm:"not null"` // +1 or -1
	CreatedAt time.Time `json:"created_at"`
}

// ----- Wilson Score Lower Bound Algorithm -----

// wilsonZ is the z-score for a 95% confidence interval (1.96).
const wilsonZ = 1.96

// WilsonScore computes the lower bound of the Wilson score confidence interval
// for the true proportion of positive ratings. This is the algorithm Reddit uses
// for "Best" comment sorting.
//
// It answers: "Given the votes we've seen, what is the WORST this comment's true
// quality could be at 95% confidence?"
//
// Properties:
//   - A comment with 1 up / 0 down ranks LOWER than 100 up / 10 down,
//     because we have less confidence about the first comment's quality.
//   - Handles small sample sizes gracefully via confidence intervals.
//   - No time factor — purely quality-based.
//   - Returns 0 when there are no votes.
//
// Formula (Wilson score confidence interval lower bound):
//
//	lower = (p̂ + z²/2n − z√(p̂(1−p̂)/n + z²/4n²)) / (1 + z²/n)
//
// Where:
//   - p̂ = upvotes / (upvotes + downvotes)
//   - n = upvotes + downvotes (total votes)
//   - z = 1.96 (95% confidence)
func WilsonScore(upvotes, downvotes int64) float64 {
	n := float64(upvotes + downvotes)
	if n == 0 {
		return 0
	}

	p := float64(upvotes) / n
	z := wilsonZ
	z2 := z * z

	denominator := 1.0 + z2/n
	centre := p + z2/(2.0*n)
	spread := z * math.Sqrt((p*(1.0-p)+z2/(4.0*n))/n)

	return (centre - spread) / denominator
}

// ControversyScore computes a "controversy" ranking score.
// Comments with many votes but a net score close to zero are most controversial.
//
// Formula: total_votes / max(|net_score|, 1)
// Higher = more controversial.
func ControversyScore(upvotes, downvotes int64) float64 {
	total := float64(upvotes + downvotes)
	if total == 0 {
		return 0
	}
	net := math.Abs(float64(upvotes - downvotes))
	if net < 1 {
		net = 1
	}
	return total / net
}
