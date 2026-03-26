package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Geetur/Notery/internal/models"
)

// ===== CREATE NOTE TESTS =====

func TestCreateNote_HappyPath(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "notecreator")

	w := serve("POST", "/notes", "/notes",
		jsonBody(map[string]interface{}{
			"subnotery_name": "test-sub",
			"title":          "Test Note",
			"price":          499,
		}), app.CreateNote, authMW(uid))
	assertStatus(t, w, http.StatusCreated)
	r := respJSON(t, w)
	if r["id"] == nil {
		t.Fatal("expected note id")
	}
	// Author should be auto-derived from the creating user's display name
	if r["author"] == nil || r["author"] == "" {
		t.Fatal("expected author to be auto-derived from user")
	}
	if r["status"] != string(models.StatusPending) {
		t.Fatalf("new note should be pending, got %v", r["status"])
	}
}

func TestCreateNote_MissingTitle(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "notittle")

	w := serve("POST", "/notes", "/notes",
		jsonBody(map[string]interface{}{
			"subnotery_name": "test-sub",
			"price":          499,
		}), app.CreateNote, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestCreateNote_AuthorAutoDerived(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "autoauthor")

	w := serve("POST", "/notes", "/notes",
		jsonBody(map[string]interface{}{
			"subnotery_name": "test-sub",
			"title":          "Test",
			"price":          100,
		}), app.CreateNote, authMW(uid))
	assertStatus(t, w, http.StatusCreated)
	// Verify author was auto-set to the user's display name
	r := respJSON(t, w)
	author, _ := r["author"].(string)
	if author == "" {
		t.Fatal("author should be auto-derived, got empty")
	}
}

func TestCreateNote_NegativePrice(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "negprice")

	w := serve("POST", "/notes", "/notes",
		jsonBody(map[string]interface{}{
			"subnotery_name": "test-sub",
			"title":          "Test Note",
			"price":          -1,
		}), app.CreateNote, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestCreateNote_ZeroPrice(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "freecreator")

	w := serve("POST", "/notes", "/notes",
		jsonBody(map[string]interface{}{
			"subnotery_name": "free-sub",
			"title":          "Free Note",
			"price":          0,
		}), app.CreateNote, authMW(uid))
	assertStatus(t, w, http.StatusCreated)
}

func TestCreateNote_MissingSubnoteryName(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "nosub")

	w := serve("POST", "/notes", "/notes",
		jsonBody(map[string]interface{}{
			"title": "Test Note",
			"price": 499,
		}), app.CreateNote, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestCreateNote_AutoCreatesSubnotery(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "autocreator")
	subName := "auto-create-sub"

	serve("POST", "/notes", "/notes",
		jsonBody(map[string]interface{}{
			"subnotery_name": subName,
			"title":          "Auto Note",
			"price":          100,
		}), app.CreateNote, authMW(uid))

	// Verify subnotery was created
	var sub models.Subnotery
	err := app.DB.Where("name = ?", subName).First(&sub).Error
	if err != nil {
		t.Fatalf("subnotery should have been auto-created: %v", err)
	}
}

func TestCreateNote_CreatorBecomesAdminOfNewSubnotery(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "admincheck")
	subName := "new-admin-sub"

	serve("POST", "/notes", "/notes",
		jsonBody(map[string]interface{}{
			"subnotery_name": subName,
			"title":          "Admin Note",
			"price":          100,
		}), app.CreateNote, authMW(uid))

	// Check user is admin
	var count int64
	app.DB.Table("user_admins").Where("user_id = ?", uid).Count(&count)
	if count == 0 {
		t.Fatal("creator should be admin of the new subnotery")
	}
}

// ===== GET NOTE BY ID =====

func TestGetNoteByID_Approved(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "getter")
	noteID := seedApprovedNote(t, app.DB, uid)

	w := serve("GET", "/notes/:id", fmt.Sprintf("/notes/%d", noteID),
		nil, app.GetNoteByID, authMW(uid))
	assertStatus(t, w, http.StatusOK)
}

func TestGetNoteByID_Pending(t *testing.T) {
	app := testApp(t)
	creator := seedUser(t, app.DB, "pendcreator")
	noteID := seedPendingNote(t, app.DB, creator)

	// Creator cannot view their own pending note (only admins via pending queue)
	w := serve("GET", "/notes/:id", fmt.Sprintf("/notes/%d", noteID),
		nil, app.GetNoteByID, authMW(creator))
	assertStatus(t, w, http.StatusForbidden)

	// Another user also cannot view a pending note
	other := seedUser(t, app.DB, "pendother")
	w = serve("GET", "/notes/:id", fmt.Sprintf("/notes/%d", noteID),
		nil, app.GetNoteByID, authMW(other))
	assertStatus(t, w, http.StatusForbidden)
}

