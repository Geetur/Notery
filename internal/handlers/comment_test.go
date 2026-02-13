package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Geetur/Notery/internal/models"
)

// ===== TEST INFRASTRUCTURE =====

func init() { gin.SetMode(gin.TestMode) }

// testApp creates an App backed by an in-memory SQLite database with all tables migrated.
func testApp(t *testing.T) *App {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Note{},
		&models.Subnotery{},
		&models.Comment{},
		&models.CommentVote{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Create the join table for subnotery admins (user_admins)
	db.Exec("CREATE TABLE IF NOT EXISTS user_admins (user_id INTEGER, subnotery_id INTEGER)")
	return &App{DB: db}
}

var seedCounter int

func seedUser(t *testing.T, db *gorm.DB, username string) uint64 {
	t.Helper()
	user := models.User{Email: username + "@test.com", Username: username}
	if err := user.SetPassword("test123"); err != nil {
		t.Fatalf("hash pw: %v", err)
	}
	db.Create(&user)
	return uint64(user.ID)
}

func seedApprovedNote(t *testing.T, db *gorm.DB, creatorID uint64) uint {
	t.Helper()
	sub := models.Subnotery{Name: fmt.Sprintf("sub-%d-%d", creatorID, seedCounter)}
	seedCounter++
	db.Create(&sub)
	note := models.Note{
		Title: "Test Note", Status: models.StatusApproved,
		SubnoteryID: sub.ID, CreatorID: creatorID, HasPDF: true,
	}
	db.Create(&note)
	return note.ID
}

func seedPendingNote(t *testing.T, db *gorm.DB, creatorID uint64) uint {
	t.Helper()
	sub := models.Subnotery{Name: fmt.Sprintf("psub-%d-%d", creatorID, seedCounter)}
	seedCounter++
	db.Create(&sub)
	note := models.Note{
		Title: "Pending Note", Status: models.StatusPending,
		SubnoteryID: sub.ID, CreatorID: creatorID,
	}
	db.Create(&note)
	return note.ID
}

func authMW(userID uint64) gin.HandlerFunc {
	return func(c *gin.Context) { c.Set("user_id", userID) }
}

func adminMW(userID uint64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Set("admin_type", true)
	}
}

func jsonBody(v interface{}) *bytes.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

func respJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode response: %v | body: %s", err, w.Body.String())
	}
	return m
}

// serve registers a handler on a gin engine and fires a single request.
// routePattern uses gin params (e.g. "/comments/:comment_id/vote").
// url is the actual request path (e.g. "/comments/42/vote").
func serve(method, routePattern, url string, body *bytes.Reader, handler gin.HandlerFunc, mw ...gin.HandlerFunc) *httptest.ResponseRecorder {
	r := gin.New()
	g := r.Group("")
	for _, m := range mw {
		g.Use(m)
	}
	g.Handle(method, routePattern, handler)

	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, url, body)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ===== ASSERTION HELPERS =====

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("expected status %d, got %d: %s", want, w.Code, w.Body.String())
	}
}

func assertFloat(t *testing.T, label string, m map[string]interface{}, key string, want float64) {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("%s: key %q missing from %v", label, key, m)
	}
	got, ok := v.(float64)
	if !ok {
		t.Fatalf("%s: key %q is %T not float64 in %v", label, key, v, m)
	}
	if got != want {
		t.Fatalf("%s: %s=%v, want %v", label, key, got, want)
	}
}

// ===== VOTE TOGGLE / SWITCH IDEMPOTENCY =====

func TestVoteComment_ToggleOff(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "voter")
	noteID := seedApprovedNote(t, app.DB, uid)

	c := models.Comment{NoteID: noteID, UserID: uid, Body: "hello"}
	app.DB.Create(&c)
	url := fmt.Sprintf("/comments/%d/vote", c.ID)

	vote := func(val int8) map[string]interface{} {
		w := serve("POST", "/comments/:comment_id/vote", url, jsonBody(map[string]int8{"value": val}), app.VoteComment, authMW(uid))
		if w.Code != http.StatusOK {
			t.Fatalf("vote %d: expected 200, got %d: %s", val, w.Code, w.Body.String())
		}
		return respJSON(t, w)
	}

	// Upvote
	r1 := vote(1)
	assertFloat(t, "user_vote=1", r1, "user_vote", 1)
	assertFloat(t, "upvotes=1", r1, "upvotes", 1)

	// Toggle off (same value)
	r2 := vote(1)
	assertFloat(t, "user_vote=0 toggle", r2, "user_vote", 0)
	assertFloat(t, "upvotes=0 toggle", r2, "upvotes", 0)

	// Repeat to prove idempotency
	vote(1) // upvote again
	r3 := vote(1)
	assertFloat(t, "user_vote=0 second toggle", r3, "user_vote", 0)
}

