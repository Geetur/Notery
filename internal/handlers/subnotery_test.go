package handlers

import (
	"fmt"
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

// ===== ListSubnoteries TESTS =====

func TestListSubnoteries_Empty(t *testing.T) {
	app := testApp(t)

	w := serve("GET", "/subnoteries", "/subnoteries", nil, app.ListSubnoteries)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	subs := r["subnoteries"].([]interface{})
	if len(subs) != 0 {
		t.Fatalf("expected 0 subnoteries, got %d", len(subs))
	}
	if r["total"].(float64) != 0 {
		t.Fatalf("expected total 0, got %v", r["total"])
	}
}

func TestListSubnoteries_WithData(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "listuser1")

	sub1 := models.Subnotery{Name: "list-sub-1"}
	sub2 := models.Subnotery{Name: "list-sub-2"}
	app.DB.Create(&sub1)
	app.DB.Create(&sub2)

	// Add user as member to sub1
	var user models.User
	app.DB.First(&user, uid)
	app.DB.Model(&sub1).Association("Members").Append(&user)

	w := serve("GET", "/subnoteries", "/subnoteries", nil, app.ListSubnoteries)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	subs := r["subnoteries"].([]interface{})
	if len(subs) != 2 {
		t.Fatalf("expected 2 subnoteries, got %d", len(subs))
	}
	if r["total"].(float64) != 2 {
		t.Fatalf("expected total 2, got %v", r["total"])
	}
}

func TestListSubnoteries_Pagination(t *testing.T) {
	app := testApp(t)
	for i := 0; i < 5; i++ {
		app.DB.Create(&models.Subnotery{Name: fmt.Sprintf("pag-sub-%d", i)})
	}

	w := serve("GET", "/subnoteries", "/subnoteries?page=1&limit=2", nil, app.ListSubnoteries)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	subs := r["subnoteries"].([]interface{})
	if len(subs) != 2 {
		t.Fatalf("expected 2 subnoteries, got %d", len(subs))
	}
	if r["total"].(float64) != 5 {
		t.Fatalf("expected total 5, got %v", r["total"])
	}
}

// ===== GetSubnoteryDetail TESTS =====

func TestGetSubnoteryDetail_HappyPath(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "detailuser1")

	sub := models.Subnotery{Name: "detail-sub"}
	app.DB.Create(&sub)

	var user models.User
	app.DB.First(&user, uid)
	app.DB.Model(&sub).Association("Admins").Append(&user)
	app.DB.Model(&sub).Association("Members").Append(&user)

	w := serve("GET", "/subnoteries/:subnotery_id", "/subnoteries/"+strconv.Itoa(int(sub.ID)),
		nil, app.GetSubnoteryDetail)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["name"] != "detail-sub" {
		t.Fatalf("unexpected name: %v", r["name"])
	}
	admins := r["admins"].([]interface{})
	if len(admins) != 1 {
		t.Fatalf("expected 1 admin, got %d", len(admins))
	}
	if r["member_count"].(float64) != 1 {
		t.Fatalf("expected 1 member, got %v", r["member_count"])
	}
}

func TestGetSubnoteryDetail_NotFound(t *testing.T) {
	app := testApp(t)

	w := serve("GET", "/subnoteries/:subnotery_id", "/subnoteries/99999",
		nil, app.GetSubnoteryDetail)
	assertStatus(t, w, http.StatusNotFound)
}

func TestGetSubnoteryDetail_InvalidID(t *testing.T) {
	app := testApp(t)

	w := serve("GET", "/subnoteries/:subnotery_id", "/subnoteries/abc",
		nil, app.GetSubnoteryDetail)
	assertStatus(t, w, http.StatusBadRequest)
}

// ===== GetSubnoteryNotes TESTS =====

func TestGetSubnoteryNotes_OnlyApproved(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "subnoteuser1")

	sub := models.Subnotery{Name: "notes-sub-1"}
	app.DB.Create(&sub)

	// Create approved + pending notes in same subnotery
	app.DB.Create(&models.Note{
		Title: "Approved Note", Status: models.StatusApproved,
		SubnoteryID: sub.ID, CreatorID: uid, HasPDF: true,
	})
	app.DB.Create(&models.Note{
		Title: "Pending Note", Status: models.StatusPending,
		SubnoteryID: sub.ID, CreatorID: uid,
	})

	w := serve("GET", "/subnoteries/:subnotery_id/notes",
		"/subnoteries/"+strconv.Itoa(int(sub.ID))+"/notes",
		nil, app.GetSubnoteryNotes)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	notes := r["notes"].([]interface{})
	if len(notes) != 1 {
		t.Fatalf("expected 1 approved note, got %d", len(notes))
	}
	if r["total"].(float64) != 1 {
		t.Fatalf("expected total 1, got %v", r["total"])
	}
}

