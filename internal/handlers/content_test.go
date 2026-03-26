// content_test.go — Tests for GetNotePreview handler.
package handlers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/Geetur/Notery/internal/models"
)

// ===== GetNotePreview HANDLER TESTS =====

func TestGetNotePreview_NoteNotFound(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "prevuser5")

	w := serve("GET", "/notes/:id/preview", "/notes/99999/preview?pages=1",
		nil, app.GetNotePreview, authMW(uid))
	assertStatus(t, w, http.StatusNotFound)
}

func TestGetNotePreview_PendingNote_NotAdmin(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "prevuser6")
	noteID := seedPendingNote(t, app.DB, uid+999) // another user's pending note

	w := serve("GET", "/notes/:id/preview",
		fmt.Sprintf("/notes/%d/preview?pages=1", noteID),
		nil, app.GetNotePreview, authMW(uid))
	assertStatus(t, w, http.StatusForbidden)
}

func TestGetNotePreview_NoPDF(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "prevuser7")
	// Create an approved note without a PDF.
	sub := models.Subnotery{Name: "prevsub7"}
	app.DB.Create(&sub)
	note := models.Note{
		Title: "No PDF Note", Status: models.StatusApproved,
		SubnoteryID: sub.ID, CreatorID: uid, HasPDF: false,
	}
	app.DB.Create(&note)

	w := serve("GET", "/notes/:id/preview",
		fmt.Sprintf("/notes/%d/preview?pages=1", note.ID),
		nil, app.GetNotePreview, authMW(uid))
	assertStatus(t, w, http.StatusNotFound)
}
