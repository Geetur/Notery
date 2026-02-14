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

	var notes []models.Note
	parseJSONArray(t, w, &notes)
	if len(notes) != 2 {
		t.Fatalf("expected 2 approved notes, got %d", len(notes))
	}
}

func TestGetApprovedNotes_Empty(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "emptylist")

	w := serve("GET", "/notes/approved", "/notes/approved",
		nil, app.GetApprovedNotes, authMW(uid))
	assertStatus(t, w, http.StatusOK)
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
