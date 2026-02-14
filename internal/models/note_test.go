package models

import (
	"testing"
)

func TestNoteStatus_Constants(t *testing.T) {
	// Verify status constants are distinct
	statuses := []NoteStatus{StatusPending, StatusApproved, StatusRejected}
	seen := make(map[NoteStatus]bool)
	for _, s := range statuses {
		if seen[s] {
			t.Fatalf("duplicate status: %s", s)
		}
		seen[s] = true
	}
}

func TestNoteStatus_Values(t *testing.T) {
	if StatusPending != "Pending" {
		t.Fatalf("StatusPending = %q, want Pending", StatusPending)
	}
	if StatusApproved != "Approved" {
		t.Fatalf("StatusApproved = %q, want Approved", StatusApproved)
	}
	if StatusRejected != "Rejected" {
		t.Fatalf("StatusRejected = %q, want Rejected", StatusRejected)
	}
}

func TestNote_DefaultValues(t *testing.T) {
	note := Note{}
	if note.Upvotes != 0 {
		t.Fatalf("expected 0 upvotes by default, got %d", note.Upvotes)
	}
	if note.Downvotes != 0 {
		t.Fatalf("expected 0 downvotes by default, got %d", note.Downvotes)
	}
	if note.HasPDF {
		t.Fatal("expected HasPDF=false by default")
	}
}
