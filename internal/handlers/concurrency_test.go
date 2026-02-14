package handlers

import (
	"bytes"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Geetur/Notery/internal/models"
)

// ===== CONCURRENCY TEST HELPERS =====

// fanOut launches n goroutines, each calling fn(i), and waits for all to finish.
// All goroutines start simultaneously via a shared barrier (WaitGroup).
func fanOut(n int, fn func(i int)) {
	var ready sync.WaitGroup
	ready.Add(1) // barrier: all goroutines wait for this

	var done sync.WaitGroup
	done.Add(n)

	for i := 0; i < n; i++ {
		go func(idx int) {
			defer done.Done()
			ready.Wait() // block until barrier is released
			fn(idx)
		}(i)
	}

	ready.Done() // release barrier — all goroutines start simultaneously
	done.Wait()  // wait for all to finish
}

// serveWithRetry calls serve() and retries up to maxRetries times on server errors.
// SQLite's single-writer model causes "database table is locked" under true concurrency,
// which can surface as 500 (transaction failed) or 404 (pre-transaction lookup failed).
// In production (PostgreSQL), these retries would never fire.
func serveWithRetry(
	method, routePattern, url string,
	body *bytes.Reader,
	handler gin.HandlerFunc,
	mw gin.HandlerFunc,
	maxRetries int,
) *httptest.ResponseRecorder {
	var w *httptest.ResponseRecorder
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Reset body reader position for retries
		if body != nil {
			body.Seek(0, 0)
		}
		w = serve(method, routePattern, url, body, handler, mw)
		// Only retry on 500 Internal Server Error (lock contention in transaction)
		// or 404 when it's a write path (lock contention on pre-transaction lookup)
		if w.Code < 500 && !(w.Code == http.StatusNotFound && method != "GET") {
			return w
		}
		// Jittered exponential backoff: base * 2^attempt + random jitter
		base := time.Duration(1<<uint(min(attempt, 8))) * time.Millisecond
		jitter := time.Duration(rand.Intn(int(base/2) + 1))
		time.Sleep(base + jitter)
	}
	return w
}

// countVoteRows returns the total number of CommentVote rows for a given comment.
func countVoteRows(app *App, commentID uint) int64 {
	var count int64
	app.DB.Model(&models.CommentVote{}).Where("comment_id = ?", commentID).Count(&count)
	return count
}

// countNoteVoteRows returns the total number of Vote rows for a given note.
func countNoteVoteRows(app *App, noteID uint) int64 {
	var count int64
	app.DB.Model(&models.Vote{}).Where("note_id = ?", noteID).Count(&count)
	return count
}

// reloadComment fetches a fresh comment from the DB.
func reloadComment(app *App, id uint) models.Comment {
	var c models.Comment
	app.DB.First(&c, id)
	return c
}

// reloadNote fetches a fresh note from the DB.
func reloadNote(app *App, id uint) models.Note {
	var n models.Note
	app.DB.First(&n, id)
	return n
}

// ===== A1: COMMENT VOTE — MULTI-USER UPVOTE CONTENTION =====
// N distinct users all upvote the same comment simultaneously.
// Invariant: upvotes == N, downvotes == 0, exactly N vote rows, score == WilsonScore(N, 0).

