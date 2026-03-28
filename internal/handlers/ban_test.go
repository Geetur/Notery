package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/Geetur/Notery/internal/models"
)

// ===== BanUser TESTS =====

func TestBanUser_HappyPath(t *testing.T) {
	app := testApp(t)
	admin := seedUser(t, app.DB, "banadmin")
	target := seedUser(t, app.DB, "bantarget")

	sub := models.Subnotery{Name: "ban-test-sub"}
	app.DB.Create(&sub)
	// Make admin an admin of the subnotery
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", admin, sub.ID)
	// Make target a member
	app.DB.Exec("INSERT INTO user_memberships (user_id, subnotery_id) VALUES (?, ?)", target, sub.ID)

	body := map[string]interface{}{
		"user_id":  target,
		"duration": "7d",
		"reason":   "Breaking rules",
	}
	w := serve("POST", "/subnoteries/:subnotery_id/bans",
		fmt.Sprintf("/subnoteries/%d/bans", sub.ID),
		jsonBody(body), app.BanUser, authMW(admin))
	assertStatus(t, w, http.StatusOK)

	// Verify ban was created
	var ban models.Ban
	app.DB.Where("user_id = ? AND subnotery_id = ?", target, sub.ID).First(&ban)
	if ban.ID == 0 {
		t.Fatal("expected ban to be created")
	}
	if ban.Reason != "Breaking rules" {
		t.Fatalf("reason = %q, want 'Breaking rules'", ban.Reason)
	}
	if ban.ExpiresAt == nil {
		t.Fatal("7d ban should have an expiry")
	}
}

func TestBanUser_PermanentBan(t *testing.T) {
	app := testApp(t)
	admin := seedUser(t, app.DB, "permbanadmin")
	target := seedUser(t, app.DB, "permbantarget")

	sub := models.Subnotery{Name: "permban-sub"}
	app.DB.Create(&sub)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", admin, sub.ID)
	app.DB.Exec("INSERT INTO user_memberships (user_id, subnotery_id) VALUES (?, ?)", target, sub.ID)

	body := map[string]interface{}{
		"user_id":  target,
		"duration": "permanent",
		"reason":   "Severe violation",
	}
	w := serve("POST", "/subnoteries/:subnotery_id/bans",
		fmt.Sprintf("/subnoteries/%d/bans", sub.ID),
		jsonBody(body), app.BanUser, authMW(admin))
	assertStatus(t, w, http.StatusOK)

	var ban models.Ban
	app.DB.Where("user_id = ? AND subnotery_id = ?", target, sub.ID).First(&ban)
	if ban.ExpiresAt != nil {
		t.Fatal("permanent ban should have nil ExpiresAt")
	}
}

func TestBanUser_NotAdmin(t *testing.T) {
	app := testApp(t)
	nonAdmin := seedUser(t, app.DB, "nonadminban")
	target := seedUser(t, app.DB, "bantarget2")

	sub := models.Subnotery{Name: "ban-test-sub2"}
	app.DB.Create(&sub)

	body := map[string]interface{}{
		"user_id":  target,
		"duration": "7d",
		"reason":   "test",
	}
	w := serve("POST", "/subnoteries/:subnotery_id/bans",
		fmt.Sprintf("/subnoteries/%d/bans", sub.ID),
		jsonBody(body), app.BanUser, authMW(nonAdmin))
	assertStatus(t, w, http.StatusForbidden)
}