func TestGetSubnoteryNotes_Empty(t *testing.T) {
	app := testApp(t)
	sub := models.Subnotery{Name: "empty-notes-sub"}
	app.DB.Create(&sub)

	w := serve("GET", "/subnoteries/:subnotery_id/notes",
		"/subnoteries/"+strconv.Itoa(int(sub.ID))+"/notes",
		nil, app.GetSubnoteryNotes)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	notes := r["notes"].([]interface{})
	if len(notes) != 0 {
		t.Fatalf("expected 0 notes, got %d", len(notes))
	}
}

func TestGetSubnoteryNotes_InvalidID(t *testing.T) {
	app := testApp(t)

	w := serve("GET", "/subnoteries/:subnotery_id/notes", "/subnoteries/abc/notes",
		nil, app.GetSubnoteryNotes)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestGetSubnoteryNotes_Pagination(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "subnoteuser2")

	sub := models.Subnotery{Name: "pag-notes-sub"}
	app.DB.Create(&sub)

	for i := 0; i < 5; i++ {
		app.DB.Create(&models.Note{
			Title:       fmt.Sprintf("Note %d", i),
			Status:      models.StatusApproved,
			SubnoteryID: sub.ID,
			CreatorID:   uid,
			HasPDF:      true,
		})
	}

	w := serve("GET", "/subnoteries/:subnotery_id/notes",
		"/subnoteries/"+strconv.Itoa(int(sub.ID))+"/notes?page=1&limit=2",
		nil, app.GetSubnoteryNotes)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	notes := r["notes"].([]interface{})
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(notes))
	}
	if r["total"].(float64) != 5 {
		t.Fatalf("expected total 5, got %v", r["total"])
	}
}

// ===== LeaveSubnotery TESTS =====

func TestLeaveSubnotery_HappyPath(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "leaver")
	sub := models.Subnotery{Name: "leave-sub"}
	app.DB.Create(&sub)

	// Join first
	var user models.User
	app.DB.First(&user, uid)
	app.DB.Model(&sub).Association("Members").Append(&user)

	// Verify membership
	var count int64
	app.DB.Table("user_memberships").Where("user_id = ? AND subnotery_id = ?", uid, sub.ID).Count(&count)
	if count != 1 {
		t.Fatal("expected user to be a member before leaving")
	}

	// Leave
	w := serve("POST", "/subnoteries/:subnotery_id/leave", "/subnoteries/"+strconv.Itoa(int(sub.ID))+"/leave",
		nil, app.LeaveSubnotery, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Verify no longer a member
	app.DB.Table("user_memberships").Where("user_id = ? AND subnotery_id = ?", uid, sub.ID).Count(&count)
	if count != 0 {
		t.Fatal("expected user to be removed from memberships after leaving")
	}
}

func TestLeaveSubnotery_AdminCannotLeave(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "adminleaver")
	sub := models.Subnotery{Name: "admin-leave-sub"}
	app.DB.Create(&sub)

	var user models.User
	app.DB.First(&user, uid)
	app.DB.Model(&sub).Association("Admins").Append(&user)
	app.DB.Model(&sub).Association("Members").Append(&user)

	w := serve("POST", "/subnoteries/:subnotery_id/leave", "/subnoteries/"+strconv.Itoa(int(sub.ID))+"/leave",
		nil, app.LeaveSubnotery, authMW(uid))
	assertStatus(t, w, http.StatusForbidden)
}

func TestLeaveSubnotery_SubnoteryNotFound(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "leavebad")

	w := serve("POST", "/subnoteries/:subnotery_id/leave", "/subnoteries/99999/leave",
		nil, app.LeaveSubnotery, authMW(uid))
	assertStatus(t, w, http.StatusNotFound)
}

// ===== GetSubnoteryDetail is_member TESTS =====

func TestGetSubnoteryDetail_IsMember(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "membdetail")
	sub := models.Subnotery{Name: "detail-member-sub"}
	app.DB.Create(&sub)

	var user models.User
	app.DB.First(&user, uid)
	app.DB.Model(&sub).Association("Members").Append(&user)

	w := serve("GET", "/subnoteries/:subnotery_id", "/subnoteries/"+strconv.Itoa(int(sub.ID)),
		nil, app.GetSubnoteryDetail, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["is_member"] != true {
		t.Fatalf("expected is_member=true, got %v", r["is_member"])
	}
}

func TestGetSubnoteryDetail_NotMember(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "nonmembdetail")
	sub := models.Subnotery{Name: "detail-nonmemb-sub"}
	app.DB.Create(&sub)

	w := serve("GET", "/subnoteries/:subnotery_id", "/subnoteries/"+strconv.Itoa(int(sub.ID)),
		nil, app.GetSubnoteryDetail, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["is_member"] != false {
		t.Fatalf("expected is_member=false, got %v", r["is_member"])
	}
}
