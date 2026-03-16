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

func TestLeaveSubnotery_AdminCanLeaveWithSuccession(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "adminleaver")
	sub := models.Subnotery{Name: "admin-leave-sub"}
	app.DB.Create(&sub)

	var user models.User
	app.DB.First(&user, uid)
	app.DB.Model(&sub).Association("Admins").Append(&user)
	app.DB.Model(&sub).Association("Members").Append(&user)

	// Admin can now leave (with succession handling)
	w := serve("POST", "/subnoteries/:subnotery_id/leave", "/subnoteries/"+strconv.Itoa(int(sub.ID))+"/leave",
		nil, app.LeaveSubnotery, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Verify user is no longer a member or admin
	var memberCount int64
	app.DB.Table("user_memberships").Where("user_id = ? AND subnotery_id = ?", uid, sub.ID).Count(&memberCount)
	if memberCount != 0 {
		t.Fatal("expected user to be removed from memberships")
	}
	var adminCount int64
	app.DB.Table("user_admins").Where("user_id = ? AND subnotery_id = ?", uid, sub.ID).Count(&adminCount)
	if adminCount != 0 {
		t.Fatal("expected user to be removed from admins")
	}
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

// ===== Admin Succession TESTS =====

func TestLeaveSubnotery_AdminSuccession_PromotesOldestMember(t *testing.T) {
	app := testApp(t)
	adminUID := seedUser(t, app.DB, "lastadmin")
	member1UID := seedUser(t, app.DB, "oldmember")
	member2UID := seedUser(t, app.DB, "newmember")

	sub := models.Subnotery{Name: "succession-sub"}
	app.DB.Create(&sub)

	// Set up admin + members
	var admin, member1, member2 models.User
	app.DB.First(&admin, adminUID)
	app.DB.First(&member1, member1UID)
	app.DB.First(&member2, member2UID)
	app.DB.Model(&sub).Association("Admins").Append(&admin)
	app.DB.Model(&sub).Association("Members").Append(&admin)
	app.DB.Model(&sub).Association("Members").Append(&member1)
	app.DB.Model(&sub).Association("Members").Append(&member2)

	// Admin leaves — should promote oldest member (lowest user_id)
	w := serve("POST", "/subnoteries/:subnotery_id/leave", "/subnoteries/"+strconv.Itoa(int(sub.ID))+"/leave",
		nil, app.LeaveSubnotery, authMW(adminUID))
	assertStatus(t, w, http.StatusOK)

	// Verify the oldest member became admin
	var newAdminCount int64
	app.DB.Table("user_admins").
		Where("user_id = ? AND subnotery_id = ?", member1UID, sub.ID).
		Count(&newAdminCount)
	if newAdminCount != 1 {
		t.Fatal("expected oldest member to be promoted to admin")
	}

	// Verify the original admin is no longer admin
	var oldAdminCount int64
	app.DB.Table("user_admins").
		Where("user_id = ? AND subnotery_id = ?", adminUID, sub.ID).
		Count(&oldAdminCount)
	if oldAdminCount != 0 {
		t.Fatal("expected leaving admin to be removed from admins")
	}
}

func TestLeaveSubnotery_AdminSuccessionNoMembers(t *testing.T) {
	app := testApp(t)
	adminUID := seedUser(t, app.DB, "lonelyadmin")

	sub := models.Subnotery{Name: "empty-succession-sub"}
	app.DB.Create(&sub)

	var admin models.User
	app.DB.First(&admin, adminUID)
	app.DB.Model(&sub).Association("Admins").Append(&admin)
	app.DB.Model(&sub).Association("Members").Append(&admin)

	// Admin leaves — no other members to promote
	w := serve("POST", "/subnoteries/:subnotery_id/leave", "/subnoteries/"+strconv.Itoa(int(sub.ID))+"/leave",
		nil, app.LeaveSubnotery, authMW(adminUID))
	assertStatus(t, w, http.StatusOK)

	// Verify no admins remain
	var adminCount int64
	app.DB.Table("user_admins").Where("subnotery_id = ?", sub.ID).Count(&adminCount)
	if adminCount != 0 {
		t.Fatal("expected no admins after sole admin leaves with no other members")
	}
}

// ===== JoinSubnotery Auto-Promote TESTS =====

func TestJoinSubnotery_AutoPromoteWhenNoAdmins(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "autoadmin")

	sub := models.Subnotery{Name: "no-admin-sub"}
	app.DB.Create(&sub)
	// No admins exist for this subnotery

	w := serve("POST", "/subnoteries/:subnotery_id/join", "/subnoteries/"+strconv.Itoa(int(sub.ID))+"/join",
		nil, app.JoinSubnotery, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Verify user was auto-promoted to admin
	var adminCount int64
	app.DB.Table("user_admins").
		Where("user_id = ? AND subnotery_id = ?", uid, sub.ID).
		Count(&adminCount)
	if adminCount != 1 {
		t.Fatal("expected first joiner to be auto-promoted to admin")
	}
}

func TestJoinSubnotery_NoAutoPromoteWhenAdminExists(t *testing.T) {
	app := testApp(t)
	existingAdmin := seedUser(t, app.DB, "existadmin")
	uid := seedUser(t, app.DB, "regularjoiner")

	sub := models.Subnotery{Name: "has-admin-sub"}
	app.DB.Create(&sub)

	// Existing admin
	var adminUser models.User
	app.DB.First(&adminUser, existingAdmin)
	app.DB.Model(&sub).Association("Admins").Append(&adminUser)

	w := serve("POST", "/subnoteries/:subnotery_id/join", "/subnoteries/"+strconv.Itoa(int(sub.ID))+"/join",
		nil, app.JoinSubnotery, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Verify user was NOT promoted to admin
	var adminCount int64
	app.DB.Table("user_admins").
		Where("user_id = ? AND subnotery_id = ?", uid, sub.ID).
		Count(&adminCount)
	if adminCount != 0 {
		t.Fatal("expected regular joiner NOT to become admin when admin already exists")
	}
}

// ===== RemoveAdminFromSubnotery TESTS =====

func TestRemoveAdmin_OlderRemovesYounger(t *testing.T) {
	app := testApp(t)
	olderUID := seedUser(t, app.DB, "olderadmin")
	youngerUID := seedUser(t, app.DB, "youngeradmin")

	sub := models.Subnotery{Name: "remove-admin-sub"}
	app.DB.Create(&sub)

	// Insert older admin first (earlier created_at)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id, created_at) VALUES (?, ?, datetime('now', '-1 hour'))", olderUID, sub.ID)
	// Insert younger admin second (later created_at)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id, created_at) VALUES (?, ?, datetime('now'))", youngerUID, sub.ID)

	w := serve("DELETE", "/subnoteries/:subnotery_id/admins/:uid",
		fmt.Sprintf("/subnoteries/%d/admins/%d", sub.ID, youngerUID),
		nil, app.RemoveAdminFromSubnotery, authMW(olderUID))
	assertStatus(t, w, http.StatusOK)

	// Verify younger admin was removed
	var count int64
	app.DB.Table("user_admins").
		Where("user_id = ? AND subnotery_id = ?", youngerUID, sub.ID).
		Count(&count)
	if count != 0 {
		t.Fatal("expected younger admin to be removed")
	}
}

func TestRemoveAdmin_YoungerCannotRemoveOlder(t *testing.T) {
	app := testApp(t)
	olderUID := seedUser(t, app.DB, "olderadmin2")
	youngerUID := seedUser(t, app.DB, "youngeradmin2")

	sub := models.Subnotery{Name: "remove-admin-sub2"}
	app.DB.Create(&sub)

	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id, created_at) VALUES (?, ?, datetime('now', '-1 hour'))", olderUID, sub.ID)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id, created_at) VALUES (?, ?, datetime('now'))", youngerUID, sub.ID)

	w := serve("DELETE", "/subnoteries/:subnotery_id/admins/:uid",
		fmt.Sprintf("/subnoteries/%d/admins/%d", sub.ID, olderUID),
		nil, app.RemoveAdminFromSubnotery, authMW(youngerUID))
	assertStatus(t, w, http.StatusForbidden)
}

