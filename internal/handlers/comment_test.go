package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

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
