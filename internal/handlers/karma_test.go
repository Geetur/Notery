package handlers

import (
	"net/http"
	"testing"

	"github.com/Geetur/Notery/internal/models"
)

// ===== GET /me/karma =====

func TestGetMyKarma_Default(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "karmadefault")

	w := serve("GET", "/me/karma", "/me/karma", nil,
		app.GetMyKarma, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["note_karma"].(float64) != 0 {
		t.Fatalf("note_karma=%v, want 0", r["note_karma"])
	}
	if r["comment_karma"].(float64) != 0 {
		t.Fatalf("comment_karma=%v, want 0", r["comment_karma"])
	}
	if r["total_karma"].(float64) != 0 {
		t.Fatalf("total_karma=%v, want 0", r["total_karma"])
	}
}

func TestGetMyKarma_WithCachedValues(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "karmacached")

	// Set cached karma
	app.DB.Model(&models.User{}).Where("id = ?", uid).Updates(map[string]interface{}{
		"note_karma":    10,
		"comment_karma": 5,
	})

	w := serve("GET", "/me/karma", "/me/karma", nil,
		app.GetMyKarma, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["note_karma"].(float64) != 10 {
		t.Fatalf("note_karma=%v, want 10", r["note_karma"])
	}
	if r["comment_karma"].(float64) != 5 {
		t.Fatalf("comment_karma=%v, want 5", r["comment_karma"])
	}
	if r["total_karma"].(float64) != 15 {
		t.Fatalf("total_karma=%v, want 15", r["total_karma"])
	}
}

// ===== POST /me/karma/refresh =====

func TestRecalculateKarma_FromNoteVotes(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "karmarecalc")
	noteID := seedApprovedNote(t, app.DB, uid)

	// Add some votes on the note
	app.DB.Model(&models.Note{}).Where("id = ?", noteID).Updates(map[string]interface{}{
		"upvotes":   7,
		"downvotes": 2,
	})

	w := serve("POST", "/me/karma/refresh", "/me/karma/refresh", nil,
		app.RecalculateMyKarma, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["note_karma"].(float64) != 5 {
		t.Fatalf("note_karma=%v, want 5 (7-2)", r["note_karma"])
	}

	// Verify it's persisted
	var user models.User
	app.DB.First(&user, uid)
	if user.NoteKarma != 5 {
		t.Fatalf("persisted note_karma=%d, want 5", user.NoteKarma)
	}
}

func TestRecalculateKarma_FromCommentVotes(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "karmacmt")
	noteID := seedApprovedNote(t, app.DB, uid)

	// Create a comment with votes
	comment := models.Comment{
		NoteID:    noteID,
		UserID:    uid,
		Body:      "Great note!",
		Upvotes:   10,
		Downvotes: 3,
	}
	app.DB.Create(&comment)

	w := serve("POST", "/me/karma/refresh", "/me/karma/refresh", nil,
		app.RecalculateMyKarma, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["comment_karma"].(float64) != 7 {
		t.Fatalf("comment_karma=%v, want 7 (10-3)", r["comment_karma"])
	}
}

func TestRecalculateKarma_OnlyApprovedNotes(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "kapproved")

	// Create an approved note with votes
	approvedID := seedApprovedNote(t, app.DB, uid)
	app.DB.Model(&models.Note{}).Where("id = ?", approvedID).Updates(map[string]interface{}{
		"upvotes": 10, "downvotes": 0,
	})

	// Create a pending note with votes (should NOT count)
	pendingID := seedPendingNote(t, app.DB, uid)
	app.DB.Model(&models.Note{}).Where("id = ?", pendingID).Updates(map[string]interface{}{
		"upvotes": 100, "downvotes": 0,
	})

	w := serve("POST", "/me/karma/refresh", "/me/karma/refresh", nil,
		app.RecalculateMyKarma, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["note_karma"].(float64) != 10 {
		t.Fatalf("note_karma=%v, want 10 (pending excluded)", r["note_karma"])
	}
}

func TestRecalculateKarma_NegativeKarma(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "kaneg")
	noteID := seedApprovedNote(t, app.DB, uid)

	app.DB.Model(&models.Note{}).Where("id = ?", noteID).Updates(map[string]interface{}{
		"upvotes": 2, "downvotes": 10,
	})

	w := serve("POST", "/me/karma/refresh", "/me/karma/refresh", nil,
		app.RecalculateMyKarma, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["note_karma"].(float64) != -8 {
		t.Fatalf("note_karma=%v, want -8", r["note_karma"])
	}
	if r["total_karma"].(float64) < 0 {
		// Expected negative total
	}
}
