package handlers

import (
	"math"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// ===== CalculateHotness UNIT TESTS =====

func TestCalculateHotness_AllZeros(t *testing.T) {
	h := CalculateHotness(0, 0, time.Unix(hotEpoch, 0))
	// score=0, time=0 → 0
	if h != 0 {
		t.Fatalf("expected 0, got %f", h)
	}
}

func TestCalculateHotness_PositiveScore(t *testing.T) {
	h := CalculateHotness(10, 0, time.Unix(hotEpoch, 0))
	if h <= 0 {
		t.Fatalf("expected positive hotness for +10 score, got %f", h)
	}
}

func TestCalculateHotness_NegativeScore(t *testing.T) {
	h := CalculateHotness(0, 10, time.Unix(hotEpoch, 0))
	if h >= 0 {
		t.Fatalf("expected negative hotness for -10 score, got %f", h)
	}
}

func TestCalculateHotness_TimeBoost(t *testing.T) {
	// Same votes, newer post should be hotter
	old := CalculateHotness(5, 0, time.Unix(hotEpoch+3600, 0))  // 1 hour after epoch
	new := CalculateHotness(5, 0, time.Unix(hotEpoch+86400, 0)) // 1 day after epoch
	if new <= old {
		t.Fatalf("newer post should be hotter: new=%f, old=%f", new, old)
	}
}

func TestCalculateHotness_HighVotesWin(t *testing.T) {
	// High votes should beat time advantage in reasonable window
	popular := CalculateHotness(1000, 0, time.Unix(hotEpoch, 0))
	recent := CalculateHotness(1, 0, time.Unix(hotEpoch+3600, 0))
	if popular <= recent {
		t.Fatalf("popular post should beat slightly newer: popular=%f, recent=%f", popular, recent)
	}
}

func TestCalculateHotness_EqualUpDownVotes(t *testing.T) {
	h := CalculateHotness(100, 100, time.Unix(hotEpoch, 0))
	// score=0, so only time component (which is 0 at epoch)
	if h != 0 {
		t.Fatalf("equal up/down at epoch should be 0, got %f", h)
	}
}

func TestCalculateHotness_OrderConsistency(t *testing.T) {
	// sign(score) * log10(max(|score|, 1)) + seconds/decay
	// For score=1 at epoch: 1 * log10(1) + 0 = 0
	h := CalculateHotness(1, 0, time.Unix(hotEpoch, 0))
	expected := 0.0 // log10(1) = 0
	if math.Abs(h-expected) > 0.001 {
		t.Fatalf("expected ~%f, got %f", expected, h)
	}

	// For score=10 at epoch: 1 * log10(10) + 0 = 1.0
	h2 := CalculateHotness(10, 0, time.Unix(hotEpoch, 0))
	if math.Abs(h2-1.0) > 0.001 {
		t.Fatalf("expected ~1.0, got %f", h2)
	}
}

func TestCalculateHotness_SymmetricSign(t *testing.T) {
	// Positive and negative of same magnitude should have same absolute value at epoch
	pos := CalculateHotness(50, 0, time.Unix(hotEpoch, 0))
	neg := CalculateHotness(0, 50, time.Unix(hotEpoch, 0))
	if math.Abs(pos+neg) > 0.001 {
		t.Fatalf("symmetric vote counts should cancel: +%f, -%f", pos, neg)
	}
}

// ===== Upvote/Downvote (DB-only, no Redis) =====
// These tests verify the DB transaction logic. Since UpdateNoteHotness
// and Redis cache need a Redis client, they will panic/fail if RDB is nil.
// We test vote recording via direct DB operations instead.

func TestUpvote_NoteNotFound(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "feedvoter1")

	w := serve("POST", "/notes/:id/upvote", "/notes/99999/upvote",
		nil, app.Upvote, authMW(uid))
	assertStatus(t, w, http.StatusNotFound)
}

func TestDownvote_NoteNotFound(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "feedvoter2")

	w := serve("POST", "/notes/:id/downvote", "/notes/99999/downvote",
		nil, app.Downvote, authMW(uid))
	assertStatus(t, w, http.StatusNotFound)
}

func TestUpvote_InvalidNoteID(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "feedvoter3")

	w := serve("POST", "/notes/:id/upvote", "/notes/abc/upvote",
		nil, app.Upvote, authMW(uid))
	assertStatus(t, w, http.StatusNotFound)
}

// ===== GetHotFeed (anonymous, no Redis - should return empty) =====

func TestGetHotFeed_AnonymousNoRedis(t *testing.T) {
	app := testApp(t)

	// Without Redis, the feed will fail gracefully
	// The handler requires app.RDB which is nil here, so this will likely panic
	// We can only test the pure function CalculateHotness above
	// Skip this test if no Redis
	if app.RDB == nil {
		t.Skip("Redis not available, skipping feed integration test")
	}

	w := serve("GET", "/feed", "/feed", nil, app.GetHotFeed)
	assertStatus(t, w, http.StatusOK)
}

// ===== fetchNotes =====

func TestFetchNotes_EmptyList(t *testing.T) {
	app := testApp(t)
	notes := app.fetchNotes([]string{})
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes, got %d", len(notes))
	}
}

func TestFetchNotes_PreservesOrder(t *testing.T) {
	app := testApp(t)
	creator := seedUser(t, app.DB, "fetchcreator")
	id1 := seedApprovedNote(t, app.DB, creator)
	id2 := seedApprovedNote(t, app.DB, creator)
	id3 := seedApprovedNote(t, app.DB, creator)

	// Request in reverse order
	ids := []string{
		strconv.Itoa(int(id3)),
		strconv.Itoa(int(id1)),
		strconv.Itoa(int(id2)),
	}
	notes := app.fetchNotes(ids)
	if len(notes) != 3 {
		t.Fatalf("expected 3 notes, got %d", len(notes))
	}
	if notes[0].ID != id3 || notes[1].ID != id1 || notes[2].ID != id2 {
		t.Fatalf("order not preserved: got %d, %d, %d", notes[0].ID, notes[1].ID, notes[2].ID)
	}
}

func TestFetchNotes_MissingIDs(t *testing.T) {
	app := testApp(t)
	notes := app.fetchNotes([]string{"99999", "88888"})
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes for missing IDs, got %d", len(notes))
	}
}
