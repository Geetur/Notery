package handlers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/Geetur/Notery/internal/models"
)

// ===== BOOKMARK HANDLER TESTS =====

func TestAddBookmark_HappyPath(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Bookmark{})
	uid := seedUser(t, app.DB, "bookmarker")
	noteID := seedApprovedNote(t, app.DB, uid)

	w := serve("POST", "/bookmarks/:note_id", "/bookmarks/"+itoa(noteID),
		nil, app.AddBookmark, authMW(uid))
	assertStatus(t, w, http.StatusCreated)
	r := respJSON(t, w)
	if r["bookmarked"] != true {
		t.Fatal("expected bookmarked=true")
	}
}

func TestAddBookmark_Idempotent(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Bookmark{})
	uid := seedUser(t, app.DB, "bkidem")
	noteID := seedApprovedNote(t, app.DB, uid)

	// Add first time
	serve("POST", "/bookmarks/:note_id", "/bookmarks/"+itoa(noteID),
		nil, app.AddBookmark, authMW(uid))

	// Add again — should succeed with 200 (already bookmarked)
	w := serve("POST", "/bookmarks/:note_id", "/bookmarks/"+itoa(noteID),
		nil, app.AddBookmark, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["bookmarked"] != true {
		t.Fatal("expected bookmarked=true on duplicate")
	}
}

func TestAddBookmark_PendingNote(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Bookmark{})
	uid := seedUser(t, app.DB, "bkpending")
	noteID := seedPendingNote(t, app.DB, uid)

	w := serve("POST", "/bookmarks/:note_id", "/bookmarks/"+itoa(noteID),
		nil, app.AddBookmark, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestAddBookmark_NoteNotFound(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Bookmark{})
	uid := seedUser(t, app.DB, "bk404")

	w := serve("POST", "/bookmarks/:note_id", "/bookmarks/99999",
		nil, app.AddBookmark, authMW(uid))
	assertStatus(t, w, http.StatusNotFound)
}

func TestAddBookmark_InvalidID(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Bookmark{})
	uid := seedUser(t, app.DB, "bkinvalid")

	w := serve("POST", "/bookmarks/:note_id", "/bookmarks/abc",
		nil, app.AddBookmark, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestRemoveBookmark_HappyPath(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Bookmark{})
	uid := seedUser(t, app.DB, "bkremove")
	noteID := seedApprovedNote(t, app.DB, uid)

	// Add bookmark first
	serve("POST", "/bookmarks/:note_id", "/bookmarks/"+itoa(noteID),
		nil, app.AddBookmark, authMW(uid))

	// Remove it
	w := serve("DELETE", "/bookmarks/:note_id", "/bookmarks/"+itoa(noteID),
		nil, app.RemoveBookmark, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["bookmarked"] != false {
		t.Fatal("expected bookmarked=false after removal")
	}
}

func TestRemoveBookmark_Idempotent(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Bookmark{})
	uid := seedUser(t, app.DB, "bkremidm")

	// Remove non-existent bookmark — should succeed
	w := serve("DELETE", "/bookmarks/:note_id", "/bookmarks/99999",
		nil, app.RemoveBookmark, authMW(uid))
	assertStatus(t, w, http.StatusOK)
}

func TestGetBookmarks_Empty(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Bookmark{})
	uid := seedUser(t, app.DB, "bkempty")

	w := serve("GET", "/bookmarks", "/bookmarks",
		nil, app.GetBookmarks, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	notes, ok := r["notes"].([]interface{})
	if !ok || len(notes) != 0 {
		t.Fatalf("expected empty notes array, got %v", r["notes"])
	}
}

func TestGetBookmarks_WithData(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Bookmark{})
	uid := seedUser(t, app.DB, "bklist")
	noteID := seedApprovedNote(t, app.DB, uid)

	// Add bookmark
	serve("POST", "/bookmarks/:note_id", "/bookmarks/"+itoa(noteID),
		nil, app.AddBookmark, authMW(uid))

	// List bookmarks
	w := serve("GET", "/bookmarks", "/bookmarks",
		nil, app.GetBookmarks, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	notes, ok := r["notes"].([]interface{})
	if !ok || len(notes) != 1 {
		t.Fatalf("expected 1 bookmark, got %v", r["notes"])
	}
}

func TestCheckBookmark_NotBookmarked(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Bookmark{})
	uid := seedUser(t, app.DB, "bkcheck")
	noteID := seedApprovedNote(t, app.DB, uid)

	w := serve("GET", "/bookmarks/:note_id", "/bookmarks/"+itoa(noteID),
		nil, app.CheckBookmark, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["bookmarked"] != false {
		t.Fatal("expected bookmarked=false")
	}
}

func TestCheckBookmark_IsBookmarked(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Bookmark{})
	uid := seedUser(t, app.DB, "bkchkyes")
	noteID := seedApprovedNote(t, app.DB, uid)

	serve("POST", "/bookmarks/:note_id", "/bookmarks/"+itoa(noteID),
		nil, app.AddBookmark, authMW(uid))

	w := serve("GET", "/bookmarks/:note_id", "/bookmarks/"+itoa(noteID),
		nil, app.CheckBookmark, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["bookmarked"] != true {
		t.Fatal("expected bookmarked=true")
	}
}

// itoa helper for test note IDs
func itoa(id uint) string {
	return fmt.Sprintf("%d", id)
}