func TestGetNoteByID_PendingGlobalAdmin(t *testing.T) {
	app := testApp(t)
	creator := seedUser(t, app.DB, "pendcreator2")
	noteID := seedPendingNote(t, app.DB, creator)

	admin := seedUser(t, app.DB, "pendgadmin")
	app.DB.Model(&models.User{}).Where("id = ?", admin).Update("is_global_admin", true)

	// Global admin CAN view a pending note
	w := serve("GET", "/notes/:id", fmt.Sprintf("/notes/%d", noteID),
		nil, app.GetNoteByID, authMW(admin))
	assertStatus(t, w, http.StatusOK)
}

func TestGetNoteByID_PendingSubnoteryAdmin(t *testing.T) {
	app := testApp(t)
	creator := seedUser(t, app.DB, "pendcreator3")
	noteID := seedPendingNote(t, app.DB, creator)

	// Look up the note's subnotery
	var note models.Note
	app.DB.First(&note, noteID)

	subAdmin := seedUser(t, app.DB, "pendsadmin")
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", subAdmin, note.SubnoteryID)

	// Subnotery admin CAN view a pending note in their subnotery
	w := serve("GET", "/notes/:id", fmt.Sprintf("/notes/%d", noteID),
		nil, app.GetNoteByID, authMW(subAdmin))
	assertStatus(t, w, http.StatusOK)
}

func TestGetNoteByID_NotFound(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "nfgetter")

	w := serve("GET", "/notes/:id", "/notes/99999",
		nil, app.GetNoteByID, authMW(uid))
	assertStatus(t, w, http.StatusNotFound)
}

func TestGetNoteByID_InvalidID(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "badidgetter")

	w := serve("GET", "/notes/:id", "/notes/abc",
		nil, app.GetNoteByID, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

// ===== GET APPROVED NOTES =====

func TestGetApprovedNotes_HappyPath(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "listuser")

	seedApprovedNote(t, app.DB, uid)
	seedApprovedNote(t, app.DB, uid)
	seedPendingNote(t, app.DB, uid) // should not appear

	w := serve("GET", "/notes/approved", "/notes/approved",
		nil, app.GetApprovedNotes, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	notes, ok := resp["notes"].([]interface{})
	if !ok {
		t.Fatalf("expected 'notes' key in response")
	}
	if len(notes) != 2 {
		t.Fatalf("expected 2 approved notes, got %d", len(notes))
	}
	if resp["total"] == nil || resp["page"] == nil || resp["limit"] == nil {
		t.Fatal("missing pagination fields in response")
	}
}

func TestGetApprovedNotes_Empty(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "emptylist")

	w := serve("GET", "/notes/approved", "/notes/approved",
		nil, app.GetApprovedNotes, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	notes, ok := resp["notes"].([]interface{})
	if !ok {
		t.Fatalf("expected 'notes' key in response")
	}
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes, got %d", len(notes))
	}
}

// ===== GET PENDING NOTES (ADMIN) =====

func TestGetPendingNotes_GlobalAdmin(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "globaladmnotes")
	app.DB.Model(&models.User{}).Where("id = ?", uid).Update("is_global_admin", true)

	seedPendingNote(t, app.DB, uid)
	seedPendingNote(t, app.DB, uid)

	w := serve("GET", "/notes/pending", "/notes/pending",
		nil, app.GetPendingNotes, adminMW(uid))
	assertStatus(t, w, http.StatusOK)
}

// ===== APPROVE NOTE =====

func TestApproveNote_WithoutPDF(t *testing.T) {
	app := testApp(t)
	creator := seedUser(t, app.DB, "nopdfcreator")
	adminUID := seedUser(t, app.DB, "nopdfadmin")
	noteID := seedPendingNote(t, app.DB, creator) // no PDF

	w := serve("PATCH", "/notes/:id/approve", fmt.Sprintf("/notes/%d/approve", noteID),
		nil, app.ApproveNote, adminMW(adminUID))
	assertStatus(t, w, http.StatusBadRequest) // can't approve without PDF
}

// ===== DELETE NOTE =====

func TestDeleteNote_HappyPath(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "delnoter")
	noteID := seedPendingNote(t, app.DB, uid)

	w := serve("DELETE", "/notes/:id", fmt.Sprintf("/notes/%d", noteID),
		nil, app.DeleteNote, adminMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Verify deleted
	var count int64
	app.DB.Model(&models.Note{}).Where("id = ?", noteID).Count(&count)
	if count != 0 {
		t.Fatal("note should be deleted")
	}
}

func TestDeleteNote_NotFound(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "delnf")

	w := serve("DELETE", "/notes/:id", "/notes/99999",
		nil, app.DeleteNote, adminMW(uid))
	assertStatus(t, w, http.StatusNotFound)
}