func TestVoteComment_SwitchDirection(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "switcher")
	noteID := seedApprovedNote(t, app.DB, uid)

	c := models.Comment{NoteID: noteID, UserID: uid, Body: "switchable"}
	app.DB.Create(&c)
	url := fmt.Sprintf("/comments/%d/vote", c.ID)

	vote := func(val int8) map[string]interface{} {
		w := serve("POST", "/comments/:comment_id/vote", url, jsonBody(map[string]int8{"value": val}), app.VoteComment, authMW(uid))
		if w.Code != http.StatusOK {
			t.Fatalf("vote %d: status %d: %s", val, w.Code, w.Body.String())
		}
		return respJSON(t, w)
	}

	// up → down (switch)
	vote(1)
	r := vote(-1)
	assertFloat(t, "switch to -1", r, "user_vote", -1)
	assertFloat(t, "upvotes=0", r, "upvotes", 0)
	assertFloat(t, "downvotes=1", r, "downvotes", 1)

	// down → up (switch back)
	r2 := vote(1)
	assertFloat(t, "switch to +1", r2, "user_vote", 1)
	assertFloat(t, "upvotes=1", r2, "upvotes", 1)
	assertFloat(t, "downvotes=0", r2, "downvotes", 0)
}

func TestVoteComment_InvalidValue(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "badvoter")
	noteID := seedApprovedNote(t, app.DB, uid)

	c := models.Comment{NoteID: noteID, UserID: uid, Body: "test"}
	app.DB.Create(&c)

	w := serve("POST", "/comments/:comment_id/vote",
		fmt.Sprintf("/comments/%d/vote", c.ID),
		jsonBody(map[string]int8{"value": 2}), app.VoteComment, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestVoteComment_OnDeletedComment(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "delvoter")
	noteID := seedApprovedNote(t, app.DB, uid)

	c := models.Comment{NoteID: noteID, UserID: uid, Body: "soon deleted", IsDeleted: true}
	app.DB.Create(&c)

	w := serve("POST", "/comments/:comment_id/vote",
		fmt.Sprintf("/comments/%d/vote", c.ID),
		jsonBody(map[string]int8{"value": 1}), app.VoteComment, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestRemoveCommentVote_Idempotent(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "unvoter")
	noteID := seedApprovedNote(t, app.DB, uid)

	c := models.Comment{NoteID: noteID, UserID: uid, Body: "hello"}
	app.DB.Create(&c)
	voteURL := fmt.Sprintf("/comments/%d/vote", c.ID)

	remove := func() *httptest.ResponseRecorder {
		return serve("DELETE", "/comments/:comment_id/vote", voteURL, nil, app.RemoveCommentVote, authMW(uid))
	}

	// Remove when no vote — idempotent
	assertStatus(t, remove(), http.StatusOK)

	// Vote then remove
	serve("POST", "/comments/:comment_id/vote", voteURL, jsonBody(map[string]int8{"value": 1}), app.VoteComment, authMW(uid))
	assertStatus(t, remove(), http.StatusOK)

	// Double-remove
	assertStatus(t, remove(), http.StatusOK)
}

// ===== DEPTH CAP ENFORCEMENT =====

func TestCreateComment_DepthCapEnforced(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "deepwriter")
	noteID := seedApprovedNote(t, app.DB, uid)

	// Build a comment chain up to MaxWriteDepth.
	var parentID *uint
	for d := 0; d <= models.MaxWriteDepth; d++ {
		body := map[string]interface{}{"body": fmt.Sprintf("depth %d", d)}
		if parentID != nil {
			body["parent_id"] = *parentID
		}
		w := serve("POST", "/notes/:id/comments",
			fmt.Sprintf("/notes/%d/comments", noteID),
			jsonBody(body), app.CreateComment, authMW(uid))
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 at depth %d, got %d: %s", d, w.Code, w.Body.String())
		}
		resp := respJSON(t, w)
		id := uint(resp["id"].(float64))
		parentID = &id
	}

	// The next reply would be at depth MaxWriteDepth+1 — should fail
	w := serve("POST", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments", noteID),
		jsonBody(map[string]interface{}{"body": "too deep", "parent_id": *parentID}),
		app.CreateComment, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
	resp := respJSON(t, w)
	if _, ok := resp["max_depth"]; !ok {
		t.Fatal("expected max_depth in error response")
	}
}