func TestRemoveAdmin_CannotRemoveSelf(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "selfremove")

	sub := models.Subnotery{Name: "selfremove-sub"}
	app.DB.Create(&sub)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", uid, sub.ID)

	w := serve("DELETE", "/subnoteries/:subnotery_id/admins/:uid",
		fmt.Sprintf("/subnoteries/%d/admins/%d", sub.ID, uid),
		nil, app.RemoveAdminFromSubnotery, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestRemoveAdmin_NonAdminRequester(t *testing.T) {
	app := testApp(t)
	nonAdmin := seedUser(t, app.DB, "nonadmin")
	targetAdmin := seedUser(t, app.DB, "targetadmin")

	sub := models.Subnotery{Name: "nonadmin-remove-sub"}
	app.DB.Create(&sub)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", targetAdmin, sub.ID)

	w := serve("DELETE", "/subnoteries/:subnotery_id/admins/:uid",
		fmt.Sprintf("/subnoteries/%d/admins/%d", sub.ID, targetAdmin),
		nil, app.RemoveAdminFromSubnotery, authMW(nonAdmin))
	assertStatus(t, w, http.StatusForbidden)
}

func TestRemoveAdmin_GlobalAdminCanRemoveAnyone(t *testing.T) {
	app := testApp(t)
	globalAdmin := seedUser(t, app.DB, "globalrem")
	targetAdmin := seedUser(t, app.DB, "targetrem")

	app.DB.Model(&models.User{}).Where("id = ?", globalAdmin).Update("is_global_admin", true)

	sub := models.Subnotery{Name: "global-remove-sub"}
	app.DB.Create(&sub)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", targetAdmin, sub.ID)

	w := serve("DELETE", "/subnoteries/:subnotery_id/admins/:uid",
		fmt.Sprintf("/subnoteries/%d/admins/%d", sub.ID, targetAdmin),
		nil, app.RemoveAdminFromSubnotery, authMW(globalAdmin))
	assertStatus(t, w, http.StatusOK)

	var count int64
	app.DB.Table("user_admins").
		Where("user_id = ? AND subnotery_id = ?", targetAdmin, sub.ID).
		Count(&count)
	if count != 0 {
		t.Fatal("expected global admin to remove target admin")
	}
}

func TestRemoveAdmin_TargetNotAdmin(t *testing.T) {
	app := testApp(t)
	adminUID := seedUser(t, app.DB, "removeradmin")
	regularUID := seedUser(t, app.DB, "regularuser")

	sub := models.Subnotery{Name: "notadmin-remove-sub"}
	app.DB.Create(&sub)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", adminUID, sub.ID)

	w := serve("DELETE", "/subnoteries/:subnotery_id/admins/:uid",
		fmt.Sprintf("/subnoteries/%d/admins/%d", sub.ID, regularUID),
		nil, app.RemoveAdminFromSubnotery, authMW(adminUID))
	assertStatus(t, w, http.StatusNotFound)
}

func TestRemoveAdmin_InvalidUserID(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "invaliduidremove")

	sub := models.Subnotery{Name: "invaliduid-sub"}
	app.DB.Create(&sub)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", uid, sub.ID)

	w := serve("DELETE", "/subnoteries/:subnotery_id/admins/:uid",
		"/subnoteries/"+strconv.Itoa(int(sub.ID))+"/admins/abc",
		nil, app.RemoveAdminFromSubnotery, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

// ===== Banner (DB-only) TESTS =====

func TestDeleteSubnoteryBanner_HappyPath(t *testing.T) {
	app := testApp(t)
	adminUID := seedUser(t, app.DB, "banneradmin")

	sub := models.Subnotery{Name: "banner-sub", BannerURL: "banners/1/banner.jpg"}
	app.DB.Create(&sub)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", adminUID, sub.ID)

	w := serve("DELETE", "/subnoteries/:subnotery_id/banner",
		"/subnoteries/"+strconv.Itoa(int(sub.ID))+"/banner",
		nil, app.DeleteSubnoteryBanner, authMW(adminUID))
	assertStatus(t, w, http.StatusOK)

	// Verify banner_url is cleared
	var updated models.Subnotery
	app.DB.First(&updated, sub.ID)
	if updated.BannerURL != "" {
		t.Fatalf("expected banner_url to be empty, got %s", updated.BannerURL)
	}
}

func TestDeleteSubnoteryBanner_NonAdminDenied(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "bannernonadmin")

	sub := models.Subnotery{Name: "banner-deny-sub", BannerURL: "banners/1/banner.jpg"}
	app.DB.Create(&sub)

	w := serve("DELETE", "/subnoteries/:subnotery_id/banner",
		"/subnoteries/"+strconv.Itoa(int(sub.ID))+"/banner",
		nil, app.DeleteSubnoteryBanner, authMW(uid))
	assertStatus(t, w, http.StatusForbidden)
}

func TestUploadSubnoteryBanner_NoR2(t *testing.T) {
	app := testApp(t)
	adminUID := seedUser(t, app.DB, "banneruploader")

	sub := models.Subnotery{Name: "banner-upload-sub"}
	app.DB.Create(&sub)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", adminUID, sub.ID)

	// With no R2 configured, should return 503
	w := serve("POST", "/subnoteries/:subnotery_id/banner",
		"/subnoteries/"+strconv.Itoa(int(sub.ID))+"/banner",
		nil, app.UploadSubnoteryBanner, authMW(adminUID))
	assertStatus(t, w, http.StatusServiceUnavailable)
}

func TestUploadSubnoteryBanner_NonAdminDenied(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "bannerupnonadmin")

	sub := models.Subnotery{Name: "banner-up-deny-sub"}
	app.DB.Create(&sub)

	w := serve("POST", "/subnoteries/:subnotery_id/banner",
		"/subnoteries/"+strconv.Itoa(int(sub.ID))+"/banner",
		nil, app.UploadSubnoteryBanner, authMW(uid))
	assertStatus(t, w, http.StatusForbidden)
}

