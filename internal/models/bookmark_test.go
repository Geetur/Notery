package models

import (
	"testing"
	"time"
)

func TestBookmark_TableName(t *testing.T) {
	b := Bookmark{}
	if b.TableName() != "bookmarks" {
		t.Fatalf("unexpected table name: %s", b.TableName())
	}
}

func TestBookmark_Fields(t *testing.T) {
	now := time.Now()
	b := Bookmark{
		ID:        1,
		UserID:    42,
		NoteID:    99,
		CreatedAt: now,
	}
	if b.UserID != 42 {
		t.Fatalf("UserID=%d, want 42", b.UserID)
	}
	if b.NoteID != 99 {
		t.Fatalf("NoteID=%d, want 99", b.NoteID)
	}
	if !b.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt mismatch")
	}
}