// ===== REJECT NOTE =====

func TestRejectNote_HappyPath(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "rejector")
	noteID := seedPendingNote(t, app.DB, uid)

	w := serve("PATCH", "/notes/:id/reject", fmt.Sprintf("/notes/%d/reject", noteID),
		nil, app.RejectNote, adminMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Verify deleted (reject also deletes)
	var count int64
	app.DB.Model(&models.Note{}).Where("id = ?", noteID).Count(&count)
	if count != 0 {
		t.Fatal("rejected note should be deleted")
	}
}

// ===== LOCK / UNLOCK NOTE =====

func TestLockNote_HappyPath(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "lockadmin")
	noteID := seedApprovedNote(t, app.DB, uid)

	w := serve("PATCH", "/notes/:id/lock", fmt.Sprintf("/notes/%d/lock", noteID),
		nil, app.LockNote, adminMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Verify is_locked is true in DB
	var note models.Note
	app.DB.First(&note, noteID)
	if !note.IsLocked {
		t.Fatal("expected note to be locked")
	}
}

func TestLockNote_NotFound(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "locknf")

	w := serve("PATCH", "/notes/:id/lock", "/notes/99999/lock",
		nil, app.LockNote, adminMW(uid))
	assertStatus(t, w, http.StatusNotFound)
}

func TestLockNote_InvalidID(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "lockbadid")

	w := serve("PATCH", "/notes/:id/lock", "/notes/abc/lock",
		nil, app.LockNote, adminMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestUnlockNote_HappyPath(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "unlockadmin")
	noteID := seedApprovedNote(t, app.DB, uid)

	// Lock first
	app.DB.Model(&models.Note{}).Where("id = ?", noteID).Update("is_locked", true)

	// Then unlock
	w := serve("PATCH", "/notes/:id/unlock", fmt.Sprintf("/notes/%d/unlock", noteID),
		nil, app.UnlockNote, adminMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Verify is_locked is false in DB
	var note models.Note
	app.DB.First(&note, noteID)
	if note.IsLocked {
		t.Fatal("expected note to be unlocked")
	}
}

func TestUnlockNote_NotFound(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "unlocknf")

	w := serve("PATCH", "/notes/:id/unlock", "/notes/99999/unlock",
		nil, app.UnlockNote, adminMW(uid))
	assertStatus(t, w, http.StatusNotFound)
}

// ===== HELPER =====

func parseJSONArray(t *testing.T, w *httptest.ResponseRecorder, dest interface{}) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), dest); err != nil {
		t.Fatalf("failed to parse json array: %v | body: %s", err, w.Body.String())
	}
}

// ===== ANONYMOUS ACCESS TESTS =====

func TestGetNoteByID_Anonymous_Approved(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "anonview")
	noteID := seedApprovedNote(t, app.DB, uid)

	// No authMW — anonymous request
	w := serve("GET", "/notes/:id", fmt.Sprintf("/notes/%d", noteID),
		nil, app.GetNoteByID)
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["has_full_access"] != false {
		t.Fatal("anonymous user should not have full access")
	}
	if r["user_vote"] != "" {
		t.Fatalf("anonymous user should have empty user_vote, got %v", r["user_vote"])
	}
}

func TestGetNoteByID_Anonymous_Pending(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "anonpend")
	noteID := seedPendingNote(t, app.DB, uid)

	// No authMW — anonymous request
	w := serve("GET", "/notes/:id", fmt.Sprintf("/notes/%d", noteID),
		nil, app.GetNoteByID)
	assertStatus(t, w, http.StatusForbidden)
}

func TestGetNoteByID_Anonymous_SoftDeleted(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "anonsoft")
	noteID := seedApprovedNote(t, app.DB, uid)

	// Soft-delete the note
	app.DB.Delete(&models.Note{}, noteID)

	// No authMW — anonymous request
	w := serve("GET", "/notes/:id", fmt.Sprintf("/notes/%d", noteID),
		nil, app.GetNoteByID)
	assertStatus(t, w, http.StatusNotFound)
}

func TestGetApprovedNotes_Anonymous(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "anonlist")
	seedApprovedNote(t, app.DB, uid)
	seedApprovedNote(t, app.DB, uid)
	seedPendingNote(t, app.DB, uid) // should not appear

	// No authMW — anonymous request
	w := serve("GET", "/notes/approved", "/notes/approved",
		nil, app.GetApprovedNotes)
	assertStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	notes, _ := resp["notes"].([]interface{})
	if len(notes) != 2 {
		t.Fatalf("expected 2 approved notes, got %d", len(notes))
	}
}

// ===== SORT CORRECTNESS TESTS =====