func TestGetSubnoteryBanner_NoBanner(t *testing.T) {
	app := testApp(t)

	sub := models.Subnotery{Name: "no-banner-sub"}
	app.DB.Create(&sub)

	w := serve("GET", "/subnoteries/:subnotery_id/banner",
		"/subnoteries/"+strconv.Itoa(int(sub.ID))+"/banner",
		nil, app.GetSubnoteryBanner)
	assertStatus(t, w, http.StatusNotFound)
}

func TestGetSubnoteryBanner_SubnoteryNotFound(t *testing.T) {
	app := testApp(t)

	w := serve("GET", "/subnoteries/:subnotery_id/banner",
		"/subnoteries/99999/banner",
		nil, app.GetSubnoteryBanner)
	assertStatus(t, w, http.StatusNotFound)
}

// ===== detectImageType TESTS =====

func TestDetectImageType_JPEG(t *testing.T) {
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	if ext := detectImageType(data); ext != "jpg" {
		t.Fatalf("expected jpg, got %s", ext)
	}
}

func TestDetectImageType_PNG(t *testing.T) {
	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A}
	if ext := detectImageType(data); ext != "png" {
		t.Fatalf("expected png, got %s", ext)
	}
}

func TestDetectImageType_GIF(t *testing.T) {
	data := []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61}
	if ext := detectImageType(data); ext != "gif" {
		t.Fatalf("expected gif, got %s", ext)
	}
}

