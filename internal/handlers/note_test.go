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
	uid := seedUser(t, app.DB, "approver")
	noteID := seedPendingNote(t, app.DB, uid) // no PDF

	w := serve("PATCH", "/notes/:id/approve", fmt.Sprintf("/notes/%d/approve", noteID),
		nil, app.ApproveNote, adminMW(uid))
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

// ===== HELPER =====

func parseJSONArray(t *testing.T, w *httptest.ResponseRecorder, dest interface{}) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), dest); err != nil {
		t.Fatalf("failed to parse json array: %v | body: %s", err, w.Body.String())
	}
}