func TestGetApprovedNotes_SortTop(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "sorttop")
	n1 := seedApprovedNote(t, app.DB, uid)
	n2 := seedApprovedNote(t, app.DB, uid)
	n3 := seedApprovedNote(t, app.DB, uid)

	// Give different vote counts
	app.DB.Model(&models.Note{}).Where("id = ?", n1).Updates(map[string]interface{}{"upvotes": 10, "downvotes": 2})
	app.DB.Model(&models.Note{}).Where("id = ?", n2).Updates(map[string]interface{}{"upvotes": 5, "downvotes": 0})
	app.DB.Model(&models.Note{}).Where("id = ?", n3).Updates(map[string]interface{}{"upvotes": 20, "downvotes": 1})

	w := serve("GET", "/notes/approved", "/notes/approved?sort=top&time=all",
		nil, app.GetApprovedNotes, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	notes, _ := resp["notes"].([]interface{})
	if len(notes) != 3 {
		t.Fatalf("expected 3 notes, got %d", len(notes))
	}

	// n3 (net 19) should be first, n1 (net 8) second, n2 (net 5) third
	first := notes[0].(map[string]interface{})
	if uint(first["id"].(float64)) != n3 {
		t.Fatalf("expected note %d first (highest net votes), got %v", n3, first["id"])
	}
}

func TestGetApprovedNotes_SortControversial(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "sortcont")
	n1 := seedApprovedNote(t, app.DB, uid)
	n2 := seedApprovedNote(t, app.DB, uid)
	n3 := seedApprovedNote(t, app.DB, uid)

	// n1: 10 up, 9 down → total=19, net=1, controversy=19/1=19
	// n2: 5 up, 0 down → total=5, net=5, controversy=5/5=1
	// n3: 8 up, 7 down → total=15, net=1, controversy=15/1=15
	app.DB.Model(&models.Note{}).Where("id = ?", n1).Updates(map[string]interface{}{"upvotes": 10, "downvotes": 9})
	app.DB.Model(&models.Note{}).Where("id = ?", n2).Updates(map[string]interface{}{"upvotes": 5, "downvotes": 0})
	app.DB.Model(&models.Note{}).Where("id = ?", n3).Updates(map[string]interface{}{"upvotes": 8, "downvotes": 7})

	w := serve("GET", "/notes/approved", "/notes/approved?sort=controversial&time=all",
		nil, app.GetApprovedNotes, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	notes, _ := resp["notes"].([]interface{})
	if len(notes) != 3 {
		t.Fatalf("expected 3 notes, got %d", len(notes))
	}

	// n1 (controversy 19) should be first
	first := notes[0].(map[string]interface{})
	if uint(first["id"].(float64)) != n1 {
		t.Fatalf("expected note %d first (most controversial), got %v", n1, first["id"])
	}
}

func TestGetApprovedNotes_SortTop_TimeFilterAll(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "sorttimeall")

	// Create notes — SQLite test DB uses created_at from insert time (now),
	// but this verifies time=all doesn't exclude anything
	seedApprovedNote(t, app.DB, uid)
	seedApprovedNote(t, app.DB, uid)

	w := serve("GET", "/notes/approved", "/notes/approved?sort=top&time=all",
		nil, app.GetApprovedNotes, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	notes, _ := resp["notes"].([]interface{})
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes with time=all, got %d", len(notes))
	}
}

func TestGetApprovedNotes_SortControversial_ZeroVotes(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "sortczero")

	// Notes with zero votes should still be returned (not crash the query)
	seedApprovedNote(t, app.DB, uid)
	seedApprovedNote(t, app.DB, uid)

	w := serve("GET", "/notes/approved", "/notes/approved?sort=controversial&time=all",
		nil, app.GetApprovedNotes, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	notes, _ := resp["notes"].([]interface{})
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(notes))
	}
}

func TestGetApprovedNotes_Anonymous_SortTop(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "anonsort")
	n1 := seedApprovedNote(t, app.DB, uid)
	n2 := seedApprovedNote(t, app.DB, uid)

	app.DB.Model(&models.Note{}).Where("id = ?", n1).Updates(map[string]interface{}{"upvotes": 3, "downvotes": 0})
	app.DB.Model(&models.Note{}).Where("id = ?", n2).Updates(map[string]interface{}{"upvotes": 10, "downvotes": 1})

	// No authMW — anonymous
	w := serve("GET", "/notes/approved", "/notes/approved?sort=top&time=all",
		nil, app.GetApprovedNotes)
	assertStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	notes, _ := resp["notes"].([]interface{})
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(notes))
	}

	// n2 (net 9) should be first
	first := notes[0].(map[string]interface{})
	if uint(first["id"].(float64)) != n2 {
		t.Fatalf("expected note %d first, got %v", n2, first["id"])
	}
}
