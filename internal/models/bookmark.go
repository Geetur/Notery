// bookmark.go — Bookmark model for saving notes.
//
// A bookmark lets a user save an approved note for later viewing.
// The (user_id, note_id) pair is unique — a user can only bookmark a note once.
package models

import "time"

// Bookmark represents a user's saved note.
type Bookmark struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint64    `json:"user_id" gorm:"uniqueIndex:idx_bookmarks_user_note;not null"`
	NoteID    uint      `json:"note_id" gorm:"uniqueIndex:idx_bookmarks_user_note;not null"`
	CreatedAt time.Time `json:"created_at"`
}
