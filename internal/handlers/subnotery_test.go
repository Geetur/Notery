package handlers

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

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

// ===== ListSubnoteries TESTS =====

func TestListSubnoteries_Empty(t *testing.T) {
	app := testApp(t)

	w := serve("GET", "/subnoteries", "/subnoteries", nil, app.ListSubnoteries)
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	total, _ := r["total"].(float64)
	if total != 0 {
		t.Fatalf("total=%v, want 0", total)
	}
}

func TestListSubnoteries_WithData(t *testing.T) {
	app := testApp(t)
	app.DB.Create(&models.Subnotery{Name: "alpha-sub"})
	app.DB.Create(&models.Subnotery{Name: "beta-sub"})

	w := serve("GET", "/subnoteries", "/subnoteries", nil, app.ListSubnoteries)
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	total, _ := r["total"].(float64)
	if total != 2 {
		t.Fatalf("total=%v, want 2", total)
	}
	subs, ok := r["subnoteries"].([]interface{})
	if !ok || len(subs) != 2 {
		t.Fatalf("expected 2 subnoteries in response")
	}
}

func TestListSubnoteries_IncludesCounts(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "listcount")
	sub := models.Subnotery{Name: "counted-sub"}
	app.DB.Create(&sub)

	// Add a member
	app.DB.Exec("INSERT INTO user_memberships (user_id, subnotery_id) VALUES (?, ?)", uid, sub.ID)

	// Add an approved note
	app.DB.Create(&models.Note{
		Title: "Count Note", Status: models.StatusApproved,
		SubnoteryID: sub.ID, CreatorID: uid,
	})

	w := serve("GET", "/subnoteries", "/subnoteries", nil, app.ListSubnoteries)
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	subs := r["subnoteries"].([]interface{})
	first := subs[0].(map[string]interface{})
	if first["member_count"].(float64) != 1 {
		t.Fatalf("member_count=%v, want 1", first["member_count"])
	}
	if first["note_count"].(float64) != 1 {
		t.Fatalf("note_count=%v, want 1", first["note_count"])
	}
}

// ===== GetSubnotery TESTS =====

func TestGetSubnotery_Success(t *testing.T) {
	app := testApp(t)
	sub := models.Subnotery{Name: "detail-sub"}
	app.DB.Create(&sub)

	w := serve("GET", "/subnoteries/:subnotery_id",
		"/subnoteries/"+strconv.Itoa(int(sub.ID)), nil,
		app.GetSubnotery)
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["name"] != "detail-sub" {
		t.Fatalf("name=%v, want 'detail-sub'", r["name"])
	}
	if r["is_member"] != false {
		t.Fatalf("is_member=%v, want false (no auth)", r["is_member"])
	}
}

func TestGetSubnotery_NotFound(t *testing.T) {
	app := testApp(t)

	w := serve("GET", "/subnoteries/:subnotery_id",
		"/subnoteries/99999", nil,
		app.GetSubnotery)
	assertStatus(t, w, http.StatusNotFound)
}

func TestGetSubnotery_WithMember(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "getsubmem")
	sub := models.Subnotery{Name: "member-check-sub"}
	app.DB.Create(&sub)
	app.DB.Exec("INSERT INTO user_memberships (user_id, subnotery_id) VALUES (?, ?)", uid, sub.ID)

	// Simulate OptionalAuth by setting user_id in context
	optionalMW := func(c *gin.Context) { c.Set("user_id", uid) }

	w := serve("GET", "/subnoteries/:subnotery_id",
		"/subnoteries/"+strconv.Itoa(int(sub.ID)), nil,
		app.GetSubnotery, optionalMW)
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["is_member"] != true {
		t.Fatalf("is_member=%v, want true", r["is_member"])
	}
}

// ===== LeaveSubnotery TESTS =====

func TestLeaveSubnotery_Success(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "leaver")
	sub := models.Subnotery{Name: "leave-sub"}
	app.DB.Create(&sub)

	// Join first
	app.DB.Exec("INSERT INTO user_memberships (user_id, subnotery_id) VALUES (?, ?)", uid, sub.ID)

	w := serve("DELETE", "/subnoteries/:subnotery_id/membership",
		"/subnoteries/"+strconv.Itoa(int(sub.ID))+"/membership", nil,
		app.LeaveSubnotery, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["message"] != "Left subnotery successfully" {
		t.Fatalf("unexpected message: %v", r["message"])
	}
}

func TestLeaveSubnotery_NotMember(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "nonmem")
	sub := models.Subnotery{Name: "leave-notmem-sub"}
	app.DB.Create(&sub)

	w := serve("DELETE", "/subnoteries/:subnotery_id/membership",
		"/subnoteries/"+strconv.Itoa(int(sub.ID))+"/membership", nil,
		app.LeaveSubnotery, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestLeaveSubnotery_AdminBlocked(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "adminleave")
	sub := models.Subnotery{Name: "leave-admin-sub"}
	app.DB.Create(&sub)

	// Make user a member and admin
	app.DB.Exec("INSERT INTO user_memberships (user_id, subnotery_id) VALUES (?, ?)", uid, sub.ID)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", uid, sub.ID)

	w := serve("DELETE", "/subnoteries/:subnotery_id/membership",
		"/subnoteries/"+strconv.Itoa(int(sub.ID))+"/membership", nil,
		app.LeaveSubnotery, authMW(uid))
	assertStatus(t, w, http.StatusForbidden)
}

func TestLeaveSubnotery_NotFound(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "leavenf")

	w := serve("DELETE", "/subnoteries/:subnotery_id/membership",
		"/subnoteries/99999/membership", nil,
		app.LeaveSubnotery, authMW(uid))
	assertStatus(t, w, http.StatusNotFound)
}