// ===== SOFT-DELETE BEHAVIOR =====

func TestDeleteComment_SoftDelete_PreservesTree(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "deleter")
	noteID := seedApprovedNote(t, app.DB, uid)

	parent := models.Comment{NoteID: noteID, UserID: uid, Body: "parent"}
	app.DB.Create(&parent)
	pid := parent.ID
	child := models.Comment{NoteID: noteID, UserID: uid, Body: "child", ParentID: &pid, Depth: 1}
	app.DB.Create(&child)

	w := serve("DELETE", "/comments/:comment_id",
		fmt.Sprintf("/comments/%d", parent.ID), nil, app.DeleteComment, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Row still exists, body cleared, flag set
	var del models.Comment
	app.DB.First(&del, parent.ID)
	if !del.IsDeleted {
		t.Fatal("expected IsDeleted=true")
	}
	if del.Body != "" {
		t.Fatalf("expected cleared body, got %q", del.Body)
	}

	// Child intact
	var ch models.Comment
	app.DB.First(&ch, child.ID)
	if ch.IsDeleted {
		t.Fatal("child must not be deleted")
	}
	if ch.Body != "child" {
		t.Fatal("child body must be unchanged")
	}
}

func TestDeleteComment_AlreadyDeleted_Idempotent(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "redeleter")
	noteID := seedApprovedNote(t, app.DB, uid)

	c := models.Comment{NoteID: noteID, UserID: uid, Body: "x", IsDeleted: true}
	app.DB.Create(&c)

	w := serve("DELETE", "/comments/:comment_id",
		fmt.Sprintf("/comments/%d", c.ID), nil, app.DeleteComment, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["message"] != "Comment already deleted" {
		t.Fatalf("unexpected: %v", r["message"])
	}
}

func TestDeleteComment_DisplaysDeletedInTree(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "treechecker")

	c := models.Comment{UserID: uid, Body: "", IsDeleted: true}
	userMap := map[uint64]string{uid: "treechecker"}
	responseMap := buildResponseMap([]models.Comment{c}, userMap, nil)
	if resp := responseMap[c.ID]; resp.Body != "[deleted]" || resp.Username != "[deleted]" {
		t.Fatalf("expected [deleted] in body and username, got body=%q user=%q", resp.Body, resp.Username)
	}
}

// ===== PERMISSION CHECKS =====

func TestDeleteComment_OwnerCanDelete(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "owner")
	noteID := seedApprovedNote(t, app.DB, uid)

	c := models.Comment{NoteID: noteID, UserID: uid, Body: "mine"}
	app.DB.Create(&c)

	w := serve("DELETE", "/comments/:comment_id",
		fmt.Sprintf("/comments/%d", c.ID), nil, app.DeleteComment, authMW(uid))
	assertStatus(t, w, http.StatusOK)
}

func TestDeleteComment_NonOwnerDenied(t *testing.T) {
	app := testApp(t)
	owner := seedUser(t, app.DB, "commauthor")
	other := seedUser(t, app.DB, "stranger")
	noteID := seedApprovedNote(t, app.DB, owner)

	c := models.Comment{NoteID: noteID, UserID: owner, Body: "not yours"}
	app.DB.Create(&c)

	w := serve("DELETE", "/comments/:comment_id",
		fmt.Sprintf("/comments/%d", c.ID), nil, app.DeleteComment, authMW(other))
	assertStatus(t, w, http.StatusForbidden)
}

func TestDeleteComment_GlobalAdminCanDelete(t *testing.T) {
	app := testApp(t)
	owner := seedUser(t, app.DB, "commentor")
	admin := seedUser(t, app.DB, "globaladm")
	app.DB.Model(&models.User{}).Where("id = ?", admin).Update("is_global_admin", true)

	noteID := seedApprovedNote(t, app.DB, owner)
	c := models.Comment{NoteID: noteID, UserID: owner, Body: "admin will nuke"}
	app.DB.Create(&c)

	w := serve("DELETE", "/comments/:comment_id",
		fmt.Sprintf("/comments/%d", c.ID), nil, app.DeleteComment, adminMW(admin))
	assertStatus(t, w, http.StatusOK)
}

func TestDeleteComment_GlobalAdminCanDeleteWithoutAdminContext(t *testing.T) {
	app := testApp(t)
	owner := seedUser(t, app.DB, "commentor2")
	admin := seedUser(t, app.DB, "globaladm2")
	app.DB.Model(&models.User{}).Where("id = ?", admin).Update("is_global_admin", true)

	noteID := seedApprovedNote(t, app.DB, owner)
	c := models.Comment{NoteID: noteID, UserID: owner, Body: "admin from DB lookup"}
	app.DB.Create(&c)

	w := serve("DELETE", "/comments/:comment_id",
		fmt.Sprintf("/comments/%d", c.ID), nil, app.DeleteComment, authMW(admin))
	assertStatus(t, w, http.StatusOK)
}