func TestConcurrency_CommentVote_MultiUserUpvote(t *testing.T) {
	app := testApp(t)
	const numVoters = 20

	creator := seedUser(t, app.DB, "cv_creator")
	noteID := seedApprovedNote(t, app.DB, creator)

	comment := models.Comment{NoteID: noteID, UserID: creator, Body: "contention target"}
	app.DB.Create(&comment)

	// Create N distinct voters
	voterIDs := make([]uint64, numVoters)
	for i := 0; i < numVoters; i++ {
		voterIDs[i] = seedUser(t, app.DB, fmt.Sprintf("cv_voter_%d", i))
	}

	url := fmt.Sprintf("/comments/%d/vote", comment.ID)
	var failures atomic.Int32

	fanOut(numVoters, func(i int) {
		w := serveWithRetry("POST", "/comments/:comment_id/vote", url,
			jsonBody(map[string]int8{"value": 1}), app.VoteComment, authMW(voterIDs[i]), 20)
		if w.Code != http.StatusOK {
			failures.Add(1)
		}
	})

	if f := failures.Load(); f > 0 {
		t.Fatalf("%d/%d concurrent upvotes failed", f, numVoters)
	}

	// Verify invariants
	c := reloadComment(app, comment.ID)

	if c.Upvotes != int64(numVoters) {
		t.Errorf("upvotes=%d, want %d", c.Upvotes, numVoters)
	}
	if c.Downvotes != 0 {
		t.Errorf("downvotes=%d, want 0", c.Downvotes)
	}

	voteCount := countVoteRows(app, comment.ID)
	if voteCount != int64(numVoters) {
		t.Errorf("vote rows=%d, want %d", voteCount, numVoters)
	}

	expectedScore := models.WilsonScore(int64(numVoters), 0)
	if c.Score != expectedScore {
		t.Errorf("score=%f, want WilsonScore(%d, 0)=%f", c.Score, numVoters, expectedScore)
	}
}

// ===== A2: COMMENT VOTE — SAME-USER TOGGLE CONTENTION =====
// One user rapidly toggles upvote on the same comment N times concurrently.
// SQLite serializes these. After all settle, the final state must be consistent:
// - vote row count is 0 or 1
// - upvotes matches vote row count
// - downvotes == 0
// - score matches counters

func TestConcurrency_CommentVote_SameUserToggle(t *testing.T) {
	app := testApp(t)
	const numToggles = 15

	uid := seedUser(t, app.DB, "cv_toggler")
	noteID := seedApprovedNote(t, app.DB, uid)

	comment := models.Comment{NoteID: noteID, UserID: uid, Body: "toggle target"}
	app.DB.Create(&comment)

	url := fmt.Sprintf("/comments/%d/vote", comment.ID)

	fanOut(numToggles, func(i int) {
		serveWithRetry("POST", "/comments/:comment_id/vote", url,
			jsonBody(map[string]int8{"value": 1}), app.VoteComment, authMW(uid), 20)
	})

	// After all toggles, final state must be self-consistent
	c := reloadComment(app, comment.ID)
	voteCount := countVoteRows(app, comment.ID)

	// Must be exactly 0 or 1 vote rows
	if voteCount != 0 && voteCount != 1 {
		t.Fatalf("vote rows=%d, want 0 or 1 after toggle contention", voteCount)
	}

	// Counters must be non-negative
	if c.Upvotes < 0 {
		t.Errorf("upvotes=%d, must not be negative", c.Upvotes)
	}
	if c.Downvotes < 0 {
		t.Errorf("downvotes=%d, must not be negative", c.Downvotes)
	}

	// Upvotes must match vote row count
	if c.Upvotes != voteCount {
		t.Errorf("upvotes=%d but vote rows=%d — inconsistent", c.Upvotes, voteCount)
	}
	if c.Downvotes != 0 {
		t.Errorf("downvotes=%d, want 0 (only upvotes were toggled)", c.Downvotes)
	}

	// Score must reflect counters
	expectedScore := models.WilsonScore(c.Upvotes, c.Downvotes)
	if c.Score != expectedScore {
		t.Errorf("score=%f, want WilsonScore(%d, %d)=%f", c.Score, c.Upvotes, c.Downvotes, expectedScore)
	}
}

// ===== A3: COMMENT VOTE — MIXED UPVOTE/DOWNVOTE CONTENTION =====
// N/2 users upvote, N/2 users downvote the same comment simultaneously.
// Invariant: upvotes + downvotes == N, counters non-negative, score consistent.

