// notification_test.go — Tests for notification handlers.
package handlers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/Geetur/Notery/internal/models"
)

// ===== NOTIFICATION HANDLER TESTS =====

func seedSubnotery(t *testing.T, app *App, name string) uint {
	t.Helper()
	sub := models.Subnotery{Name: name}
	if err := app.DB.Create(&sub).Error; err != nil {
		t.Fatalf("seed subnotery: %v", err)
	}
	return sub.ID
}

func seedNotification(t *testing.T, app *App, userID uint64, ntype models.NotificationType, title string) uint {
	t.Helper()
	n := models.Notification{
		UserID:       userID,
		Type:         ntype,
		Title:        title,
		Message:      "Test notification",
		ActionStatus: models.NotifPending,
	}
	if err := app.DB.Create(&n).Error; err != nil {
		t.Fatalf("seed notification: %v", err)
	}
	return n.ID
}

func seedAdminInviteNotification(t *testing.T, app *App, userID uint64, subnoteryID uint, inviterID uint64) uint {
	t.Helper()
	n := models.Notification{
		UserID:        userID,
		Type:          models.NotifAdminInvite,
		Title:         "Admin invite for n/test",
		Message:       "You've been invited to become an admin",
		ReferenceID:   uint64(subnoteryID),
		ReferenceType: "subnotery",
		ActionStatus:  models.NotifPending,
		ActorID:       inviterID,
	}
	if err := app.DB.Create(&n).Error; err != nil {
		t.Fatalf("seed admin invite notification: %v", err)
	}
	return n.ID
}

func TestGetNotifications_Empty(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	uid := seedUser(t, app.DB, "notif_empty")

	w := serve("GET", "/notifications", "/notifications",
		nil, app.GetNotifications, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	notifs := r["notifications"].([]interface{})
	if len(notifs) != 0 {
		t.Fatalf("expected empty notifications, got %d", len(notifs))
	}
}

func TestGetNotifications_WithData(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	uid := seedUser(t, app.DB, "notif_data")
	seedNotification(t, app, uid, models.NotifUpvoteMilestone, "Your post hit 10 upvotes!")
	seedNotification(t, app, uid, models.NotifUpvoteMilestone, "Your post hit 25 upvotes!")

	w := serve("GET", "/notifications", "/notifications",
		nil, app.GetNotifications, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	notifs := r["notifications"].([]interface{})
	if len(notifs) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(notifs))
	}
}

func TestGetNotifications_OtherUserCantSee(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	uid1 := seedUser(t, app.DB, "notif_user1")
	uid2 := seedUser(t, app.DB, "notif_user2")
	seedNotification(t, app, uid1, models.NotifUpvoteMilestone, "Private notif")

	w := serve("GET", "/notifications", "/notifications",
		nil, app.GetNotifications, authMW(uid2))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	notifs := r["notifications"].([]interface{})
	if len(notifs) != 0 {
		t.Fatalf("expected 0 notifications for other user, got %d", len(notifs))
	}
}