func TestDeleteComment_SubnoteryAdminCanDelete(t *testing.T) {
	app := testApp(t)
	owner := seedUser(t, app.DB, "poster")
	subAdmin := seedUser(t, app.DB, "subadm")

	sub := models.Subnotery{Name: "admin-test-sub"}
	app.DB.Create(&sub)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", subAdmin, sub.ID)

	note := models.Note{Title: "SA Test", Status: models.StatusApproved, SubnoteryID: sub.ID, CreatorID: owner, HasPDF: true}
	app.DB.Create(&note)
	c := models.Comment{NoteID: note.ID, UserID: owner, Body: "subadmin scope test"}
	app.DB.Create(&c)

	w := serve("DELETE", "/comments/:comment_id",
		fmt.Sprintf("/comments/%d", c.ID), nil, app.DeleteComment, authMW(subAdmin))
	assertStatus(t, w, http.StatusOK)
}

func TestDeleteComment_WrongSubnoteryAdminDenied(t *testing.T) {
	app := testApp(t)
	owner := seedUser(t, app.DB, "writer")
	wrongAdmin := seedUser(t, app.DB, "wrongadm")

	sub1 := models.Subnotery{Name: "sub-one"}
	app.DB.Create(&sub1)
	sub2 := models.Subnotery{Name: "sub-two"}
	app.DB.Create(&sub2)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", wrongAdmin, sub2.ID)

	note := models.Note{Title: "Cross-Sub", Status: models.StatusApproved, SubnoteryID: sub1.ID, CreatorID: owner, HasPDF: true}
	app.DB.Create(&note)
	c := models.Comment{NoteID: note.ID, UserID: owner, Body: "protected"}
	app.DB.Create(&c)

	w := serve("DELETE", "/comments/:comment_id",
		fmt.Sprintf("/comments/%d", c.ID), nil, app.DeleteComment, authMW(wrongAdmin))
	assertStatus(t, w, http.StatusForbidden)
}

// ===== EDIT PERMISSIONS =====

func TestEditComment_OwnerCanEdit(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "editor")
	noteID := seedApprovedNote(t, app.DB, uid)

	c := models.Comment{NoteID: noteID, UserID: uid, Body: "original"}
	app.DB.Create(&c)

	w := serve("PUT", "/comments/:comment_id",
		fmt.Sprintf("/comments/%d", c.ID),
		jsonBody(map[string]string{"body": "edited"}), app.EditComment, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["body"] != "edited" {
		t.Fatalf("body=%v, want 'edited'", r["body"])
	}
}

func TestEditComment_NonOwnerDenied(t *testing.T) {
	app := testApp(t)
	owner := seedUser(t, app.DB, "realowner")
	other := seedUser(t, app.DB, "intruder")
	noteID := seedApprovedNote(t, app.DB, owner)

	c := models.Comment{NoteID: noteID, UserID: owner, Body: "mine"}
	app.DB.Create(&c)

	w := serve("PUT", "/comments/:comment_id",
		fmt.Sprintf("/comments/%d", c.ID),
		jsonBody(map[string]string{"body": "hacked"}), app.EditComment, authMW(other))
	assertStatus(t, w, http.StatusForbidden)
}

func TestEditComment_DeletedCannotEdit(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "deleditor")
	noteID := seedApprovedNote(t, app.DB, uid)

	c := models.Comment{NoteID: noteID, UserID: uid, Body: "", IsDeleted: true}
	app.DB.Create(&c)

	w := serve("PUT", "/comments/:comment_id",
		fmt.Sprintf("/comments/%d", c.ID),
		jsonBody(map[string]string{"body": "revive"}), app.EditComment, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

// ===== INPUT NORMALIZATION =====

func TestCreateComment_WhitespaceOnlyRejected(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "spacer")
	noteID := seedApprovedNote(t, app.DB, uid)

	for _, body := range []string{"   ", "\t\n", "  \n  ", "\t"} {
		w := serve("POST", "/notes/:id/comments",
			fmt.Sprintf("/notes/%d/comments", noteID),
			jsonBody(map[string]string{"body": body}), app.CreateComment, authMW(uid))
		assertStatus(t, w, http.StatusBadRequest)
	}
}