func TestConcurrency_CommentVote_MixedDirections(t *testing.T) {
	app := testApp(t)
	const numVoters = 20

	creator := seedUser(t, app.DB, "cv_mix_creator")
	noteID := seedApprovedNote(t, app.DB, creator)

	comment := models.Comment{NoteID: noteID, UserID: creator, Body: "mixed vote target"}
	app.DB.Create(&comment)

	voterIDs := make([]uint64, numVoters)
	for i := 0; i < numVoters; i++ {
		voterIDs[i] = seedUser(t, app.DB, fmt.Sprintf("cv_mix_%d", i))
	}

	url := fmt.Sprintf("/comments/%d/vote", comment.ID)

	fanOut(numVoters, func(i int) {
		val := int8(1)
		if i%2 == 1 {
			val = -1
		}
		w := serveWithRetry("POST", "/comments/:comment_id/vote", url,
			jsonBody(map[string]int8{"value": val}), app.VoteComment, authMW(voterIDs[i]), 20)
		if w.Code != http.StatusOK {
			t.Errorf("voter %d: status=%d", i, w.Code)
		}
	})

	c := reloadComment(app, comment.ID)
	voteCount := countVoteRows(app, comment.ID)

	if voteCount != int64(numVoters) {
		t.Errorf("vote rows=%d, want %d", voteCount, numVoters)
	}

	if c.Upvotes < 0 || c.Downvotes < 0 {
		t.Errorf("negative counters: up=%d down=%d", c.Upvotes, c.Downvotes)
	}

	// Total votes must equal N
	totalVotes := c.Upvotes + c.Downvotes
	if totalVotes != int64(numVoters) {
		t.Errorf("upvotes(%d) + downvotes(%d) = %d, want %d", c.Upvotes, c.Downvotes, totalVotes, numVoters)
	}

	// Should be evenly split
	expectedUp := int64(numVoters / 2)
	expectedDown := int64(numVoters / 2)
	if c.Upvotes != expectedUp || c.Downvotes != expectedDown {
		t.Errorf("expected up=%d down=%d, got up=%d down=%d", expectedUp, expectedDown, c.Upvotes, c.Downvotes)
	}

	// Score must match counters
	expectedScore := models.WilsonScore(c.Upvotes, c.Downvotes)
	if c.Score != expectedScore {
		t.Errorf("score=%f, want WilsonScore(%d, %d)=%f", c.Score, c.Upvotes, c.Downvotes, expectedScore)
	}
}

// ===== A4: COMMENT VOTE REMOVAL UNDER CONTENTION =====
// N users all upvote, then all concurrently remove their votes.
// Invariant: 0 vote rows, upvotes == 0, downvotes == 0 after removal.

func TestConcurrency_CommentVote_RemoveContention(t *testing.T) {
	app := testApp(t)
	const numVoters = 15

	creator := seedUser(t, app.DB, "cv_rem_creator")
	noteID := seedApprovedNote(t, app.DB, creator)

	comment := models.Comment{NoteID: noteID, UserID: creator, Body: "remove contention"}
	app.DB.Create(&comment)

	voterIDs := make([]uint64, numVoters)
	for i := 0; i < numVoters; i++ {
		voterIDs[i] = seedUser(t, app.DB, fmt.Sprintf("cv_rem_%d", i))
	}

	voteURL := fmt.Sprintf("/comments/%d/vote", comment.ID)

	// Phase 1: all upvote sequentially (deterministic setup)
	for i := 0; i < numVoters; i++ {
		w := serve("POST", "/comments/:comment_id/vote", voteURL,
			jsonBody(map[string]int8{"value": 1}), app.VoteComment, authMW(voterIDs[i]))
		if w.Code != http.StatusOK {
			t.Fatalf("setup vote %d: status=%d", i, w.Code)
		}
	}

	// Verify setup
	c := reloadComment(app, comment.ID)
	if c.Upvotes != int64(numVoters) {
		t.Fatalf("setup check: upvotes=%d, want %d", c.Upvotes, numVoters)
	}

	// Phase 2: all remove votes concurrently
	fanOut(numVoters, func(i int) {
		serveWithRetry("DELETE", "/comments/:comment_id/vote", voteURL, nil,
			app.RemoveCommentVote, authMW(voterIDs[i]), 20)
	})

	c = reloadComment(app, comment.ID)
	voteCount := countVoteRows(app, comment.ID)

	if voteCount != 0 {
		t.Errorf("vote rows=%d, want 0 after concurrent removal", voteCount)
	}
	if c.Upvotes != 0 {
		t.Errorf("upvotes=%d, want 0 after concurrent removal", c.Upvotes)
	}
	if c.Downvotes != 0 {
		t.Errorf("downvotes=%d, want 0 after concurrent removal", c.Downvotes)
	}
}

