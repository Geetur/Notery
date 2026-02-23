package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/Geetur/Notery/internal/models"
)

// ===== PurchaseSingleNote TESTS =====

func TestPurchaseSingleNote_HappyPath(t *testing.T) {
	app := testApp(t)
	creator := seedUser(t, app.DB, "pcreator")
	buyer := seedUser(t, app.DB, "pbuyer")
	noteID := seedApprovedNote(t, app.DB, creator)

	w := serve("POST", "/notes/:id/purchase", "/notes/"+strconv.Itoa(int(noteID))+"/purchase",
		jsonBody(map[string]string{}), app.PurchaseSingleNote, authMW(buyer))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["order_id"] == nil {
		t.Fatal("expected order_id in response")
	}
	if r["message"] != "Purchase successful" {
		t.Fatalf("expected success message, got %v", r["message"])
	}
}

func TestPurchaseSingleNote_NoteNotFound(t *testing.T) {
	app := testApp(t)
	buyer := seedUser(t, app.DB, "pbuyer2")

	w := serve("POST", "/notes/:id/purchase", "/notes/99999/purchase",
		jsonBody(map[string]string{}), app.PurchaseSingleNote, authMW(buyer))
	assertStatus(t, w, http.StatusNotFound)
}

func TestPurchaseSingleNote_PendingNote(t *testing.T) {
	app := testApp(t)
	creator := seedUser(t, app.DB, "pcreator3")
	buyer := seedUser(t, app.DB, "pbuyer3")
	noteID := seedPendingNote(t, app.DB, creator)

	w := serve("POST", "/notes/:id/purchase", "/notes/"+strconv.Itoa(int(noteID))+"/purchase",
		jsonBody(map[string]string{}), app.PurchaseSingleNote, authMW(buyer))
	assertStatus(t, w, http.StatusForbidden)
}

func TestPurchaseSingleNote_NoPDF(t *testing.T) {
	app := testApp(t)
	creator := seedUser(t, app.DB, "pcreator4")
	buyer := seedUser(t, app.DB, "pbuyer4")

	// Create approved note without PDF
	sub := models.Subnotery{Name: "nopdf-sub"}
	app.DB.Create(&sub)
	note := models.Note{
		Title:       "No PDF Note",
		Status:      models.StatusApproved,
		SubnoteryID: sub.ID,
		CreatorID:   creator,
		HasPDF:      false,
	}
	app.DB.Create(&note)

	w := serve("POST", "/notes/:id/purchase", "/notes/"+strconv.Itoa(int(note.ID))+"/purchase",
		jsonBody(map[string]string{}), app.PurchaseSingleNote, authMW(buyer))
	assertStatus(t, w, http.StatusForbidden)
}

func TestPurchaseSingleNote_AlreadyPurchased(t *testing.T) {
	app := testApp(t)
	creator := seedUser(t, app.DB, "pcreator5")
	buyer := seedUser(t, app.DB, "pbuyer5")
	noteID := seedApprovedNote(t, app.DB, creator)

	// Create existing purchase
	purchase := models.Purchase{
		UserID: uint(buyer),
		NoteID: noteID,
	}
	app.DB.Create(&purchase)

	w := serve("POST", "/notes/:id/purchase", "/notes/"+strconv.Itoa(int(noteID))+"/purchase",
		jsonBody(map[string]string{}), app.PurchaseSingleNote, authMW(buyer))
	assertStatus(t, w, http.StatusConflict)
}