func TestCreateComment_TrimmedWhitespace(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "trimmer")
	noteID := seedApprovedNote(t, app.DB, uid)

	w := serve("POST", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments", noteID),
		jsonBody(map[string]string{"body": "  hello world  "}), app.CreateComment, authMW(uid))
	assertStatus(t, w, http.StatusCreated)
	r := respJSON(t, w)
	if r["body"] != "hello world" {
		t.Fatalf("body=%q, want 'hello world'", r["body"])
	}
}

func TestEditComment_WhitespaceOnlyRejected(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "editspacer")
	noteID := seedApprovedNote(t, app.DB, uid)

	c := models.Comment{NoteID: noteID, UserID: uid, Body: "original"}
	app.DB.Create(&c)

	w := serve("PUT", "/comments/:comment_id",
		fmt.Sprintf("/comments/%d", c.ID),
		jsonBody(map[string]string{"body": "   "}), app.EditComment, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

// ===== COMMENT ON PENDING NOTE =====

func TestCreateComment_PendingNoteBlocked(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "penduser")
	noteID := seedPendingNote(t, app.DB, uid)

	w := serve("POST", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments", noteID),
		jsonBody(map[string]string{"body": "should fail"}), app.CreateComment, authMW(uid))
	assertStatus(t, w, http.StatusForbidden)
}

// ===== REPLY TO DELETED COMMENT BLOCKED =====