func TestGetUnreadCount_AllUnread(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	uid := seedUser(t, app.DB, "unread_all")
	seedNotification(t, app, uid, models.NotifUpvoteMilestone, "Notif 1")
	seedNotification(t, app, uid, models.NotifUpvoteMilestone, "Notif 2")

	w := serve("GET", "/notifications/unread-count", "/notifications/unread-count",
		nil, app.GetUnreadCount, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["unread_count"].(float64) != 2 {
		t.Fatalf("expected unread_count=2, got %v", r["unread_count"])
	}
}

func TestMarkNotificationRead(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	uid := seedUser(t, app.DB, "mark_read")
	nid := seedNotification(t, app, uid, models.NotifUpvoteMilestone, "Read me")

	w := serve("PATCH", "/notifications/:id/read", fmt.Sprintf("/notifications/%d/read", nid),
		nil, app.MarkNotificationRead, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Unread count should now be 0
	w2 := serve("GET", "/notifications/unread-count", "/notifications/unread-count",
		nil, app.GetUnreadCount, authMW(uid))
	r := respJSON(t, w2)
	if r["unread_count"].(float64) != 0 {
		t.Fatalf("expected unread_count=0 after marking read, got %v", r["unread_count"])
	}
}

func TestMarkNotificationRead_OtherUser(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	uid1 := seedUser(t, app.DB, "read_owner")
	uid2 := seedUser(t, app.DB, "read_other")
	nid := seedNotification(t, app, uid1, models.NotifUpvoteMilestone, "Not yours")

	w := serve("PATCH", "/notifications/:id/read", fmt.Sprintf("/notifications/%d/read", nid),
		nil, app.MarkNotificationRead, authMW(uid2))
	assertStatus(t, w, http.StatusNotFound)
}

func TestMarkAllNotificationsRead(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	uid := seedUser(t, app.DB, "readall")
	seedNotification(t, app, uid, models.NotifUpvoteMilestone, "N1")
	seedNotification(t, app, uid, models.NotifUpvoteMilestone, "N2")
	seedNotification(t, app, uid, models.NotifUpvoteMilestone, "N3")

	w := serve("POST", "/notifications/read-all", "/notifications/read-all",
		nil, app.MarkAllNotificationsRead, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	w2 := serve("GET", "/notifications/unread-count", "/notifications/unread-count",
		nil, app.GetUnreadCount, authMW(uid))
	r := respJSON(t, w2)
	if r["unread_count"].(float64) != 0 {
		t.Fatalf("expected all read, got %v unread", r["unread_count"])
	}
}

func TestAcceptAdminInvite_HappyPath(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	uid := seedUser(t, app.DB, "invite_accept")
	inviter := seedUser(t, app.DB, "invite_admin")
	subID := seedSubnotery(t, app, "invite-sub")

	nid := seedAdminInviteNotification(t, app, uid, subID, inviter)

	w := serve("POST", "/notifications/:id/accept", fmt.Sprintf("/notifications/%d/accept", nid),
		nil, app.AcceptNotification, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Verify user is now admin
	var count int64
	app.DB.Table("user_admins").Where("user_id = ? AND subnotery_id = ?", uid, subID).Count(&count)
	if count != 1 {
		t.Fatalf("expected user to be admin after accepting, got count=%d", count)
	}

	// Verify notification status is accepted
	var notif models.Notification
	app.DB.First(&notif, nid)
	if notif.ActionStatus != models.NotifAccepted {
		t.Fatalf("expected status accepted, got %s", notif.ActionStatus)
	}
}

func TestDenyAdminInvite(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	uid := seedUser(t, app.DB, "invite_deny")
	inviter := seedUser(t, app.DB, "invite_deny_admin")
	subID := seedSubnotery(t, app, "deny-sub")

	nid := seedAdminInviteNotification(t, app, uid, subID, inviter)

	w := serve("POST", "/notifications/:id/deny", fmt.Sprintf("/notifications/%d/deny", nid),
		nil, app.DenyNotification, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Verify user is NOT admin
	var count int64
	app.DB.Table("user_admins").Where("user_id = ? AND subnotery_id = ?", uid, subID).Count(&count)
	if count != 0 {
		t.Fatalf("expected user to NOT be admin after denying, got count=%d", count)
	}

	// Verify notification status is denied
	var notif models.Notification
	app.DB.First(&notif, nid)
	if notif.ActionStatus != models.NotifDenied {
		t.Fatalf("expected status denied, got %s", notif.ActionStatus)
	}
}

func TestAcceptNotification_AlreadyActioned(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	uid := seedUser(t, app.DB, "invite_double")
	inviter := seedUser(t, app.DB, "invite_double_admin")
	subID := seedSubnotery(t, app, "double-sub")

	nid := seedAdminInviteNotification(t, app, uid, subID, inviter)

	// Accept first time
	serve("POST", "/notifications/:id/accept", fmt.Sprintf("/notifications/%d/accept", nid),
		nil, app.AcceptNotification, authMW(uid))

	// Try to accept again
	w := serve("POST", "/notifications/:id/accept", fmt.Sprintf("/notifications/%d/accept", nid),
		nil, app.AcceptNotification, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestInviteAdmin_HappyPath(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	admin := seedUser(t, app.DB, "inv_admin")
	target := seedUser(t, app.DB, "inv_target")
	subID := seedSubnotery(t, app, "inv-sub")

	// Make admin an admin of the subnotery
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", admin, subID)

	body := jsonBody(map[string]string{"username": "inv_target"})
	w := serve("POST", "/subnoteries/:subnotery_id/invite-admin", fmt.Sprintf("/subnoteries/%d/invite-admin", subID),
		body, app.InviteAdmin, authMW(admin))
	assertStatus(t, w, http.StatusOK)

	// Verify notification was created for target
	var count int64
	app.DB.Model(&models.Notification{}).
		Where("user_id = ? AND type = ? AND reference_id = ?", target, models.NotifAdminInvite, subID).
		Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 admin invite notification, got %d", count)
	}
}

func TestInviteAdmin_NotAdmin(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	nonAdmin := seedUser(t, app.DB, "nonadmin")
	seedUser(t, app.DB, "inv_target2")
	subID := seedSubnotery(t, app, "nonadmin-sub")

	body := jsonBody(map[string]string{"username": "inv_target2"})
	w := serve("POST", "/subnoteries/:subnotery_id/invite-admin", fmt.Sprintf("/subnoteries/%d/invite-admin", subID),
		body, app.InviteAdmin, authMW(nonAdmin))
	assertStatus(t, w, http.StatusForbidden)
}

func TestInviteAdmin_UserNotFound(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	admin := seedUser(t, app.DB, "inv_admin3")
	subID := seedSubnotery(t, app, "inv-sub3")
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", admin, subID)

	body := jsonBody(map[string]string{"username": "nonexistent_user"})
	w := serve("POST", "/subnoteries/:subnotery_id/invite-admin", fmt.Sprintf("/subnoteries/%d/invite-admin", subID),
		body, app.InviteAdmin, authMW(admin))
	assertStatus(t, w, http.StatusNotFound)
}

func TestInviteAdmin_InviteSelf(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	admin := seedUser(t, app.DB, "inv_self")
	subID := seedSubnotery(t, app, "inv-self-sub")
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", admin, subID)

	body := jsonBody(map[string]string{"username": "inv_self"})
	w := serve("POST", "/subnoteries/:subnotery_id/invite-admin", fmt.Sprintf("/subnoteries/%d/invite-admin", subID),
		body, app.InviteAdmin, authMW(admin))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestInviteAdmin_AlreadyAdmin(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	admin := seedUser(t, app.DB, "inv_adm_dup")
	target := seedUser(t, app.DB, "inv_tgt_dup")
	subID := seedSubnotery(t, app, "inv-dup-sub")
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", admin, subID)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", target, subID)

	body := jsonBody(map[string]string{"username": "inv_tgt_dup"})
	w := serve("POST", "/subnoteries/:subnotery_id/invite-admin", fmt.Sprintf("/subnoteries/%d/invite-admin", subID),
		body, app.InviteAdmin, authMW(admin))
	assertStatus(t, w, http.StatusConflict)
}

func TestInviteAdmin_DuplicateInvite(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	admin := seedUser(t, app.DB, "inv_dup_adm")
	seedUser(t, app.DB, "inv_dup_tgt")
	subID := seedSubnotery(t, app, "inv-dup2-sub")
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", admin, subID)

	body := jsonBody(map[string]string{"username": "inv_dup_tgt"})
	serve("POST", "/subnoteries/:subnotery_id/invite-admin", fmt.Sprintf("/subnoteries/%d/invite-admin", subID),
		body, app.InviteAdmin, authMW(admin))

	// Second invite should fail
	body2 := jsonBody(map[string]string{"username": "inv_dup_tgt"})
	w := serve("POST", "/subnoteries/:subnotery_id/invite-admin", fmt.Sprintf("/subnoteries/%d/invite-admin", subID),
		body2, app.InviteAdmin, authMW(admin))
	assertStatus(t, w, http.StatusConflict)
}

// ===== REMOVE MEMBER TESTS =====

func TestRemoveMember_HappyPath(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	admin := seedUser(t, app.DB, "rm_admin")
	member := seedUser(t, app.DB, "rm_member")
	subID := seedSubnotery(t, app, "rm-sub")
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", admin, subID)
	app.DB.Exec("INSERT INTO user_memberships (user_id, subnotery_id) VALUES (?, ?)", member, subID)

	w := serve("DELETE", "/subnoteries/:subnotery_id/members/:uid",
		fmt.Sprintf("/subnoteries/%d/members/%d", subID, member),
		nil, app.RemoveMemberFromSubnotery, authMW(admin))
	assertStatus(t, w, http.StatusOK)

	// Verify member is removed
	var count int64
	app.DB.Table("user_memberships").Where("user_id = ? AND subnotery_id = ?", member, subID).Count(&count)
	if count != 0 {
		t.Fatal("expected member to be removed")
	}
}

func TestRemoveMember_NotAdmin(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	nonAdmin := seedUser(t, app.DB, "rm_noadmin")
	member := seedUser(t, app.DB, "rm_member2")
	subID := seedSubnotery(t, app, "rm-sub2")
	app.DB.Exec("INSERT INTO user_memberships (user_id, subnotery_id) VALUES (?, ?)", member, subID)

	w := serve("DELETE", "/subnoteries/:subnotery_id/members/:uid",
		fmt.Sprintf("/subnoteries/%d/members/%d", subID, member),
		nil, app.RemoveMemberFromSubnotery, authMW(nonAdmin))
	assertStatus(t, w, http.StatusForbidden)
}

func TestRemoveMember_CannotRemoveSelf(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	admin := seedUser(t, app.DB, "rm_self")
	subID := seedSubnotery(t, app, "rm-self-sub")
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", admin, subID)
	app.DB.Exec("INSERT INTO user_memberships (user_id, subnotery_id) VALUES (?, ?)", admin, subID)

	w := serve("DELETE", "/subnoteries/:subnotery_id/members/:uid",
		fmt.Sprintf("/subnoteries/%d/members/%d", subID, admin),
		nil, app.RemoveMemberFromSubnotery, authMW(admin))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestRemoveMember_YoungerAdminCannotRemoveOlder(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	olderAdmin := seedUser(t, app.DB, "rm_older")
	youngerAdmin := seedUser(t, app.DB, "rm_younger")
	subID := seedSubnotery(t, app, "rm-seniority")
	// Older admin added first (earlier created_at)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id, created_at) VALUES (?, ?, datetime('now', '-1 hour'))", olderAdmin, subID)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id, created_at) VALUES (?, ?, datetime('now'))", youngerAdmin, subID)
	app.DB.Exec("INSERT INTO user_memberships (user_id, subnotery_id) VALUES (?, ?)", olderAdmin, subID)
	app.DB.Exec("INSERT INTO user_memberships (user_id, subnotery_id) VALUES (?, ?)", youngerAdmin, subID)

	// Younger admin tries to remove older
	w := serve("DELETE", "/subnoteries/:subnotery_id/members/:uid",
		fmt.Sprintf("/subnoteries/%d/members/%d", subID, olderAdmin),
		nil, app.RemoveMemberFromSubnotery, authMW(youngerAdmin))
	assertStatus(t, w, http.StatusForbidden)
}

func TestRemoveMember_OlderAdminCanRemoveYounger(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	olderAdmin := seedUser(t, app.DB, "rm_older2")
	youngerAdmin := seedUser(t, app.DB, "rm_younger2")
	subID := seedSubnotery(t, app, "rm-seniority2")
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id, created_at) VALUES (?, ?, datetime('now', '-1 hour'))", olderAdmin, subID)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id, created_at) VALUES (?, ?, datetime('now'))", youngerAdmin, subID)
	app.DB.Exec("INSERT INTO user_memberships (user_id, subnotery_id) VALUES (?, ?)", olderAdmin, subID)
	app.DB.Exec("INSERT INTO user_memberships (user_id, subnotery_id) VALUES (?, ?)", youngerAdmin, subID)

	// Older admin removes younger — should work, removing both admin and member
	w := serve("DELETE", "/subnoteries/:subnotery_id/members/:uid",
		fmt.Sprintf("/subnoteries/%d/members/%d", subID, youngerAdmin),
		nil, app.RemoveMemberFromSubnotery, authMW(olderAdmin))
	assertStatus(t, w, http.StatusOK)

	// Verify both admin and member status removed
	var adminCount int64
	app.DB.Table("user_admins").Where("user_id = ? AND subnotery_id = ?", youngerAdmin, subID).Count(&adminCount)
	if adminCount != 0 {
		t.Fatal("expected younger admin to be removed from admins")
	}
	var memberCount int64
	app.DB.Table("user_memberships").Where("user_id = ? AND subnotery_id = ?", youngerAdmin, subID).Count(&memberCount)
	if memberCount != 0 {
		t.Fatal("expected younger admin to be removed from members")
	}
}

func TestRemoveMember_MemberNotFound(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	admin := seedUser(t, app.DB, "rm_notfound")
	subID := seedSubnotery(t, app, "rm-notfound")
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", admin, subID)

	w := serve("DELETE", "/subnoteries/:subnotery_id/members/:uid",
		fmt.Sprintf("/subnoteries/%d/members/99999", subID),
		nil, app.RemoveMemberFromSubnotery, authMW(admin))
	assertStatus(t, w, http.StatusNotFound)
}

// ===== GET SUBNOTERY MEMBERS TESTS =====

func TestGetSubnoteryMembers_Empty(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	subID := seedSubnotery(t, app, "members-empty")

	w := serve("GET", "/subnoteries/:subnotery_id/members",
		fmt.Sprintf("/subnoteries/%d/members", subID),
		nil, app.GetSubnoteryMembers)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	members := r["members"].([]interface{})
	if len(members) != 0 {
		t.Fatalf("expected 0 members, got %d", len(members))
	}
}

func TestGetSubnoteryMembers_WithData(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	uid1 := seedUser(t, app.DB, "member1")
	uid2 := seedUser(t, app.DB, "member2")
	subID := seedSubnotery(t, app, "members-data")
	app.DB.Exec("INSERT INTO user_memberships (user_id, subnotery_id) VALUES (?, ?)", uid1, subID)
	app.DB.Exec("INSERT INTO user_memberships (user_id, subnotery_id) VALUES (?, ?)", uid2, subID)
	app.DB.Exec("INSERT INTO user_admins (user_id, subnotery_id) VALUES (?, ?)", uid1, subID)

	w := serve("GET", "/subnoteries/:subnotery_id/members",
		fmt.Sprintf("/subnoteries/%d/members", subID),
		nil, app.GetSubnoteryMembers)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	members := r["members"].([]interface{})
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	// Check that admin status is reported
	foundAdmin := false
	for _, m := range members {
		mb := m.(map[string]interface{})
		if mb["is_admin"] == true {
			foundAdmin = true
		}
	}
	if !foundAdmin {
		t.Fatal("expected at least one admin in members list")
	}
}

// ===== MILESTONE NOTIFICATION TESTS =====

func TestSendUpvoteMilestoneNotification(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	uid := seedUser(t, app.DB, "milestone_user")

	app.SendUpvoteMilestoneNotification(uid, 1, "note", "My great note", 10)

	var count int64
	app.DB.Model(&models.Notification{}).
		Where("user_id = ? AND type = ?", uid, models.NotifUpvoteMilestone).
		Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 milestone notification, got %d", count)
	}
}

func TestSendUpvoteMilestoneNotification_NoDuplicates(t *testing.T) {
	app := testApp(t)
	app.DB.AutoMigrate(&models.Notification{})
	uid := seedUser(t, app.DB, "milestone_nodup")

	app.SendUpvoteMilestoneNotification(uid, 1, "note", "Same note", 10)
	app.SendUpvoteMilestoneNotification(uid, 1, "note", "Same note", 10) // duplicate

	var count int64
	app.DB.Model(&models.Notification{}).
		Where("user_id = ? AND type = ?", uid, models.NotifUpvoteMilestone).
		Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 milestone notification (no dupes), got %d", count)
	}
}