func TestPurchaseSingleNote_InvalidNoteID(t *testing.T) {
	app := testApp(t)
	buyer := seedUser(t, app.DB, "pbuyer6")

	w := serve("POST", "/notes/:id/purchase", "/notes/abc/purchase",
		jsonBody(map[string]string{}), app.PurchaseSingleNote, authMW(buyer))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestPurchaseSingleNote_IdempotencyKey(t *testing.T) {
	app := testApp(t)
	creator := seedUser(t, app.DB, "pcreator7")
	buyer := seedUser(t, app.DB, "pbuyer7")
	noteID := seedApprovedNote(t, app.DB, creator)

	// First purchase with idempotency key
	w := serve("POST", "/notes/:id/purchase", "/notes/"+strconv.Itoa(int(noteID))+"/purchase",
		jsonBody(map[string]string{"idempotency_key": "unique-key-123"}), app.PurchaseSingleNote, authMW(buyer))
	assertStatus(t, w, http.StatusOK)

	// Second purchase with same idempotency key should return idempotent result
	w2 := serve("POST", "/notes/:id/purchase", "/notes/"+strconv.Itoa(int(noteID))+"/purchase",
		jsonBody(map[string]string{"idempotency_key": "unique-key-123"}), app.PurchaseSingleNote, authMW(buyer))
	assertStatus(t, w2, http.StatusOK)
	r := respJSON(t, w2)
	if r["idempotent"] != true {
		t.Fatal("expected idempotent=true on second call")
	}
}

// ===== CheckPurchaseStatus TESTS =====

func TestCheckPurchaseStatus_NotPurchased(t *testing.T) {
	app := testApp(t)
	buyer := seedUser(t, app.DB, "statusbuyer")
	creator := seedUser(t, app.DB, "statuscreator")
	noteID := seedApprovedNote(t, app.DB, creator)

	w := serve("GET", "/notes/:id/purchased", "/notes/"+strconv.Itoa(int(noteID))+"/purchased",
		nil, app.CheckPurchaseStatus, authMW(buyer))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["purchased"] != false {
		t.Fatal("expected purchased=false")
	}
}

func TestCheckPurchaseStatus_Purchased(t *testing.T) {
	app := testApp(t)
	buyer := seedUser(t, app.DB, "statusbuyer2")
	creator := seedUser(t, app.DB, "statuscreator2")
	noteID := seedApprovedNote(t, app.DB, creator)

	// Create purchase record
	purchase := models.Purchase{
		UserID: uint(buyer),
		NoteID: noteID,
	}
	app.DB.Create(&purchase)

	w := serve("GET", "/notes/:id/purchased", "/notes/"+strconv.Itoa(int(noteID))+"/purchased",
		nil, app.CheckPurchaseStatus, authMW(buyer))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["purchased"] != true {
		t.Fatal("expected purchased=true")
	}
}

func TestCheckPurchaseStatus_InvalidNoteID(t *testing.T) {
	app := testApp(t)
	buyer := seedUser(t, app.DB, "statusbuyer3")

	w := serve("GET", "/notes/:id/purchased", "/notes/abc/purchased",
		nil, app.CheckPurchaseStatus, authMW(buyer))
	assertStatus(t, w, http.StatusBadRequest)
}

// ===== GetPurchaseHistory TESTS =====

func TestGetPurchaseHistory_Empty(t *testing.T) {
	app := testApp(t)
	buyer := seedUser(t, app.DB, "histbuyer")

	w := serve("GET", "/me/purchases/history", "/me/purchases/history",
		nil, app.GetPurchaseHistory, authMW(buyer))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	// purchases should be an empty array (not null) when no records exist
	purchases, ok := r["purchases"].([]interface{})
	if !ok {
		t.Fatal("expected purchases to be a JSON array, got nil or non-array")
	}
	if len(purchases) != 0 {
		t.Fatalf("expected 0 purchases, got %d", len(purchases))
	}
}

func TestGetPurchaseHistory_WithPurchases(t *testing.T) {
	app := testApp(t)
	creator := seedUser(t, app.DB, "histcreator")
	buyer := seedUser(t, app.DB, "histbuyer2")
	noteID := seedApprovedNote(t, app.DB, creator)

	// Create a purchase record
	purchase := models.Purchase{
		UserID:    uint(buyer),
		NoteID:    noteID,
		PricePaid: 499,
	}
	app.DB.Create(&purchase)

	w := serve("GET", "/me/purchases/history", "/me/purchases/history",
		nil, app.GetPurchaseHistory, authMW(buyer))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	total := r["total"].(float64)
	if total != 1 {
		t.Fatalf("expected total=1, got %v", total)
	}
}

func TestGetPurchaseHistory_Pagination(t *testing.T) {
	app := testApp(t)
	creator := seedUser(t, app.DB, "histcreator3")
	buyer := seedUser(t, app.DB, "histbuyer3")

	// Create multiple purchases
	for i := 0; i < 5; i++ {
		noteID := seedApprovedNote(t, app.DB, creator)
		purchase := models.Purchase{
			UserID:    uint(buyer),
			NoteID:    noteID,
			PricePaid: int64(100 * (i + 1)),
		}
		app.DB.Create(&purchase)
	}

	// Request page 1 with limit 2
	w := serve("GET", "/me/purchases/history", "/me/purchases/history?page=1&limit=2",
		nil, app.GetPurchaseHistory, authMW(buyer))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	total := r["total"].(float64)
	if total != 5 {
		t.Fatalf("expected total=5, got %v", total)
	}
	purchases := r["purchases"].([]interface{})
	if len(purchases) != 2 {
		t.Fatalf("expected 2 purchases on page, got %d", len(purchases))
	}
}

// ===== PURCHASE → ACCESS INTEGRATION TESTS =====

// TestPurchaseGrantsFullAccess verifies the full flow:
// 1. Before purchase: CheckPurchaseStatus returns purchased=false
// 2. Purchase the note
// 3. After purchase: CheckPurchaseStatus returns purchased=true
// 4. GetNoteByID returns has_full_access=true
func TestPurchaseGrantsFullAccess(t *testing.T) {
	app := testApp(t)
	creator := seedUser(t, app.DB, "intcreator")
	buyer := seedUser(t, app.DB, "intbuyer")

	// Create a PAID approved note (price > 0) so free-note shortcut doesn't apply
	sub := models.Subnotery{Name: "int-sub"}
	app.DB.Create(&sub)
	note := models.Note{
		Title: "Paid Note", Status: models.StatusApproved,
		SubnoteryID: sub.ID, CreatorID: creator, HasPDF: true, Price: 499,
	}
	app.DB.Create(&note)
	noteStr := strconv.Itoa(int(note.ID))

	// Step 1: Before purchase — purchased=false
	w := serve("GET", "/notes/:id/purchased", "/notes/"+noteStr+"/purchased",
		nil, app.CheckPurchaseStatus, authMW(buyer))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["purchased"] != false {
		t.Fatal("expected purchased=false before purchase")
	}

	// Step 2: Before purchase — has_full_access=false via GetNoteByID
	w = serve("GET", "/notes/:id", "/notes/"+noteStr,
		nil, app.GetNoteByID, authMW(buyer))
	assertStatus(t, w, http.StatusOK)
	var noteResp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&noteResp)
	if noteResp["has_full_access"] == true {
		t.Fatal("expected has_full_access=false before purchase")
	}

	// Step 3: Purchase the note
	w = serve("POST", "/notes/:id/purchase", "/notes/"+noteStr+"/purchase",
		jsonBody(map[string]string{}), app.PurchaseSingleNote, authMW(buyer))
	assertStatus(t, w, http.StatusOK)
	pr := respJSON(t, w)
	if pr["message"] != "Purchase successful" {
		t.Fatalf("purchase failed: %v", pr)
	}

	// Step 4: After purchase — purchased=true
	w = serve("GET", "/notes/:id/purchased", "/notes/"+noteStr+"/purchased",
		nil, app.CheckPurchaseStatus, authMW(buyer))
	assertStatus(t, w, http.StatusOK)
	r = respJSON(t, w)
	if r["purchased"] != true {
		t.Fatal("expected purchased=true after purchase")
	}

	// Step 5: After purchase — has_full_access=true via GetNoteByID
	w = serve("GET", "/notes/:id", "/notes/"+noteStr,
		nil, app.GetNoteByID, authMW(buyer))
	assertStatus(t, w, http.StatusOK)
	json.NewDecoder(w.Body).Decode(&noteResp)
	if noteResp["has_full_access"] != true {
		t.Fatalf("expected has_full_access=true after purchase, got %v", noteResp["has_full_access"])
	}

	// Step 6: Purchase also appears in history
	w = serve("GET", "/me/purchases/history", "/me/purchases/history",
		nil, app.GetPurchaseHistory, authMW(buyer))
	assertStatus(t, w, http.StatusOK)
	r = respJSON(t, w)
	total := r["total"].(float64)
	if total != 1 {
		t.Fatalf("expected 1 purchase in history, got %v", total)
	}

	// Step 7: Purchase also in GetMyPurchases
	w = serve("GET", "/me/purchases", "/me/purchases",
		nil, app.GetMyPurchases, authMW(buyer))
	assertStatus(t, w, http.StatusOK)
	r = respJSON(t, w)
	pnotes := r["purchases"].([]interface{})
	if len(pnotes) != 1 {
		t.Fatalf("expected 1 note in my purchases, got %d", len(pnotes))
	}
}