func TestCreateComment_ReplyToDeletedBlocked(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "replydel")
	noteID := seedApprovedNote(t, app.DB, uid)

	parent := models.Comment{NoteID: noteID, UserID: uid, Body: "", IsDeleted: true}
	app.DB.Create(&parent)

	w := serve("POST", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments", noteID),
		jsonBody(map[string]interface{}{"body": "reply", "parent_id": parent.ID}),
		app.CreateComment, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

// ===== PUBLIC READ =====

func TestGetNoteComments_PublicAnonymous(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "anon")
	noteID := seedApprovedNote(t, app.DB, uid)

	app.DB.Create(&models.Comment{NoteID: noteID, UserID: uid, Body: "public"})

	w := serve("GET", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments", noteID), nil, app.GetNoteComments)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	assertFloat(t, "total=1", r, "total", 1)
}

func TestGetNoteComments_PendingNoteDenied(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "pendreader")
	noteID := seedPendingNote(t, app.DB, uid)

	w := serve("GET", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments", noteID), nil, app.GetNoteComments)
	assertStatus(t, w, http.StatusForbidden)
}

// ===== WILSON SCORE RECALCULATION =====

func TestVoteComment_WilsonScoreRecalculated(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "scorer")
	noteID := seedApprovedNote(t, app.DB, uid)

	c := models.Comment{NoteID: noteID, UserID: uid, Body: "score me"}
	app.DB.Create(&c)

	w := serve("POST", "/comments/:comment_id/vote",
		fmt.Sprintf("/comments/%d/vote", c.ID),
		jsonBody(map[string]int8{"value": 1}), app.VoteComment, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	score := r["score"].(float64)
	expected := models.WilsonScore(1, 0)
	if score != expected {
		t.Fatalf("score=%f, want WilsonScore(1,0)=%f", score, expected)
	}
}

// ===== CROSS-NOTE PARENT REJECTED =====

func TestCreateComment_CrossNoteParentRejected(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "crossnote")
	noteA := seedApprovedNote(t, app.DB, uid)
	noteB := seedApprovedNote(t, app.DB, uid)

	parent := models.Comment{NoteID: noteA, UserID: uid, Body: "on note A"}
	app.DB.Create(&parent)

	w := serve("POST", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments", noteB),
		jsonBody(map[string]interface{}{"body": "cross-note", "parent_id": parent.ID}),
		app.CreateComment, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

// ===== MATERIALIZED PATH =====

func TestCreateComment_PathSetCorrectly(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "pathtester")
	noteID := seedApprovedNote(t, app.DB, uid)

	// Create top-level comment via handler
	w := serve("POST", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments", noteID),
		jsonBody(map[string]string{"body": "root"}), app.CreateComment, authMW(uid))
	assertStatus(t, w, http.StatusCreated)
	r := respJSON(t, w)
	rootID := uint(r["id"].(float64))

	// Check DB path for top-level
	var root models.Comment
	app.DB.First(&root, rootID)
	expectedPath := fmt.Sprintf("/%d/", rootID)
	if root.Path != expectedPath {
		t.Fatalf("root path=%q, want %q", root.Path, expectedPath)
	}

	// Create reply via handler
	w2 := serve("POST", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments", noteID),
		jsonBody(map[string]interface{}{"body": "reply", "parent_id": rootID}),
		app.CreateComment, authMW(uid))
	assertStatus(t, w2, http.StatusCreated)
	r2 := respJSON(t, w2)
	replyID := uint(r2["id"].(float64))

	var reply models.Comment
	app.DB.First(&reply, replyID)
	expectedReplyPath := fmt.Sprintf("/%d/%d/", rootID, replyID)
	if reply.Path != expectedReplyPath {
		t.Fatalf("reply path=%q, want %q", reply.Path, expectedReplyPath)
	}

	// Create nested reply (depth 2)
	w3 := serve("POST", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments", noteID),
		jsonBody(map[string]interface{}{"body": "nested", "parent_id": replyID}),
		app.CreateComment, authMW(uid))
	assertStatus(t, w3, http.StatusCreated)
	r3 := respJSON(t, w3)
	nestedID := uint(r3["id"].(float64))

	var nested models.Comment
	app.DB.First(&nested, nestedID)
	expectedNestedPath := fmt.Sprintf("/%d/%d/%d/", rootID, replyID, nestedID)
	if nested.Path != expectedNestedPath {
		t.Fatalf("nested path=%q, want %q", nested.Path, expectedNestedPath)
	}
}

// ===== TWO-PHASE LISTING =====

func TestGetNoteComments_TwoPhaseOnlyPageRoots(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "twophase")
	noteID := seedApprovedNote(t, app.DB, uid)

	// Create 3 top-level comments via handler so paths are set
	var rootIDs []uint
	for i := 0; i < 3; i++ {
		w := serve("POST", "/notes/:id/comments",
			fmt.Sprintf("/notes/%d/comments", noteID),
			jsonBody(map[string]string{"body": fmt.Sprintf("root %d", i)}),
			app.CreateComment, authMW(uid))
		assertStatus(t, w, http.StatusCreated)
		r := respJSON(t, w)
		rootIDs = append(rootIDs, uint(r["id"].(float64)))
	}

	// Add a reply to root 0 and root 1
	for _, rid := range rootIDs[:2] {
		w := serve("POST", "/notes/:id/comments",
			fmt.Sprintf("/notes/%d/comments", noteID),
			jsonBody(map[string]interface{}{"body": "child", "parent_id": rid}),
			app.CreateComment, authMW(uid))
		assertStatus(t, w, http.StatusCreated)
	}

	// Fetch page 1, limit 2 — should see roots 0,1 and their children, NOT root 2's tree
	w := serve("GET", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments?page=1&limit=2&sort=old", noteID),
		nil, app.GetNoteComments)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)

	assertFloat(t, "total=3", r, "total", 3)

	comments := r["comments"].([]interface{})
	if len(comments) != 2 {
		t.Fatalf("expected 2 roots on page 1, got %d", len(comments))
	}

	// Each of the 2 roots should have 1 child
	for i, c := range comments {
		cm := c.(map[string]interface{})
		children := cm["children"].([]interface{})
		if len(children) != 1 {
			t.Fatalf("root %d: expected 1 child, got %d", i, len(children))
		}
	}

	// Fetch page 2 — should see root 2 with no children
	w2 := serve("GET", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments?page=2&limit=2&sort=old", noteID),
		nil, app.GetNoteComments)
	assertStatus(t, w2, http.StatusOK)
	r2 := respJSON(t, w2)
	comments2 := r2["comments"].([]interface{})
	if len(comments2) != 1 {
		t.Fatalf("expected 1 root on page 2, got %d", len(comments2))
	}
	cm2 := comments2[0].(map[string]interface{})
	children2 := cm2["children"].([]interface{})
	if len(children2) != 0 {
		t.Fatalf("root 2 should have 0 children, got %d", len(children2))
	}
}

func TestGetNoteComments_EmptyPage(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "emptypage")
	noteID := seedApprovedNote(t, app.DB, uid)

	// No comments — should return empty array
	w := serve("GET", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments", noteID), nil, app.GetNoteComments)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	assertFloat(t, "total=0", r, "total", 0)
	comments := r["comments"].([]interface{})
	if len(comments) != 0 {
		t.Fatalf("expected 0 comments, got %d", len(comments))
	}
}