// ===== A5: REMOVE COMMENT VOTE — IDEMPOTENT UNDER CONTENTION =====
// Multiple goroutines all try to remove the same user's vote concurrently.
// Invariant: all succeed (200), vote ends up removed, no panics/errors.

func TestConcurrency_RemoveCommentVote_IdempotentUnderContention(t *testing.T) {
	app := testApp(t)
	const numAttempts = 10

	uid := seedUser(t, app.DB, "cv_idem_rem")
	noteID := seedApprovedNote(t, app.DB, uid)

	comment := models.Comment{NoteID: noteID, UserID: uid, Body: "idem remove"}
	app.DB.Create(&comment)

	// Setup: user upvotes
	voteURL := fmt.Sprintf("/comments/%d/vote", comment.ID)
	w := serve("POST", "/comments/:comment_id/vote", voteURL,
		jsonBody(map[string]int8{"value": 1}), app.VoteComment, authMW(uid))
	if w.Code != http.StatusOK {
		t.Fatalf("setup vote: status=%d", w.Code)
	}

	// Concurrently remove the same vote many times
	var failures atomic.Int32
	fanOut(numAttempts, func(i int) {
		w := serveWithRetry("DELETE", "/comments/:comment_id/vote", voteURL, nil,
			app.RemoveCommentVote, authMW(uid), 20)
		if w.Code != http.StatusOK {
			failures.Add(1)
		}
	})

	if f := failures.Load(); f > 0 {
		t.Errorf("%d/%d idempotent removals returned non-200", f, numAttempts)
	}

	// Final state: 0 votes
	c := reloadComment(app, comment.ID)
	voteCount := countVoteRows(app, comment.ID)

	if voteCount != 0 {
		t.Errorf("vote rows=%d, want 0", voteCount)
	}
	if c.Upvotes != 0 || c.Downvotes != 0 {
		t.Errorf("counters: up=%d down=%d, want 0,0", c.Upvotes, c.Downvotes)
	}
}

// ===== A6: NOTE VOTE — MULTI-USER UPVOTE CONTENTION =====
// N distinct users all upvote the same note simultaneously.
// Invariant: upvotes == N, exactly N vote rows.

func TestConcurrency_NoteVote_MultiUserUpvote(t *testing.T) {
	app := testApp(t)
	const numVoters = 20

	creator := seedUser(t, app.DB, "nv_creator")
	noteID := seedApprovedNote(t, app.DB, creator)

	voterIDs := make([]uint64, numVoters)
	for i := 0; i < numVoters; i++ {
		voterIDs[i] = seedUser(t, app.DB, fmt.Sprintf("nv_voter_%d", i))
	}

	url := fmt.Sprintf("/notes/%d/upvote", noteID)
	var failures atomic.Int32

	fanOut(numVoters, func(i int) {
		w := serveWithRetry("POST", "/notes/:id/upvote", url, nil,
			app.Upvote, authMW(voterIDs[i]), 20)
		if w.Code != http.StatusOK {
			failures.Add(1)
		}
	})

	if f := failures.Load(); f > 0 {
		t.Fatalf("%d/%d concurrent note upvotes failed", f, numVoters)
	}

	n := reloadNote(app, noteID)
	voteCount := countNoteVoteRows(app, noteID)

	if voteCount != int64(numVoters) {
		t.Errorf("vote rows=%d, want %d", voteCount, numVoters)
	}
	if n.Upvotes != uint64(numVoters) {
		t.Errorf("upvotes=%d, want %d", n.Upvotes, numVoters)
	}
	if n.Downvotes != 0 {
		t.Errorf("downvotes=%d, want 0", n.Downvotes)
	}
}

// ===== A7: NOTE VOTE — SAME-USER TOGGLE CONTENTION =====
// Single user rapidly toggles upvote on a note N times concurrently.
// Invariant: final state is consistent (0 or 1 vote row, counters match).

