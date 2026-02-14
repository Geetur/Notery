package handlers

import (
	"net/http"
	"testing"
)

// ===== SEARCH NOTES TESTS =====
// Note: Meilisearch is not available in unit tests, so we test
// the validation paths and the "not configured" path.

func TestSearchNotes_MissingQuery(t *testing.T) {
	app := testApp(t)

	w := serve("GET", "/search", "/search", nil, app.SearchNotes)
	assertStatus(t, w, http.StatusBadRequest)

	r := respJSON(t, w)
	if r["error"] == nil {
		t.Fatal("expected error about missing query")
	}
}

func TestSearchNotes_MeilisearchNotConfigured(t *testing.T) {
	app := testApp(t)
	// app.Search is nil by default in testApp

	w := serve("GET", "/search", "/search?q=calculus", nil, app.SearchNotes)
	assertStatus(t, w, http.StatusServiceUnavailable)

	r := respJSON(t, w)
	if r["error"] == nil {
		t.Fatal("expected service unavailable error")
	}
}

func TestSearchNotes_EmptyQuery(t *testing.T) {
	app := testApp(t)

	w := serve("GET", "/search", "/search?q=", nil, app.SearchNotes)
	assertStatus(t, w, http.StatusBadRequest)
}