func TestGetNoteComments_PartialPathDataFallsBack(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "partialpath")
	noteID := seedApprovedNote(t, app.DB, uid)

	// Root created through handler to get a materialized path.
	w := serve("POST", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments", noteID),
		jsonBody(map[string]string{"body": "root"}), app.CreateComment, authMW(uid))
	assertStatus(t, w, http.StatusCreated)
	rootResp := respJSON(t, w)
	rootID := uint(rootResp["id"].(float64))

	// Simulate legacy/unbackfilled data: child exists but path is empty.
	pid := rootID
	child := models.Comment{
		NoteID:   noteID,
		UserID:   uid,
		ParentID: &pid,
		Body:     "legacy child",
		Depth:    1,
		Path:     "",
	}
	app.DB.Create(&child)

	list := serve("GET", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments?sort=old&limit=10&page=1", noteID),
		nil, app.GetNoteComments)
	assertStatus(t, list, http.StatusOK)
	r := respJSON(t, list)

	comments := r["comments"].([]interface{})
	if len(comments) != 1 {
		t.Fatalf("expected 1 root, got %d", len(comments))
	}
	root := comments[0].(map[string]interface{})
	children := root["children"].([]interface{})
	if len(children) != 1 {
		t.Fatalf("expected 1 child with partial path fallback, got %d", len(children))
	}
}

func TestGetNoteComments_ControversialSortUsesControversyScore(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "controversial")
	noteID := seedApprovedNote(t, app.DB, uid)

	app.DB.Create(&models.Comment{
		NoteID: noteID, UserID: uid, Body: "low-controversy", Upvotes: 100, Downvotes: 0,
	})
	app.DB.Create(&models.Comment{
		NoteID: noteID, UserID: uid, Body: "mid-controversy", Upvotes: 30, Downvotes: 20,
	})
	app.DB.Create(&models.Comment{
		NoteID: noteID, UserID: uid, Body: "high-controversy", Upvotes: 10, Downvotes: 10,
	})

	w := serve("GET", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments?sort=controversial&limit=10&page=1", noteID),
		nil, app.GetNoteComments)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	comments := r["comments"].([]interface{})
	if len(comments) != 3 {
		t.Fatalf("expected 3 comments, got %d", len(comments))
	}

	got0 := comments[0].(map[string]interface{})["body"]
	got1 := comments[1].(map[string]interface{})["body"]
	got2 := comments[2].(map[string]interface{})["body"]
	if got0 != "high-controversy" || got1 != "mid-controversy" || got2 != "low-controversy" {
		t.Fatalf("unexpected controversial order: [%v, %v, %v]", got0, got1, got2)
	}
}

func TestGetNoteComments_TruncatedFalseWhenBudgetExactlyFilled(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "budgetfill")
	noteID := seedApprovedNote(t, app.DB, uid)

	root := models.Comment{NoteID: noteID, UserID: uid, Body: "root"}
	app.DB.Create(&root)

	// One root consumes one slot from MaxNodesPerRequest.
	budget := models.MaxNodesPerRequest - 1
	for i := 0; i < budget; i++ {
		pid := root.ID
		app.DB.Create(&models.Comment{
			NoteID: noteID, UserID: uid, ParentID: &pid, Body: fmt.Sprintf("child %d", i), Depth: 1,
		})
	}

	w := serve("GET", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments?sort=old&limit=10&page=1", noteID),
		nil, app.GetNoteComments)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	truncated, ok := r["truncated"].(bool)
	if !ok {
		t.Fatalf("expected boolean truncated flag, got %#v", r["truncated"])
	}
	if truncated {
		t.Fatal("expected truncated=false when descendant budget is exactly filled")
	}
}

// ===== SUBTREE FETCH (GetComment) =====

func TestGetComment_ExactSubtree(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "subtreefetch")
	noteID := seedApprovedNote(t, app.DB, uid)

	// Create a tree:
	//   root1 → child1a
	//   root2 → child2a → grandchild2a
	createComment := func(body string, parentID *uint) uint {
		payload := map[string]interface{}{"body": body}
		if parentID != nil {
			payload["parent_id"] = *parentID
		}
		w := serve("POST", "/notes/:id/comments",
			fmt.Sprintf("/notes/%d/comments", noteID),
			jsonBody(payload), app.CreateComment, authMW(uid))
		if w.Code != http.StatusCreated {
			t.Fatalf("create %q: got %d: %s", body, w.Code, w.Body.String())
		}
		r := respJSON(t, w)
		id := uint(r["id"].(float64))
		return id
	}

	root1 := createComment("root1", nil)
	createComment("child1a", &root1)

	root2 := createComment("root2", nil)
	child2a := createComment("child2a", &root2)
	createComment("grandchild2a", &child2a)

	// Fetch subtree of root2 — should include root2, child2a, grandchild2a
	// but NOT root1 or child1a
	w := serve("GET", "/comments/:comment_id",
		fmt.Sprintf("/comments/%d", root2), nil, app.GetComment)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)

	if uint(r["id"].(float64)) != root2 {
		t.Fatalf("subtree root id=%v, want %d", r["id"], root2)
	}
	children := r["children"].([]interface{})
	if len(children) != 1 {
		t.Fatalf("expected 1 child of root2, got %d", len(children))
	}
	child := children[0].(map[string]interface{})
	grandchildren := child["children"].([]interface{})
	if len(grandchildren) != 1 {
		t.Fatalf("expected 1 grandchild of child2a, got %d", len(grandchildren))
	}
}