func TestConcurrency_NoteVote_SameUserToggle(t *testing.T) {
	app := testApp(t)
	const numToggles = 15

	uid := seedUser(t, app.DB, "nv_toggler")
	noteID := seedApprovedNote(t, app.DB, uid)

	url := fmt.Sprintf("/notes/%d/upvote", noteID)

	fanOut(numToggles, func(i int) {
		serveWithRetry("POST", "/notes/:id/upvote", url, nil,
			app.Upvote, authMW(uid), 20)
	})

	n := reloadNote(app, noteID)
	voteCount := countNoteVoteRows(app, noteID)

	if voteCount != 0 && voteCount != 1 {
		t.Fatalf("vote rows=%d, want 0 or 1 after toggle contention", voteCount)
	}

	// upvotes must match vote rows
	if uint64(voteCount) != n.Upvotes {
		t.Errorf("upvotes=%d but vote rows=%d — inconsistent", n.Upvotes, voteCount)
	}
	if n.Downvotes != 0 {
		t.Errorf("downvotes=%d, want 0", n.Downvotes)
	}
}

// ===== A8: NOTE VOTE — MIXED UPVOTE/DOWNVOTE CONTENTION =====

func TestConcurrency_NoteVote_MixedDirections(t *testing.T) {
	app := testApp(t)
	const numVoters = 20

	creator := seedUser(t, app.DB, "nv_mix_creator")
	noteID := seedApprovedNote(t, app.DB, creator)

	voterIDs := make([]uint64, numVoters)
	for i := 0; i < numVoters; i++ {
		voterIDs[i] = seedUser(t, app.DB, fmt.Sprintf("nv_mix_%d", i))
	}

	fanOut(numVoters, func(i int) {
		if i%2 == 0 {
			serveWithRetry("POST", "/notes/:id/upvote", fmt.Sprintf("/notes/%d/upvote", noteID),
				nil, app.Upvote, authMW(voterIDs[i]), 20)
		} else {
			serveWithRetry("POST", "/notes/:id/downvote", fmt.Sprintf("/notes/%d/downvote", noteID),
				nil, app.Downvote, authMW(voterIDs[i]), 20)
		}
	})

	n := reloadNote(app, noteID)
	voteCount := countNoteVoteRows(app, noteID)

	if voteCount != int64(numVoters) {
		t.Errorf("vote rows=%d, want %d", voteCount, numVoters)
	}

	totalVotes := n.Upvotes + n.Downvotes
	if totalVotes != uint64(numVoters) {
		t.Errorf("upvotes(%d) + downvotes(%d) = %d, want %d", n.Upvotes, n.Downvotes, totalVotes, numVoters)
	}

	expectedUp := uint64(numVoters / 2)
	expectedDown := uint64(numVoters / 2)
	if n.Upvotes != expectedUp || n.Downvotes != expectedDown {
		t.Errorf("expected up=%d down=%d, got up=%d down=%d", expectedUp, expectedDown, n.Upvotes, n.Downvotes)
	}
}

// ===== A9: NOTE VOTE — SWITCH DIRECTION UNDER CONTENTION =====
// All users first upvote, then concurrently switch to downvote.
// Invariant: upvotes == 0, downvotes == N.

