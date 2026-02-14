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
			"author":         "Author",
			"price":          499,
		}), app.CreateNote, authMW(uid))
	assertStatus(t, w, http.StatusCreated)
	r := respJSON(t, w)
	if r["id"] == nil {
		t.Fatal("expected note id")
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
			"author":         "Author",
			"price":          499,
		}), app.CreateNote, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestCreateNote_MissingAuthor(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "noauthor")

	w := serve("POST", "/notes", "/notes",
		jsonBody(map[string]interface{}{
			"subnotery_name": "test-sub",
			"title":          "Test",
			"price":          100,
		}), app.CreateNote, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestCreateNote_NegativePrice(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "negprice")

	w := serve("POST", "/notes", "/notes",
		jsonBody(map[string]interface{}{
			"subnotery_name": "test-sub",
			"title":          "Test Note",
			"author":         "Author",
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
			"author":         "Author",
			"price":          0,
		}), app.CreateNote, authMW(uid))
	assertStatus(t, w, http.StatusCreated)
}

func TestCreateNote_MissingSubnoteryName(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "nosub")

	w := serve("POST", "/notes", "/notes",
		jsonBody(map[string]interface{}{
			"title":  "Test Note",
			"author": "Author",
			"price":  499,
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
			"author":         "Author",
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
			"author":         "Author",
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
	uid := seedUser(t, app.DB, "pendgetter")
	noteID := seedPendingNote(t, app.DB, uid)

	w := serve("GET", "/notes/:id", fmt.Sprintf("/notes/%d", noteID),
		nil, app.GetNoteByID, authMW(uid))
	assertStatus(t, w, http.StatusForbidden)
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

	r := respJSON(t, w)
	total, _ := r["total"].(float64)
	if total != 2 {
		t.Fatalf("expected total=2 approved notes, got %v", total)
	}
	notes, ok := r["notes"].([]interface{})
	if !ok || len(notes) != 2 {
		t.Fatalf("expected 2 approved notes in response")
	}
}

func TestGetApprovedNotes_Empty(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "emptylist")

	w := serve("GET", "/notes/approved", "/notes/approved",
		nil, app.GetApprovedNotes, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	total, _ := r["total"].(float64)
	if total != 0 {
		t.Fatalf("expected total=0, got %v", total)
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

// ===== GET APPROVED NOTES (PAGINATED) =====

func TestGetApprovedNotes_Paginated(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "apprvpg")

	// Create 5 approved notes
	for i := 0; i < 5; i++ {
		seedApprovedNote(t, app.DB, uid)
	}

	w := serve("GET", "/notes/approved", "/notes/approved?page=1&limit=2", nil,
		app.GetApprovedNotes, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	total, _ := r["total"].(float64)
	if total != 5 {
		t.Fatalf("total=%v, want 5", total)
	}
	notes, ok := r["notes"].([]interface{})
	if !ok {
		t.Fatal("expected notes array")
	}
	if len(notes) != 2 {
		t.Fatalf("got %d notes, want 2 (limit=2)", len(notes))
	}
	if r["page"].(float64) != 1 {
		t.Fatalf("page=%v, want 1", r["page"])
	}
}

func TestGetApprovedNotes_EmptyPaginated(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "apprvempty")

	w := serve("GET", "/notes/approved", "/notes/approved", nil,
		app.GetApprovedNotes, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	total, _ := r["total"].(float64)
	if total != 0 {
		t.Fatalf("total=%v, want 0", total)
	}
}

// ===== GET MY NOTES =====

func TestGetMyNotes_All(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "mynotes")
	seedApprovedNote(t, app.DB, uid)
	seedPendingNote(t, app.DB, uid)

	w := serve("GET", "/me/notes", "/me/notes", nil,
		app.GetMyNotes, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	total, _ := r["total"].(float64)
	if total != 2 {
		t.Fatalf("total=%v, want 2", total)
	}
	if r["status"] != "all" {
		t.Fatalf("status=%v, want 'all'", r["status"])
	}
}

func TestGetMyNotes_FilterByStatus(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "mynotefilter")
	seedApprovedNote(t, app.DB, uid)
	seedApprovedNote(t, app.DB, uid)
	seedPendingNote(t, app.DB, uid)

	w := serve("GET", "/me/notes", "/me/notes?status=approved", nil,
		app.GetMyNotes, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	total, _ := r["total"].(float64)
	if total != 2 {
		t.Fatalf("total=%v, want 2 approved", total)
	}
}

func TestGetMyNotes_FilterPending(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "mynotefp")
	seedApprovedNote(t, app.DB, uid)
	seedPendingNote(t, app.DB, uid)

	w := serve("GET", "/me/notes", "/me/notes?status=pending", nil,
		app.GetMyNotes, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	total, _ := r["total"].(float64)
	if total != 1 {
		t.Fatalf("total=%v, want 1 pending", total)
	}
}

func TestGetMyNotes_InvalidStatus(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "mynoteinv")

	w := serve("GET", "/me/notes", "/me/notes?status=invalid", nil,
		app.GetMyNotes, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestGetMyNotes_Empty(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "mynoteempty")

	w := serve("GET", "/me/notes", "/me/notes", nil,
		app.GetMyNotes, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	total, _ := r["total"].(float64)
	if total != 0 {
		t.Fatalf("total=%v, want 0", total)
	}
}

func TestGetMyNotes_OtherUsersExcluded(t *testing.T) {
	app := testApp(t)
	uid1 := seedUser(t, app.DB, "mynote1")
	uid2 := seedUser(t, app.DB, "mynote2")
	seedApprovedNote(t, app.DB, uid1)
	seedApprovedNote(t, app.DB, uid2)

	w := serve("GET", "/me/notes", "/me/notes", nil,
		app.GetMyNotes, authMW(uid1))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	total, _ := r["total"].(float64)
	if total != 1 {
		t.Fatalf("total=%v, want 1 (only own notes)", total)
	}
}
