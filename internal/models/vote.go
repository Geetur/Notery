// vote.go — Vote model: tracks user votes on notes.
package models

// VoteDirection represents the direction of a user's vote.
type VoteDirection string

const (
	VoteUp   VoteDirection = "up"
	VoteDown VoteDirection = "down"
)

// Vote records a single user's vote on a note.
// The composite unique index on (user_id, note_id) ensures one vote per user per note.
// This is the authoritative source of truth for vote state; Redis is used only as a cache.
type Vote struct {
	ID        uint          `json:"id" gorm:"primaryKey"`
	UserID    uint64        `json:"user_id" gorm:"uniqueIndex:idx_vote_user_note;not null"`
	NoteID    uint64        `json:"note_id" gorm:"uniqueIndex:idx_vote_user_note;index;not null"`
	Direction VoteDirection `json:"direction" gorm:"not null"`
}