func TestConcurrency_NoteVote_SwitchDirection(t *testing.T) {
	app := testApp(t)
	const numVoters = 15

	creator := seedUser(t, app.DB, "nv_switch_creator")
	noteID := seedApprovedNote(t, app.DB, creator)

	voterIDs := make([]uint64, numVoters)
	for i := 0; i < numVoters; i++ {
		voterIDs[i] = seedUser(t, app.DB, fmt.Sprintf("nv_switch_%d", i))
	}

	// Phase 1: all upvote sequentially
	for i := 0; i < numVoters; i++ {
		w := serve("POST", "/notes/:id/upvote", fmt.Sprintf("/notes/%d/upvote", noteID),
			nil, app.Upvote, authMW(voterIDs[i]))
		if w.Code != http.StatusOK {
			t.Fatalf("setup upvote %d: status=%d", i, w.Code)
		}
	}

	// Verify setup
	n := reloadNote(app, noteID)
	if n.Upvotes != uint64(numVoters) {
		t.Fatalf("setup: upvotes=%d, want %d", n.Upvotes, numVoters)
	}

	// Phase 2: all switch to downvote concurrently
	fanOut(numVoters, func(i int) {
		serveWithRetry("POST", "/notes/:id/downvote", fmt.Sprintf("/notes/%d/downvote", noteID),
			nil, app.Downvote, authMW(voterIDs[i]), 20)
	})

	n = reloadNote(app, noteID)
	voteCount := countNoteVoteRows(app, noteID)

	if voteCount != int64(numVoters) {
		t.Errorf("vote rows=%d, want %d after switch", voteCount, numVoters)
	}
	if n.Upvotes != 0 {
		t.Errorf("upvotes=%d, want 0 after all switched to down", n.Upvotes)
	}
	if n.Downvotes != uint64(numVoters) {
		t.Errorf("downvotes=%d, want %d after all switched to down", n.Downvotes, numVoters)
	}
}

// ===== A10: COMMENT VOTE — VOTE THEN SWITCH UNDER CONTENTION =====
// All users upvote sequentially, then concurrently switch to downvote.
// Invariant: upvotes==0, downvotes==N, score matches.

func TestConcurrency_CommentVote_SwitchDirection(t *testing.T) {
	app := testApp(t)
	const numVoters = 15

	creator := seedUser(t, app.DB, "cv_switch_creator")
	noteID := seedApprovedNote(t, app.DB, creator)

	comment := models.Comment{NoteID: noteID, UserID: creator, Body: "switch target"}
	app.DB.Create(&comment)

	voterIDs := make([]uint64, numVoters)
	for i := 0; i < numVoters; i++ {
		voterIDs[i] = seedUser(t, app.DB, fmt.Sprintf("cv_switch_%d", i))
	}

	voteURL := fmt.Sprintf("/comments/%d/vote", comment.ID)

	// Phase 1: all upvote sequentially
	for i := 0; i < numVoters; i++ {
		w := serve("POST", "/comments/:comment_id/vote", voteURL,
			jsonBody(map[string]int8{"value": 1}), app.VoteComment, authMW(voterIDs[i]))
		if w.Code != http.StatusOK {
			t.Fatalf("setup upvote %d: status=%d", i, w.Code)
		}
	}

	// Setup check
	c := reloadComment(app, comment.ID)
	if c.Upvotes != int64(numVoters) {
		t.Fatalf("setup: upvotes=%d, want %d", c.Upvotes, numVoters)
	}

	// Phase 2: all switch to downvote concurrently
	fanOut(numVoters, func(i int) {
		serveWithRetry("POST", "/comments/:comment_id/vote", voteURL,
			jsonBody(map[string]int8{"value": -1}), app.VoteComment, authMW(voterIDs[i]), 20)
	})

	c = reloadComment(app, comment.ID)
	voteCount := countVoteRows(app, comment.ID)

	if voteCount != int64(numVoters) {
		t.Errorf("vote rows=%d, want %d after switch", voteCount, numVoters)
	}
	if c.Upvotes != 0 {
		t.Errorf("upvotes=%d, want 0 after all switched to down", c.Upvotes)
	}
	if c.Downvotes != int64(numVoters) {
		t.Errorf("downvotes=%d, want %d", c.Downvotes, numVoters)
	}

	expectedScore := models.WilsonScore(0, int64(numVoters))
	if c.Score != expectedScore {
		t.Errorf("score=%f, want WilsonScore(0, %d)=%f", c.Score, numVoters, expectedScore)
	}
}

// ===== A11: CONCURRENT COMMENT CREATION =====
// N users create comments on the same note simultaneously.
// Invariant: all succeed, no duplicates, paths are valid.