func TestGetComment_HasMoreWhenBudgetTruncated(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "subtreecap")
	noteID := seedApprovedNote(t, app.DB, uid)

	root := models.Comment{NoteID: noteID, UserID: uid, Body: "root"}
	app.DB.Create(&root)

	// Target subtree size will be 1(root) + 500(children) = 501 > MaxNodesPerRequest(500).
	for i := 0; i < models.MaxNodesPerRequest; i++ {
		pid := root.ID
		app.DB.Create(&models.Comment{
			NoteID: noteID, UserID: uid, ParentID: &pid, Body: fmt.Sprintf("child %d", i), Depth: 1,
		})
	}

	w := serve("GET", "/comments/:comment_id",
		fmt.Sprintf("/comments/%d", root.ID), nil, app.GetComment)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)

	hasMore, ok := r["has_more_replies"].(bool)
	if !ok || !hasMore {
		t.Fatalf("expected has_more_replies=true when subtree hits node budget, got %v", r["has_more_replies"])
	}

	children := r["children"].([]interface{})
	if len(children) != models.MaxNodesPerRequest-1 {
		t.Fatalf("expected %d children after budget trim, got %d", models.MaxNodesPerRequest-1, len(children))
	}
}

// ===== FILTER HELPERS =====

func TestFilterDescendantsOfRoots(t *testing.T) {
	// Simulate: roots=[1,3], comments include 2(parent=1), 4(parent=3), 5(parent=2), 6(parent=nil)
	pid1 := uint(1)
	pid2 := uint(2)
	pid3 := uint(3)
	comments := []models.Comment{
		{ID: 1},
		{ID: 2, ParentID: &pid1},
		{ID: 3},
		{ID: 4, ParentID: &pid3},
		{ID: 5, ParentID: &pid2},
		{ID: 6},
	}

	result := filterDescendantsOfRoots(comments, []uint{1, 3})
	ids := make(map[uint]bool)
	for _, c := range result {
		ids[c.ID] = true
	}

	// Should include 1, 2, 3, 4, 5 (descendants of roots 1 and 3) but NOT 6
	for _, want := range []uint{1, 2, 3, 4, 5} {
		if !ids[want] {
			t.Fatalf("expected comment %d in result", want)
		}
	}
	if ids[6] {
		t.Fatal("comment 6 should not be included (not a descendant of any root)")
	}
}

func TestFilterExactSubtree(t *testing.T) {
	pid1 := uint(1)
	pid2 := uint(2)
	pid3 := uint(3)
	comments := []models.Comment{
		{ID: 1},
		{ID: 2, ParentID: &pid1},
		{ID: 3},
		{ID: 4, ParentID: &pid3},
		{ID: 5, ParentID: &pid2},
	}

	result := filterExactSubtree(comments, 1)
	ids := make(map[uint]bool)
	for _, c := range result {
		ids[c.ID] = true
	}

	// Subtree of 1: 1, 2, 5 (5 is child of 2 which is child of 1)
	for _, want := range []uint{1, 2, 5} {
		if !ids[want] {
			t.Fatalf("expected comment %d in subtree of 1", want)
		}
	}
	// Exclude 3 and 4
	for _, exclude := range []uint{3, 4} {
		if ids[exclude] {
			t.Fatalf("comment %d should not be in subtree of 1", exclude)
		}
	}
}

func TestSortSlice_BestTieBreaksDeterministically(t *testing.T) {
	ts := time.Unix(1700000000, 0).UTC()
	nodes := []*CommentResponse{
		{ID: 2, Score: 1.0, CreatedAt: ts},
		{ID: 1, Score: 1.0, CreatedAt: ts},
	}

	sortSlice(nodes, models.SortBest)

	if nodes[0].ID != 1 || nodes[1].ID != 2 {
		t.Fatalf("best sort tie-break should be deterministic by id for equal timestamp/score, got [%d, %d]", nodes[0].ID, nodes[1].ID)
	}
}
