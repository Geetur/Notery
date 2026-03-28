// karma.go — Notoriety (karma) system: ledger tracking and delta algorithms.
//
// DESIGN:
// -------
// Each vote (note or comment) records a KarmaLedger row containing the delta
// applied to the author's karma. When a vote is toggled off or switched, the
// prior ledger row is used to reverse the delta exactly, preventing drift.
//
// ALGORITHMS:
// -----------
// Post karma uses   K=20, N0=25   (diminishing returns on popular posts).
// Comment karma uses Kc=10, N0c=40 (tighter confidence gate for comments).
//
//	base  = K / (K + max(0, S))
//	conf  = min(1, ln(1+N) / ln(1+N0))
//	delta = v * base * conf
//
// Where S = upvotes − downvotes, N = upvotes + downvotes, v = +1 or −1.
//
// STORAGE:
// --------
// Karma is stored as float64 on the User model. The ledger stores the exact
// delta per vote so that undo operations are precise.
package models

import (
	"math"
	"time"
)

// KarmaType distinguishes post karma from comment karma.
type KarmaType string

const (
	KarmaPost    KarmaType = "post"
	KarmaComment KarmaType = "comment"
)

// KarmaLedger records the karma delta applied for a single vote event.
// This enables exact reversal when a vote is toggled off or switched.
//
// Indexes:
//   - (vote_type, vote_id) — lookup ledger row for a specific vote
//   - author_id           — audit trail for a user's karma history
type KarmaLedger struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	AuthorID  uint64    `json:"author_id" gorm:"index;not null"`                         // user who received the karma
	VoterID   uint64    `json:"voter_id" gorm:"not null"`                                // user who cast the vote
	VoteType  KarmaType `json:"vote_type" gorm:"not null"`                               // "post" or "comment"
	VoteID    uint      `json:"vote_id" gorm:"index:idx_karma_vote_type_id;not null"`    // Vote.ID or CommentVote.ID
	TargetID  uint64    `json:"target_id" gorm:"not null"`                               // Note.ID or Comment.ID
	KarmaType KarmaType `json:"karma_type" gorm:"index:idx_karma_vote_type_id;not null"` // redundant with vote_type for compound index
	Delta     float64   `json:"delta" gorm:"not null"`                                   // the exact karma change applied
	CreatedAt time.Time `json:"created_at"`
}

// ── Post Karma Constants ──────────────────────────────────────────────────────

const (
	// postK controls diminishing returns: higher → slower decay.
	postK float64 = 20
	// postN0 is the confidence gate: roughly how many votes before full weight.
	postN0 float64 = 25
)

// ── Comment Karma Constants ───────────────────────────────────────────────────

const (
	commentK  float64 = 10
	commentN0 float64 = 40
)

// CalculatePostKarmaDelta computes the karma change for a single note vote.
//
// Parameters:
//   - v:         +1 for upvote, -1 for downvote
//   - upvotes:   total upvotes on the note AFTER applying the vote
//   - downvotes: total downvotes on the note AFTER applying the vote
//
// Returns the delta to add to the author's PostKarma.
func CalculatePostKarmaDelta(v int, upvotes, downvotes uint64) float64 {
	return calculateKarmaDelta(v, int64(upvotes), int64(downvotes), postK, postN0)
}

// CalculateCommentKarmaDelta computes the karma change for a single comment vote.
//
// Parameters:
//   - v:         +1 for upvote, -1 for downvote
//   - upvotes:   total upvotes on the comment AFTER applying the vote
//   - downvotes: total downvotes on the comment AFTER applying the vote
//
// Returns the delta to add to the author's CommentKarma.
func CalculateCommentKarmaDelta(v int, upvotes, downvotes int64) float64 {
	return calculateKarmaDelta(v, upvotes, downvotes, commentK, commentN0)
}

// calculateKarmaDelta is the unified algorithm for both post and comment karma.
//
//	base  = K / (K + max(0, S))
//	conf  = min(1, ln(1+N) / ln(1+N0))
//	delta = v * base * conf
func calculateKarmaDelta(v int, upvotes, downvotes int64, k, n0 float64) float64 {
	s := float64(upvotes - downvotes) // net score
	n := float64(upvotes + downvotes) // total votes

	maxS := math.Max(0, s)
	base := k / (k + maxS)

	conf := 1.0
	if n0 > 0 {
		lnDenom := math.Log(1 + n0)
		if lnDenom > 0 {
			conf = math.Min(1, math.Log(1+n)/lnDenom)
		}
	}

	return float64(v) * base * conf
}