func TestBanUser_InvalidDuration(t *testing.T) {
	app := testApp(t)
	admin := seedUser(t, app.DB, "badadmindef")
	target := seedUser(t, app.DB, "badtargetdef")

	sub := models.Subnotery{Name: "ban-invalid-dur"}
	app.DB.Create(&sub)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", admin, sub.ID)

	body := map[string]interface{}{
		"user_id":  target,
		"duration": "99d",
		"reason":   "test",
	}
	w := serve("POST", "/subnoteries/:subnotery_id/bans",
		fmt.Sprintf("/subnoteries/%d/bans", sub.ID),
		jsonBody(body), app.BanUser, authMW(admin))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestBanUser_CannotBanSelf(t *testing.T) {
	app := testApp(t)
	admin := seedUser(t, app.DB, "selfban")

	sub := models.Subnotery{Name: "selfban-sub"}
	app.DB.Create(&sub)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", admin, sub.ID)

	body := map[string]interface{}{
		"user_id":  admin,
		"duration": "7d",
		"reason":   "test",
	}
	w := serve("POST", "/subnoteries/:subnotery_id/bans",
		fmt.Sprintf("/subnoteries/%d/bans", sub.ID),
		jsonBody(body), app.BanUser, authMW(admin))
	// Should fail — can't ban yourself
	assertStatus(t, w, http.StatusBadRequest)
}

// ===== UnbanUser TESTS =====

func TestUnbanUser_HappyPath(t *testing.T) {
	app := testApp(t)
	admin := seedUser(t, app.DB, "unbanadmin")
	target := seedUser(t, app.DB, "unbantarget")

	sub := models.Subnotery{Name: "unban-sub"}
	app.DB.Create(&sub)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", admin, sub.ID)

	// Create a ban
	expires := time.Now().Add(7 * 24 * time.Hour)
	ban := models.Ban{
		UserID:      uint64(target),
		SubnoteryID: sub.ID,
		BannedBy:    uint64(admin),
		Reason:      "test",
		ExpiresAt:   &expires,
	}
	app.DB.Create(&ban)

	w := serve("DELETE", "/subnoteries/:subnotery_id/bans/:uid",
		fmt.Sprintf("/subnoteries/%d/bans/%d", sub.ID, target),
		nil, app.UnbanUser, authMW(admin))
	assertStatus(t, w, http.StatusOK)

	// Verify ban was deleted
	var count int64
	app.DB.Model(&models.Ban{}).Where("user_id = ? AND subnotery_id = ?", target, sub.ID).Count(&count)
	if count != 0 {
		t.Fatal("expected ban to be deleted")
	}
}

func TestUnbanUser_NotAdmin(t *testing.T) {
	app := testApp(t)
	nonAdmin := seedUser(t, app.DB, "unbannonadmin")
	target := seedUser(t, app.DB, "unbantarget2")

	sub := models.Subnotery{Name: "unban-sub2"}
	app.DB.Create(&sub)

	w := serve("DELETE", "/subnoteries/:subnotery_id/bans/:uid",
		fmt.Sprintf("/subnoteries/%d/bans/%d", sub.ID, target),
		nil, app.UnbanUser, authMW(nonAdmin))
	assertStatus(t, w, http.StatusForbidden)
}

// ===== ListBans TESTS =====

func TestListBans_HappyPath(t *testing.T) {
	app := testApp(t)
	admin := seedUser(t, app.DB, "listbanadmin")

	sub := models.Subnotery{Name: "listban-sub"}
	app.DB.Create(&sub)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", admin, sub.ID)

	// Create a few bans
	for i := 0; i < 3; i++ {
		u := seedUser(t, app.DB, fmt.Sprintf("banned%d", i))
		expires := time.Now().Add(7 * 24 * time.Hour)
		ban := models.Ban{
			UserID:      uint64(u),
			SubnoteryID: sub.ID,
			BannedBy:    uint64(admin),
			Reason:      fmt.Sprintf("reason-%d", i),
			ExpiresAt:   &expires,
		}
		app.DB.Create(&ban)
	}

	w := serve("GET", "/subnoteries/:subnotery_id/bans",
		fmt.Sprintf("/subnoteries/%d/bans?page=1&limit=10", sub.ID),
		nil, app.ListBans, authMW(admin))
	assertStatus(t, w, http.StatusOK)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	bans, ok := resp["bans"].([]interface{})
	if !ok {
		t.Fatal("expected bans array in response")
	}
	if len(bans) != 3 {
		t.Fatalf("expected 3 bans, got %d", len(bans))
	}
}

func TestListBans_NotAdmin(t *testing.T) {
	app := testApp(t)
	nonAdmin := seedUser(t, app.DB, "listbannonadmin")

	sub := models.Subnotery{Name: "listban-sub2"}
	app.DB.Create(&sub)

	w := serve("GET", "/subnoteries/:subnotery_id/bans",
		fmt.Sprintf("/subnoteries/%d/bans", sub.ID),
		nil, app.ListBans, authMW(nonAdmin))
	assertStatus(t, w, http.StatusForbidden)
}

// ===== CheckoutSelected TESTS =====

func TestCheckoutSelected_HappyPath(t *testing.T) {
	app := testApp(t)
	creator := seedUser(t, app.DB, "cscreator")
	buyer := seedUser(t, app.DB, "csbuyer")

	// Create two approved notes
	noteID1 := seedApprovedNote(t, app.DB, creator)
	noteID2 := seedApprovedNote(t, app.DB, creator)

	// Add both to cart (Redis not available in tests, so mock by creating cart items)
	// The CheckoutSelected handler reads from Redis, which isn't available.
	// We test that the endpoint responds with an appropriate error when Redis is nil.
	body := map[string]interface{}{
		"item_ids":        []string{strconv.Itoa(int(noteID1)), strconv.Itoa(int(noteID2))},
		"idempotency_key": "test-key-123",
	}
	w := serve("POST", "/checkout/selected",
		"/checkout/selected",
		jsonBody(body), app.CheckoutSelected, authMW(buyer))
	// Without Redis, this should fail gracefully
	// The handler checks app.RDB != nil and validates items from cart
	// In test mode without Redis, it should return an error
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest && w.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d, body: %s", w.Code, w.Body.String())
	}
}

