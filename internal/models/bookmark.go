// Package models/bookmark.go contains the Bookmark (saved note) model.
//
// DESIGN:
// -------
// Users can save/bookmark notes for later viewing. This is a simple
// many-to-many relationship between users and notes.
//
// A composite unique index on (user_id, note_id) prevents duplicate bookmarks.
// Only approved notes can be bookmarked (enforced at the handler level).
package models

import "time"

// Bookmark represents a user's saved/bookmarked note.
// This enables the "Saved Notes" feature similar to Reddit's save functionality.
type Bookmark struct {
	ID     uint   `json:"id" gorm:"primaryKey"`
	UserID uint64 `json:"user_id" gorm:"uniqueIndex:idx_bookmark_user_note;index;not null"`
	NoteID uint64 `json:"note_id" gorm:"uniqueIndex:idx_bookmark_user_note;index;not null"`

	// CreatedAt records when the bookmark was created (for chronological listing).
	CreatedAt time.Time `json:"created_at"`
}

// TableName overrides GORM's default table name.
func (Bookmark) TableName() string {
	return "bookmarks"
}