func TestConcurrency_CommentCreation_MultiUser(t *testing.T) {
	app := testApp(t)
	const numWriters = 15

	creator := seedUser(t, app.DB, "cc_creator")
	noteID := seedApprovedNote(t, app.DB, creator)

	writerIDs := make([]uint64, numWriters)
	for i := 0; i < numWriters; i++ {
		writerIDs[i] = seedUser(t, app.DB, fmt.Sprintf("cc_writer_%d", i))
	}

	var failures atomic.Int32
	fanOut(numWriters, func(i int) {
		w := serveWithRetry("POST", "/notes/:id/comments",
			fmt.Sprintf("/notes/%d/comments", noteID),
			jsonBody(map[string]string{"body": fmt.Sprintf("concurrent comment %d", i)}),
			app.CreateComment, authMW(writerIDs[i]), 20)
		if w.Code != http.StatusCreated {
			failures.Add(1)
		}
	})

	if f := failures.Load(); f > 0 {
		t.Errorf("%d/%d concurrent comment creations failed", f, numWriters)
	}

	var count int64
	app.DB.Model(&models.Comment{}).Where("note_id = ?", noteID).Count(&count)
	if count != int64(numWriters) {
		t.Errorf("comment count=%d, want %d", count, numWriters)
	}

	// Verify all paths are unique and well-formed
	var comments []models.Comment
	app.DB.Where("note_id = ?", noteID).Find(&comments)

	paths := make(map[string]bool)
	for _, c := range comments {
		if c.Path == "" {
			t.Errorf("comment %d has empty path", c.ID)
			continue
		}
		if paths[c.Path] {
			t.Errorf("duplicate path %q", c.Path)
		}
		paths[c.Path] = true

		expected := fmt.Sprintf("/%d/", c.ID)
		if c.ParentID == nil && c.Path != expected {
			t.Errorf("top-level comment %d: path=%q, want %q", c.ID, c.Path, expected)
		}
	}
}

// ===== A12: CONCURRENT REPLIES TO SAME PARENT =====
// N users all reply to the same parent comment simultaneously.
// Invariant: all succeed, parent has N children with correct depth/path.

func TestConcurrency_CommentReply_SameParent(t *testing.T) {
	app := testApp(t)
	const numReplies = 15

	creator := seedUser(t, app.DB, "cr_creator")
	noteID := seedApprovedNote(t, app.DB, creator)

	// Create parent comment
	w := serve("POST", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments", noteID),
		jsonBody(map[string]string{"body": "parent"}), app.CreateComment, authMW(creator))
	assertStatus(t, w, http.StatusCreated)
	r := respJSON(t, w)
	parentID := uint(r["id"].(float64))

	replierIDs := make([]uint64, numReplies)
	for i := 0; i < numReplies; i++ {
		replierIDs[i] = seedUser(t, app.DB, fmt.Sprintf("cr_replier_%d", i))
	}

	var failures atomic.Int32
	fanOut(numReplies, func(i int) {
		w := serveWithRetry("POST", "/notes/:id/comments",
			fmt.Sprintf("/notes/%d/comments", noteID),
			jsonBody(map[string]interface{}{
				"body":      fmt.Sprintf("reply %d", i),
				"parent_id": parentID,
			}), app.CreateComment, authMW(replierIDs[i]), 20)
		if w.Code != http.StatusCreated {
			failures.Add(1)
		}
	})

	if f := failures.Load(); f > 0 {
		t.Errorf("%d/%d concurrent replies failed", f, numReplies)
	}

	// Verify children
	var children []models.Comment
	app.DB.Where("note_id = ? AND parent_id = ?", noteID, parentID).Find(&children)
	if len(children) != numReplies {
		t.Errorf("child count=%d, want %d", len(children), numReplies)
	}

	for _, ch := range children {
		if ch.Depth != 1 {
			t.Errorf("child %d: depth=%d, want 1", ch.ID, ch.Depth)
		}
		expectedPrefix := fmt.Sprintf("/%d/", parentID)
		if ch.Path == "" || ch.Path[:len(expectedPrefix)] != expectedPrefix {
			t.Errorf("child %d: path=%q, should start with %q", ch.ID, ch.Path, expectedPrefix)
		}
	}
}