// TestPurchaseFreeNote verifies free notes (price=0) get has_full_access=true
// without any purchase required (just being authenticated).
func TestFreeNoteFullAccess(t *testing.T) {
	app := testApp(t)
	creator := seedUser(t, app.DB, "freecreator")
	viewer := seedUser(t, app.DB, "freeviewer")

	// Create a free approved note
	sub := models.Subnotery{Name: "free-sub"}
	app.DB.Create(&sub)
	note := models.Note{
		Title:       "Free Note",
		Status:      models.StatusApproved,
		SubnoteryID: sub.ID,
		CreatorID:   creator,
		HasPDF:      true,
		Price:       0,
	}
	app.DB.Create(&note)
	noteStr := strconv.Itoa(int(note.ID))

	// Free note should have has_full_access=true for any authenticated user
	w := serve("GET", "/notes/:id", "/notes/"+noteStr,
		nil, app.GetNoteByID, authMW(viewer))
	assertStatus(t, w, http.StatusOK)
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["has_full_access"] != true {
		t.Fatalf("expected has_full_access=true for free note, got %v", resp["has_full_access"])
	}
}

// TestCreatorAlwaysHasAccess verifies the note creator can see their note without purchasing.
func TestCreatorAlwaysHasAccess(t *testing.T) {
	app := testApp(t)
	creator := seedUser(t, app.DB, "accesscreator")
	noteID := seedApprovedNote(t, app.DB, creator)
	noteStr := strconv.Itoa(int(noteID))

	w := serve("GET", "/notes/:id", "/notes/"+noteStr,
		nil, app.GetNoteByID, authMW(creator))
	assertStatus(t, w, http.StatusOK)
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["has_full_access"] != true {
		t.Fatalf("expected has_full_access=true for creator, got %v", resp["has_full_access"])
	}
}
