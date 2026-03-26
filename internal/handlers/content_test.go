// content_test.go — Tests for PDF preview extraction and GetNotePreview handler.
package handlers

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/Geetur/Notery/internal/models"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// buildOnePage builds a minimal valid 1-page PDF with correct xref offsets.
func buildOnePage() []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")

	obj1Off := b.Len()
	b.WriteString("1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n")

	obj2Off := b.Len()
	b.WriteString("2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n")

	obj3Off := b.Len()
	b.WriteString("3 0 obj<</Type/Page/MediaBox[0 0 612 792]/Parent 2 0 R>>endobj\n")

	xrefOff := b.Len()
	b.WriteString("xref\n0 4\n")
	fmt.Fprintf(&b, "0000000000 65535 f \n")
	fmt.Fprintf(&b, "%010d 00000 n \n", obj1Off)
	fmt.Fprintf(&b, "%010d 00000 n \n", obj2Off)
	fmt.Fprintf(&b, "%010d 00000 n \n", obj3Off)
	b.WriteString("trailer<</Size 4/Root 1 0 R>>\n")
	fmt.Fprintf(&b, "startxref\n%d\n", xrefOff)
	b.WriteString("%%EOF\n")

	return b.Bytes()
}

// buildTestPDF creates a valid PDF with the given number of blank pages.
func buildTestPDF(t *testing.T, pages int) []byte {
	t.Helper()
	if pages < 1 {
		t.Fatal("need at least 1 page")
	}

	onePage := buildOnePage()
	if pages == 1 {
		return onePage
	}

	// Merge N copies of the 1-page PDF.
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	rsc := make([]io.ReadSeeker, pages)
	for i := range rsc {
		rsc[i] = bytes.NewReader(onePage)
	}
	var out bytes.Buffer
	if err := api.MergeRaw(rsc, &out, false, conf); err != nil {
		t.Fatalf("merge %d pages: %v", pages, err)
	}
	return out.Bytes()
}

// ===== extractPreviewPages UNIT TESTS =====

func TestExtractPreviewPages_HappyPath(t *testing.T) {
	pdf := buildTestPDF(t, 10)
	extracted, totalPages, err := extractPreviewPages(bytes.NewReader(pdf), 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if totalPages != 10 {
		t.Fatalf("expected 10 total pages, got %d", totalPages)
	}
	// Verify the extracted PDF has exactly 2 pages.
	count, err := extractPageCount(bytes.NewReader(extracted))
	if err != nil {
		t.Fatalf("failed to read extracted PDF: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 extracted pages, got %d", count)
	}
}

func TestExtractPreviewPages_RequestedEqualToTotal(t *testing.T) {
	pdf := buildTestPDF(t, 5)
	_, _, err := extractPreviewPages(bytes.NewReader(pdf), 5)
	if err == nil {
		t.Fatal("expected error when requesting all pages")
	}
}

func TestExtractPreviewPages_RequestedMoreThanTotal(t *testing.T) {
	pdf := buildTestPDF(t, 3)
	_, _, err := extractPreviewPages(bytes.NewReader(pdf), 10)
	if err == nil {
		t.Fatal("expected error when requesting more pages than available")
	}
}

func TestExtractPreviewPages_SinglePage(t *testing.T) {
	pdf := buildTestPDF(t, 10)
	extracted, total, err := extractPreviewPages(bytes.NewReader(pdf), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 10 {
		t.Fatalf("expected 10 total pages, got %d", total)
	}
	count, err := extractPageCount(bytes.NewReader(extracted))
	if err != nil {
		t.Fatalf("failed to read extracted PDF: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 extracted page, got %d", count)
	}
}

func TestExtractPreviewPages_InvalidPDF(t *testing.T) {
	_, _, err := extractPreviewPages(bytes.NewReader([]byte("not a pdf")), 1)
	if err == nil {
		t.Fatal("expected error for invalid PDF data")
	}
}

// ===== extractPageCount UNIT TESTS =====

func TestExtractPageCount_HappyPath(t *testing.T) {
	pdf := buildTestPDF(t, 7)
	count, err := extractPageCount(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 7 {
		t.Fatalf("expected 7 pages, got %d", count)
	}
}

func TestExtractPageCount_InvalidPDF(t *testing.T) {
	_, err := extractPageCount(bytes.NewReader([]byte("garbage")))
	if err == nil {
		t.Fatal("expected error for invalid PDF")
	}
}

// ===== GetNotePreview HANDLER TESTS =====

func TestGetNotePreview_MissingPagesParam(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "prevuser1")

	w := serve("GET", "/notes/:id/preview", "/notes/1/preview",
		nil, app.GetNotePreview, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestGetNotePreview_InvalidPagesParam(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "prevuser2")

	w := serve("GET", "/notes/:id/preview", "/notes/1/preview?pages=abc",
		nil, app.GetNotePreview, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestGetNotePreview_ZeroPagesParam(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "prevuser3")

	w := serve("GET", "/notes/:id/preview", "/notes/1/preview?pages=0",
		nil, app.GetNotePreview, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestGetNotePreview_NegativePagesParam(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "prevuser4")

	w := serve("GET", "/notes/:id/preview", "/notes/1/preview?pages=-1",
		nil, app.GetNotePreview, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

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