func TestDetectImageType_WebP(t *testing.T) {
	data := []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50}
	if ext := detectImageType(data); ext != "webp" {
		t.Fatalf("expected webp, got %s", ext)
	}
}

func TestDetectImageType_Unknown(t *testing.T) {
	data := []byte{0x00, 0x01, 0x02, 0x03}
	if ext := detectImageType(data); ext != "" {
		t.Fatalf("expected empty string for unknown type, got %s", ext)
	}
}

func TestDetectImageType_TooShort(t *testing.T) {
	data := []byte{0xFF, 0xD8}
	if ext := detectImageType(data); ext != "" {
		t.Fatalf("expected empty string for too-short data, got %s", ext)
	}
}

// ===== isSubnoteryAdmin TESTS =====

func TestIsSubnoteryAdmin_SubnoteryAdmin(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "isadmintest")
	sub := models.Subnotery{Name: "isadmin-sub"}
	app.DB.Create(&sub)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", uid, sub.ID)

	if !app.isSubnoteryAdmin(uid, sub.ID) {
		t.Fatal("expected user to be detected as subnotery admin")
	}
}

func TestIsSubnoteryAdmin_GlobalAdmin(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "globalcheck")
	app.DB.Model(&models.User{}).Where("id = ?", uid).Update("is_global_admin", true)

	sub := models.Subnotery{Name: "global-check-sub"}
	app.DB.Create(&sub)
	// NOT added to user_admins — should still be admin via global flag

	if !app.isSubnoteryAdmin(uid, sub.ID) {
		t.Fatal("expected global admin to be detected as admin")
	}
}

func TestIsSubnoteryAdmin_RegularUser(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "regularcheck")

	sub := models.Subnotery{Name: "regular-check-sub"}
	app.DB.Create(&sub)

	if app.isSubnoteryAdmin(uid, sub.ID) {
		t.Fatal("expected regular user NOT to be detected as admin")
	}
}

// ===== GetSubnoteryDetail banner_url TESTS =====

func TestGetSubnoteryDetail_IncludesBannerURL(t *testing.T) {
	app := testApp(t)

	sub := models.Subnotery{Name: "banner-detail-sub", BannerURL: "banners/1/banner.png"}
	app.DB.Create(&sub)

	w := serve("GET", "/subnoteries/:subnotery_id", "/subnoteries/"+strconv.Itoa(int(sub.ID)),
		nil, app.GetSubnoteryDetail)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["banner_url"] != "banners/1/banner.png" {
		t.Fatalf("expected banner_url=banners/1/banner.png, got %v", r["banner_url"])
	}
}
