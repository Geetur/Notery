package handlers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/Geetur/Notery/internal/models"
)

// ===== POST /notes/:id/bookmark =====

func TestBookmarkNote_Success(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "bkuser")
	noteID := seedApprovedNote(t, app.DB, uid)

	w := serve("POST", "/notes/:id/bookmark",
		fmt.Sprintf("/notes/%d/bookmark", noteID), nil,
		app.BookmarkNote, authMW(uid))
	assertStatus(t, w, http.StatusCreated)

	r := respJSON(t, w)
	if r["bookmarked"] != true {
		t.Fatalf("bookmarked=%v, want true", r["bookmarked"])
	}
}

func TestBookmarkNote_Idempotent(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "bkidem")
	noteID := seedApprovedNote(t, app.DB, uid)

	// Bookmark twice
	serve("POST", "/notes/:id/bookmark",
		fmt.Sprintf("/notes/%d/bookmark", noteID), nil,
		app.BookmarkNote, authMW(uid))
	w := serve("POST", "/notes/:id/bookmark",
		fmt.Sprintf("/notes/%d/bookmark", noteID), nil,
		app.BookmarkNote, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["bookmarked"] != true {
		t.Fatalf("bookmarked=%v, want true", r["bookmarked"])
	}

	// Only one bookmark record should exist
	var count int64
	app.DB.Model(&models.Bookmark{}).Where("user_id = ?", uid).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 bookmark row, got %d", count)
	}
}

func TestBookmarkNote_NotFound(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "bknf")

	w := serve("POST", "/notes/:id/bookmark",
		"/notes/99999/bookmark", nil,
		app.BookmarkNote, authMW(uid))
	assertStatus(t, w, http.StatusNotFound)
}

func TestBookmarkNote_PendingNote(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "bkpend")
	noteID := seedPendingNote(t, app.DB, uid)

	w := serve("POST", "/notes/:id/bookmark",
		fmt.Sprintf("/notes/%d/bookmark", noteID), nil,
		app.BookmarkNote, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

// ===== DELETE /notes/:id/bookmark =====

func TestRemoveBookmark_Success(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "bkrm")
	noteID := seedApprovedNote(t, app.DB, uid)

	// Add then remove
	serve("POST", "/notes/:id/bookmark",
		fmt.Sprintf("/notes/%d/bookmark", noteID), nil,
		app.BookmarkNote, authMW(uid))
	w := serve("DELETE", "/notes/:id/bookmark",
		fmt.Sprintf("/notes/%d/bookmark", noteID), nil,
		app.RemoveBookmark, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["bookmarked"] != false {
		t.Fatalf("bookmarked=%v, want false", r["bookmarked"])
	}
}

func TestRemoveBookmark_Idempotent(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "bkrmidem")

	// Remove non-existent bookmark
	w := serve("DELETE", "/notes/:id/bookmark",
		"/notes/99999/bookmark", nil,
		app.RemoveBookmark, authMW(uid))
	assertStatus(t, w, http.StatusOK)
}

// ===== GET /me/bookmarks =====

func TestGetMyBookmarks_Empty(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "bkmpty")

	w := serve("GET", "/me/bookmarks", "/me/bookmarks", nil,
		app.GetMyBookmarks, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	total, _ := r["total"].(float64)
	if total != 0 {
		t.Fatalf("total=%v, want 0", total)
	}
}

func TestGetMyBookmarks_WithBookmarks(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "bklist")
	noteID1 := seedApprovedNote(t, app.DB, uid)
	noteID2 := seedApprovedNote(t, app.DB, uid)

	// Bookmark both
	serve("POST", "/notes/:id/bookmark",
		fmt.Sprintf("/notes/%d/bookmark", noteID1), nil,
		app.BookmarkNote, authMW(uid))
	serve("POST", "/notes/:id/bookmark",
		fmt.Sprintf("/notes/%d/bookmark", noteID2), nil,
		app.BookmarkNote, authMW(uid))

	w := serve("GET", "/me/bookmarks", "/me/bookmarks", nil,
		app.GetMyBookmarks, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	total, _ := r["total"].(float64)
	if total != 2 {
		t.Fatalf("total=%v, want 2", total)
	}
}

func TestGetMyBookmarks_ExcludesPendingNotes(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "bkexcl")
	noteID := seedApprovedNote(t, app.DB, uid)

	// Bookmark approved note
	serve("POST", "/notes/:id/bookmark",
		fmt.Sprintf("/notes/%d/bookmark", noteID), nil,
		app.BookmarkNote, authMW(uid))

	// Now change the note to pending (simulate rejection)
	app.DB.Model(&models.Note{}).Where("id = ?", noteID).Update("status", models.StatusPending)

	w := serve("GET", "/me/bookmarks", "/me/bookmarks", nil,
		app.GetMyBookmarks, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	total, _ := r["total"].(float64)
	if total != 0 {
		t.Fatalf("total=%v, want 0 (pending note excluded)", total)
	}
}

// ===== GET /notes/:id/bookmarked =====

func TestCheckBookmarkStatus_Bookmarked(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "bkchk")
	noteID := seedApprovedNote(t, app.DB, uid)

	// Bookmark it
	serve("POST", "/notes/:id/bookmark",
		fmt.Sprintf("/notes/%d/bookmark", noteID), nil,
		app.BookmarkNote, authMW(uid))

	w := serve("GET", "/notes/:id/bookmarked",
		fmt.Sprintf("/notes/%d/bookmarked", noteID), nil,
		app.CheckBookmarkStatus, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["bookmarked"] != true {
		t.Fatalf("bookmarked=%v, want true", r["bookmarked"])
	}
}

func TestCheckBookmarkStatus_NotBookmarked(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "bknobk")
	noteID := seedApprovedNote(t, app.DB, uid)

	w := serve("GET", "/notes/:id/bookmarked",
		fmt.Sprintf("/notes/%d/bookmarked", noteID), nil,
		app.CheckBookmarkStatus, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["bookmarked"] != false {
		t.Fatalf("bookmarked=%v, want false", r["bookmarked"])
	}
}