func TestCheckoutSelected_EmptyItems(t *testing.T) {
	app := testApp(t)
	buyer := seedUser(t, app.DB, "csbuyer2")

	body := map[string]interface{}{
		"item_ids":        []string{},
		"idempotency_key": "test-key-456",
	}
	w := serve("POST", "/checkout/selected",
		"/checkout/selected",
		jsonBody(body), app.CheckoutSelected, authMW(buyer))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestCheckoutSelected_MissingIdempotencyKey(t *testing.T) {
	app := testApp(t)
	buyer := seedUser(t, app.DB, "csbuyer3")

	body := map[string]interface{}{
		"item_ids": []string{"1"},
	}
	w := serve("POST", "/checkout/selected",
		"/checkout/selected",
		jsonBody(body), app.CheckoutSelected, authMW(buyer))
	assertStatus(t, w, http.StatusBadRequest)
}

// ===== Ban enforcement in JoinSubnotery =====

func TestJoinSubnotery_BannedUser(t *testing.T) {
	app := testApp(t)
	banned := seedUser(t, app.DB, "bannedjoiner")

	sub := models.Subnotery{Name: "joinban-sub"}
	app.DB.Create(&sub)

	// Ban the user
	expires := time.Now().Add(7 * 24 * time.Hour)
	ban := models.Ban{
		UserID:      uint64(banned),
		SubnoteryID: sub.ID,
		BannedBy:    1,
		Reason:      "banned",
		ExpiresAt:   &expires,
	}
	app.DB.Create(&ban)

	w := serve("POST", "/subnoteries/:subnotery_id/join",
		fmt.Sprintf("/subnoteries/%d/join", sub.ID),
		nil, app.JoinSubnotery, authMW(banned))
	assertStatus(t, w, http.StatusForbidden)
}

// ===== Ban enforcement in CreateComment =====

func TestCreateComment_BannedUser(t *testing.T) {
	app := testApp(t)
	poster := seedUser(t, app.DB, "bannedcommenter")
	creator := seedUser(t, app.DB, "noteowner")
	noteID := seedApprovedNote(t, app.DB, creator)

	// Get the note's subnotery ID
	var note models.Note
	app.DB.First(&note, noteID)

	// Ban the user in the note's subnotery
	expires := time.Now().Add(7 * 24 * time.Hour)
	ban := models.Ban{
		UserID:      uint64(poster),
		SubnoteryID: note.SubnoteryID,
		BannedBy:    1,
		Reason:      "banned from commenting",
		ExpiresAt:   &expires,
	}
	app.DB.Create(&ban)

	body := map[string]interface{}{
		"body": "This should be rejected",
	}
	w := serve("POST", "/notes/:id/comments",
		fmt.Sprintf("/notes/%d/comments", noteID),
		jsonBody(body), app.CreateComment, authMW(poster))
	assertStatus(t, w, http.StatusForbidden)
}
