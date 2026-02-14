package handlers

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/Geetur/Notery/internal/models"
)

// ===== JoinSubnotery TESTS =====

func TestJoinSubnotery_HappyPath(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "joiner")
	sub := models.Subnotery{Name: "join-test-sub"}
	app.DB.Create(&sub)

	w := serve("POST", "/subnoteries/:subnotery_id/join", "/subnoteries/"+strconv.Itoa(int(sub.ID))+"/join",
		nil, app.JoinSubnotery, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["message"] != "Joined subnotery successfully" {
		t.Fatalf("unexpected message: %v", r["message"])
	}
}

func TestJoinSubnotery_SubnoteryNotFound(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "joiner2")

	w := serve("POST", "/subnoteries/:subnotery_id/join", "/subnoteries/99999/join",
		nil, app.JoinSubnotery, authMW(uid))
	assertStatus(t, w, http.StatusNotFound)
}

func TestJoinSubnotery_InvalidSubnoteryID(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "joiner3")

	w := serve("POST", "/subnoteries/:subnotery_id/join", "/subnoteries/abc/join",
		nil, app.JoinSubnotery, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestJoinSubnotery_DoubleJoin(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "joiner4")
	sub := models.Subnotery{Name: "doublejoin-sub"}
	app.DB.Create(&sub)

	// First join
	w := serve("POST", "/subnoteries/:subnotery_id/join", "/subnoteries/"+strconv.Itoa(int(sub.ID))+"/join",
		nil, app.JoinSubnotery, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Second join should still succeed (idempotent association append)
	w2 := serve("POST", "/subnoteries/:subnotery_id/join", "/subnoteries/"+strconv.Itoa(int(sub.ID))+"/join",
		nil, app.JoinSubnotery, authMW(uid))
	assertStatus(t, w2, http.StatusOK)
}

// ===== AddAdminToSubnotery TESTS =====

func TestAddAdmin_HappyPath(t *testing.T) {
	app := testApp(t)
	adminUID := seedUser(t, app.DB, "admincaller")
	targetUID := seedUser(t, app.DB, "newtarget")
	_ = adminUID

	sub := models.Subnotery{Name: "admin-test-sub"}
	app.DB.Create(&sub)

	w := serve("POST", "/subnoteries/:subnotery_id/admins", "/subnoteries/"+strconv.Itoa(int(sub.ID))+"/admins",
		jsonBody(map[string]string{"email": "newtarget@test.com"}),
		app.AddAdminToSubnotery, authMW(adminUID))
	assertStatus(t, w, http.StatusOK)
	_ = targetUID
}

func TestAddAdmin_UserNotFound(t *testing.T) {
	app := testApp(t)
	adminUID := seedUser(t, app.DB, "admincaller2")
	sub := models.Subnotery{Name: "admin-test-sub2"}
	app.DB.Create(&sub)

	w := serve("POST", "/subnoteries/:subnotery_id/admins", "/subnoteries/"+strconv.Itoa(int(sub.ID))+"/admins",
		jsonBody(map[string]string{"email": "nonexistent@test.com"}),
		app.AddAdminToSubnotery, authMW(adminUID))
	assertStatus(t, w, http.StatusNotFound)
}

func TestAddAdmin_SubnoteryNotFound(t *testing.T) {
	app := testApp(t)
	adminUID := seedUser(t, app.DB, "admincaller3")
	seedUser(t, app.DB, "admintarget3")

	w := serve("POST", "/subnoteries/:subnotery_id/admins", "/subnoteries/99999/admins",
		jsonBody(map[string]string{"email": "admintarget3@test.com"}),
		app.AddAdminToSubnotery, authMW(adminUID))
	assertStatus(t, w, http.StatusNotFound)
}

func TestAddAdmin_MissingEmail(t *testing.T) {
	app := testApp(t)
	adminUID := seedUser(t, app.DB, "admincaller4")
	sub := models.Subnotery{Name: "admin-test-sub4"}
	app.DB.Create(&sub)

	w := serve("POST", "/subnoteries/:subnotery_id/admins", "/subnoteries/"+strconv.Itoa(int(sub.ID))+"/admins",
		jsonBody(map[string]string{}),
		app.AddAdminToSubnotery, authMW(adminUID))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestAddAdmin_InvalidEmail(t *testing.T) {
	app := testApp(t)
	adminUID := seedUser(t, app.DB, "admincaller5")
	sub := models.Subnotery{Name: "admin-test-sub5"}
	app.DB.Create(&sub)

	w := serve("POST", "/subnoteries/:subnotery_id/admins", "/subnoteries/"+strconv.Itoa(int(sub.ID))+"/admins",
		jsonBody(map[string]string{"email": "not-an-email"}),
		app.AddAdminToSubnotery, authMW(adminUID))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestAddAdmin_InvalidSubnoteryID(t *testing.T) {
	app := testApp(t)
	adminUID := seedUser(t, app.DB, "admincaller6")

	w := serve("POST", "/subnoteries/:subnotery_id/admins", "/subnoteries/abc/admins",
		jsonBody(map[string]string{"email": "anyone@test.com"}),
		app.AddAdminToSubnotery, authMW(adminUID))
	assertStatus(t, w, http.StatusBadRequest)
}
