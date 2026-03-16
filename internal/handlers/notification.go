// notification.go — HTTP handlers for the notification system.
//
// ENDPOINTS:
//
//	GET    /notifications              List notifications for the authenticated user (paginated)
//	GET    /notifications/unread-count Get the count of unread notifications
//	PATCH  /notifications/:id/read     Mark a single notification as read
//	POST   /notifications/read-all     Mark all notifications as read
//	POST   /notifications/:id/accept   Accept an actionable notification (e.g., admin invite)
//	POST   /notifications/:id/deny     Deny an actionable notification (e.g., admin invite)
//
// DESIGN:
//
//	Notifications are user-scoped. Only the recipient can view or act on their
//	notifications. Actionable notifications (admin_invite) modify system state
//	when accepted (e.g., adding user as subnotery admin). The unread-count
//	endpoint powers the notification bell badge in the top nav.
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// notifLog is the domain-specific logger for notification operations.
var notifLog = helpers.NewLogger("NOTIFICATION")

// GetNotifications returns a paginated list of notifications for the authenticated user.
//
// Supports optional ?unread_only=true filter to show only unread notifications.
//
// DB: COUNT + SELECT from notifications WHERE user_id, ordered by created_at DESC.
// Helpers: helpers.GetUserID, helpers.ParsePagination.
//
// Route: GET /api/v1/notifications
func (app *App) GetNotifications(c *gin.Context) {
	notifLog.Log("LIST", "Processing list notifications request")
	userID := helpers.GetUserID(c)
	pag := helpers.ParsePagination(c)

	query := app.DB.Model(&models.Notification{}).Where("user_id = ?", userID)

	if c.Query("unread_only") == "true" {
		query = query.Where("is_read = ?", false)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		notifLog.Log("LIST", "Failed to count notifications", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notifications"})
		return
	}

	var notifications []models.Notification
	if err := query.Order("created_at DESC").
		Offset(pag.Offset).Limit(pag.Limit).
		Find(&notifications).Error; err != nil {
		notifLog.Log("LIST", "Failed to fetch notifications", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notifications"})
		return
	}

	// Populate actor usernames for display
	type notifResponse struct {
		ID            uint                     `json:"id"`
		Type          models.NotificationType  `json:"type"`
		Title         string                   `json:"title"`
		Message       string                   `json:"message"`
		ReferenceID   uint64                   `json:"reference_id"`
		ReferenceType string                   `json:"reference_type"`
		ActionStatus  models.NotificationStatus `json:"action_status"`
		IsRead        bool                     `json:"is_read"`
		ActorID       uint64                   `json:"actor_id"`
		ActorUsername string                   `json:"actor_username"`
		Metadata      string                   `json:"metadata"`
		CreatedAt     string                   `json:"created_at"`
	}

	// Collect unique actor IDs for batch lookup
	actorIDs := make(map[uint64]bool)
	for _, n := range notifications {
		if n.ActorID != 0 {
			actorIDs[n.ActorID] = true
		}
	}
	actorNames := make(map[uint64]string)
	if len(actorIDs) > 0 {
		ids := make([]uint64, 0, len(actorIDs))
		for id := range actorIDs {
			ids = append(ids, id)
		}
		var actors []models.User
		app.DB.Select("id", "username").Where("id IN ?", ids).Find(&actors)
		for _, a := range actors {
			actorNames[uint64(a.ID)] = a.Username
		}
	}

	items := make([]notifResponse, len(notifications))
	for i, n := range notifications {
		items[i] = notifResponse{
			ID:            n.ID,
			Type:          n.Type,
			Title:         n.Title,
			Message:       n.Message,
			ReferenceID:   n.ReferenceID,
			ReferenceType: n.ReferenceType,
			ActionStatus:  n.ActionStatus,
			IsRead:        n.IsRead,
			ActorID:       n.ActorID,
			ActorUsername: actorNames[n.ActorID],
			Metadata:      n.Metadata,
			CreatedAt:     n.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	notifLog.Log("LIST", "Notifications listed", "userID", userID, "count", len(items), "total", total)
	c.JSON(http.StatusOK, gin.H{
		"notifications": items,
		"total":         total,
		"page":          pag.Page,
		"limit":         pag.Limit,
	})
}

// GetUnreadCount returns the number of unread notifications for the authenticated user.
//
// DB: COUNT from notifications WHERE user_id AND is_read = false.
// Helpers: helpers.GetUserID.
//
// Route: GET /api/v1/notifications/unread-count
func (app *App) GetUnreadCount(c *gin.Context) {
	userID := helpers.GetUserID(c)

	var count int64
	if err := app.DB.Model(&models.Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Count(&count).Error; err != nil {
		notifLog.Log("UNREAD", "Failed to count unread", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch unread count"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"unread_count": count})
}

// MarkNotificationRead marks a single notification as read.
//
// Only the notification's recipient can mark it as read.
//
// DB: UPDATE notifications SET is_read = true WHERE id AND user_id.
// Helpers: helpers.GetUserID.
//
// Route: PATCH /api/v1/notifications/:id/read
func (app *App) MarkNotificationRead(c *gin.Context) {
	userID := helpers.GetUserID(c)
	notifID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid notification ID"})
		return
	}

	result := app.DB.Model(&models.Notification{}).
		Where("id = ? AND user_id = ?", notifID, userID).
		Update("is_read", true)
	if result.Error != nil {
		notifLog.Log("READ", "Failed to mark read", "error", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark notification as read"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notification marked as read"})
}

// MarkAllNotificationsRead marks all notifications as read for the authenticated user.
//
// DB: UPDATE notifications SET is_read = true WHERE user_id AND is_read = false.
// Helpers: helpers.GetUserID.
//
// Route: POST /api/v1/notifications/read-all
func (app *App) MarkAllNotificationsRead(c *gin.Context) {
	userID := helpers.GetUserID(c)

	if err := app.DB.Model(&models.Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Update("is_read", true).Error; err != nil {
		notifLog.Log("READ_ALL", "Failed to mark all read", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark notifications as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "All notifications marked as read"})
}

// AcceptNotification handles accepting an actionable notification (e.g., admin invite).
//
// For admin_invite: adds the user as admin of the referenced subnotery and also
// ensures they are a member. Updates notification status to "accepted".
//
// DB: SELECT notification, conditional INSERT into user_admins + user_memberships.
// Helpers: helpers.GetUserID.
//
// Route: POST /api/v1/notifications/:id/accept
func (app *App) AcceptNotification(c *gin.Context) {
	userID := helpers.GetUserID(c)
	notifID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid notification ID"})
		return
	}

	var notif models.Notification
	if err := app.DB.Where("id = ? AND user_id = ?", notifID, userID).First(&notif).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
		return
	}

	if notif.ActionStatus != models.NotifPending {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Notification has already been acted on"})
		return
	}

	switch notif.Type {
	case models.NotifAdminInvite:
		if err := app.acceptAdminInvite(c, &notif, userID); err != nil {
			return // error already sent to client
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "This notification type is not actionable"})
		return
	}

	// Mark notification as accepted and read
	app.DB.Model(&notif).Updates(map[string]interface{}{
		"action_status": models.NotifAccepted,
		"is_read":       true,
	})

	notifLog.Log("ACCEPT", "Notification accepted", "notifID", notifID, "userID", userID, "type", notif.Type)
	c.JSON(http.StatusOK, gin.H{"message": "Notification accepted"})
}

// DenyNotification handles denying an actionable notification (e.g., admin invite).
//
// Simply marks the notification as denied. No system state changes.
//
// DB: UPDATE notification SET action_status = 'denied'.
// Helpers: helpers.GetUserID.
//
// Route: POST /api/v1/notifications/:id/deny
func (app *App) DenyNotification(c *gin.Context) {
	userID := helpers.GetUserID(c)
	notifID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid notification ID"})
		return
	}

	var notif models.Notification
	if err := app.DB.Where("id = ? AND user_id = ?", notifID, userID).First(&notif).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
		return
	}

	if notif.ActionStatus != models.NotifPending {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Notification has already been acted on"})
		return
	}

	app.DB.Model(&notif).Updates(map[string]interface{}{
		"action_status": models.NotifDenied,
		"is_read":       true,
	})

	notifLog.Log("DENY", "Notification denied", "notifID", notifID, "userID", userID, "type", notif.Type)
	c.JSON(http.StatusOK, gin.H{"message": "Notification denied"})
}

// acceptAdminInvite processes an accepted admin invitation.
// Adds the user as admin + member of the referenced subnotery.
func (app *App) acceptAdminInvite(c *gin.Context, notif *models.Notification, userID uint64) error {
	subnoteryID := notif.ReferenceID

	var subnotery models.Subnotery
	if err := app.DB.First(&subnotery, subnoteryID).Error; err != nil {
		notifLog.Log("ACCEPT_INVITE", "Subnotery not found", "subnoteryID", subnoteryID)
		c.JSON(http.StatusNotFound, gin.H{"error": "Community no longer exists"})
		return err
	}

	var user models.User
	if err := app.DB.First(&user, userID).Error; err != nil {
		notifLog.Log("ACCEPT_INVITE", "User not found", "userID", userID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find user"})
		return err
	}

	// Check if already admin (idempotent)
	var adminCount int64
	app.DB.Table("user_admins").Where("user_id = ? AND subnotery_id = ?", userID, subnoteryID).Count(&adminCount)
	if adminCount > 0 {
		notifLog.Log("ACCEPT_INVITE", "User already admin", "userID", userID, "subnoteryID", subnoteryID)
		// Still mark as accepted (not an error)
		return nil
	}

	// Add as admin
	if err := app.DB.Model(&subnotery).Association("Admins").Append(&user); err != nil {
		notifLog.Log("ACCEPT_INVITE", "Failed to add admin", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add you as admin"})
		return err
	}

	// Ensure also a member
	if err := app.DB.Model(&subnotery).Association("Members").Append(&user); err != nil {
		notifLog.Log("ACCEPT_INVITE", "Failed to add member", "error", err)
		// Non-fatal — admin was added successfully
	}

	notifLog.Log("ACCEPT_INVITE", "Admin invite accepted", "userID", userID, "subnoteryID", subnoteryID)
	return nil
}

// SendAdminInviteNotification creates an admin invite notification for a target user.
// Called from the InviteAdmin handler when an admin invites someone.
//
// Checks for duplicate pending invites to the same subnotery before creating.
func (app *App) SendAdminInviteNotification(inviterID, targetUserID, subnoteryID uint64, subnoteryName, inviterUsername string) error {
	// Check for existing pending invite to same subnotery
	var existing int64
	app.DB.Model(&models.Notification{}).
		Where("user_id = ? AND type = ? AND reference_id = ? AND action_status = ?",
			targetUserID, models.NotifAdminInvite, subnoteryID, models.NotifPending).
		Count(&existing)
	if existing > 0 {
		return fmt.Errorf("pending invite already exists")
	}

	notif := models.Notification{
		UserID:        targetUserID,
		Type:          models.NotifAdminInvite,
		Title:         fmt.Sprintf("Admin invite for n/%s", subnoteryName),
		Message:       fmt.Sprintf("%s invited you to become an admin of n/%s", inviterUsername, subnoteryName),
		ReferenceID:   subnoteryID,
		ReferenceType: "subnotery",
		ActionStatus:  models.NotifPending,
		ActorID:       inviterID,
	}

	if err := app.DB.Create(&notif).Error; err != nil {
		notifLog.Log("INVITE", "Failed to create admin invite notification", "error", err)
		return err
	}

	notifLog.Log("INVITE", "Admin invite notification sent",
		"targetUserID", targetUserID, "subnoteryID", subnoteryID, "inviterID", inviterID)
	return nil
}

// SendUpvoteMilestoneNotification creates a milestone notification for a content author.
//
// Parameters:
//   - authorID:  the user who created the content
//   - targetID:  the note or comment ID
//   - targetType: "note" or "comment"
//   - title:     the note title or comment excerpt
//   - milestone: the upvote count milestone reached
func (app *App) SendUpvoteMilestoneNotification(authorID, targetID uint64, targetType, title string, milestone uint64) {
	// Don't create duplicate milestone notifications
	metadata := fmt.Sprintf(`{"milestone":%d,"target":"%s"}`, milestone, targetType)
	var existing int64
	app.DB.Model(&models.Notification{}).
		Where("user_id = ? AND type = ? AND reference_id = ? AND reference_type = ? AND metadata = ?",
			authorID, models.NotifUpvoteMilestone, targetID, targetType, metadata).
		Count(&existing)
	if existing > 0 {
		return
	}

	// Truncate title for display
	displayTitle := title
	if len(displayTitle) > 80 {
		displayTitle = displayTitle[:77] + "..."
	}

	unitName := "post"
	if targetType == "comment" {
		unitName = "comment"
	}

	notif := models.Notification{
		UserID:        authorID,
		Type:          models.NotifUpvoteMilestone,
		Title:         fmt.Sprintf("Your %s hit %d upvotes!", unitName, milestone),
		Message:       fmt.Sprintf(`Your %s "%s" reached %d upvotes. Congrats!`, unitName, displayTitle, milestone),
		ReferenceID:   targetID,
		ReferenceType: targetType,
		ActionStatus:  models.NotifPending,
		Metadata:      metadata,
	}

	if err := app.DB.Create(&notif).Error; err != nil {
		notifLog.Log("MILESTONE", "Failed to create milestone notification", "error", err)
		return
	}

	notifLog.Log("MILESTONE", "Milestone notification sent",
		"authorID", authorID, "targetID", targetID, "targetType", targetType, "milestone", milestone)
}

// InviteAdmin sends an admin invite notification to a user (by username).
//
// Only existing admins of the subnotery (or global admins) can invite.
// The target user receives a notification they can accept/deny.
//
// DB: SELECT subnotery, SELECT user by username, INSERT notification.
// Helpers: helpers.MustParseSubnoteryID, helpers.GetUserID, helpers.BindJSON.
//
// Route: POST /api/v1/subnoteries/:subnotery_id/invite-admin
func (app *App) InviteAdmin(c *gin.Context) {
	notifLog.Log("INVITE_ADMIN", "Processing admin invite request")

	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		return
	}

	inviterID := helpers.GetUserID(c)

	// Verify inviter is admin
	if !app.isSubnoteryAdmin(inviterID, uint(subnoteryID)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only admins can invite new admins"})
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
	}
	if !helpers.BindJSON(c, &req) {
		return
	}

	// Fetch subnotery for name
	var subnotery models.Subnotery
	if err := app.DB.Select("id", "name").First(&subnotery, subnoteryID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Community not found"})
		return
	}

	// Fetch target user by username
	var targetUser models.User
	if err := app.DB.Where("username = ?", req.Username).First(&targetUser).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Cannot invite yourself
	if uint64(targetUser.ID) == inviterID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "You cannot invite yourself"})
		return
	}

	// Check if target is already an admin
	var alreadyAdmin int64
	app.DB.Table("user_admins").
		Where("user_id = ? AND subnotery_id = ?", targetUser.ID, subnoteryID).
		Count(&alreadyAdmin)
	if alreadyAdmin > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "User is already an admin of this community"})
		return
	}

	// Fetch inviter username
	var inviter models.User
	if err := app.DB.Select("id", "username").First(&inviter, inviterID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch inviter info"})
		return
	}

	// Send the invite notification
	if err := app.SendAdminInviteNotification(
		inviterID,
		uint64(targetUser.ID),
		uint64(subnotery.ID),
		subnotery.Name,
		inviter.Username,
	); err != nil {
		if err.Error() == "pending invite already exists" {
			c.JSON(http.StatusConflict, gin.H{"error": "A pending invite already exists for this user"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send invite"})
		return
	}

	notifLog.Log("INVITE_ADMIN", "Admin invite sent",
		"subnoteryID", subnoteryID, "targetUser", req.Username, "inviterID", inviterID)
	c.JSON(http.StatusOK, gin.H{"message": "Admin invite sent successfully"})
}

// RemoveMemberFromSubnotery removes a member from a subnotery (admin action).
//
// Admins can remove regular members. Younger admins cannot remove older admins.
// Global admins can remove anyone except other global admins.
// Removing an admin also strips their admin status.
//
// DB: SELECT user_admins for seniority check, DELETE from user_memberships + user_admins.
// Helpers: helpers.MustParseSubnoteryID, helpers.GetUserID.
//
// Route: DELETE /api/v1/subnoteries/:subnotery_id/members/:uid
func (app *App) RemoveMemberFromSubnotery(c *gin.Context) {
	subnoteryLog.Log("REMOVE_MEMBER", "Processing remove member request")

	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		return
	}

	targetUID, err := strconv.ParseUint(c.Param("uid"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	requesterID := helpers.GetUserID(c)

	// Cannot remove yourself — use the leave endpoint
	if requesterID == targetUID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot remove yourself — use the leave endpoint instead"})
		return
	}

	// Verify requester is admin
	var requester models.User
	if err := app.DB.Select("id", "is_global_admin").First(&requester, requesterID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user"})
		return
	}

	isRequesterAdmin := requester.IsGlobalAdmin
	if !isRequesterAdmin {
		var adminCount int64
		app.DB.Table("user_admins").
			Where("user_id = ? AND subnotery_id = ?", requesterID, subnoteryID).
			Count(&adminCount)
		isRequesterAdmin = adminCount > 0
	}
	if !isRequesterAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only admins can remove members"})
		return
	}

	// Check if target is a member
	var memberCount int64
	app.DB.Table("user_memberships").
		Where("user_id = ? AND subnotery_id = ?", targetUID, subnoteryID).
		Count(&memberCount)
	if memberCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "User is not a member of this community"})
		return
	}

	// If target is also an admin, check seniority (older admin cannot be removed by younger)
	var targetAdminRow models.UserAdmin
	app.DB.Table("user_admins").
		Where("user_id = ? AND subnotery_id = ?", targetUID, subnoteryID).
		First(&targetAdminRow)

	if targetAdminRow.UserID != 0 && !requester.IsGlobalAdmin {
		var requesterAdminRow models.UserAdmin
		app.DB.Table("user_admins").
			Where("user_id = ? AND subnotery_id = ?", requesterID, subnoteryID).
			First(&requesterAdminRow)

		// Older admin (earlier created_at) can remove newer admin (later created_at).
		// If they were added at the same time (legacy backfill), neither can remove the other.
		if !requesterAdminRow.CreatedAt.Before(targetAdminRow.CreatedAt) {
			c.JSON(http.StatusForbidden, gin.H{"error": "You cannot remove admins who were added before you"})
			return
		}

		// Remove target from admins too
		var target models.User
		if err := app.DB.First(&target, targetUID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch target user"})
			return
		}
		var sub models.Subnotery
		if err := app.DB.First(&sub, subnoteryID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Subnotery not found"})
			return
		}
		if err := app.DB.Model(&sub).Association("Admins").Delete(&target); err != nil {
			subnoteryLog.Log("REMOVE_MEMBER", "Failed to remove admin role", "error", err)
		}
	}

	// Remove from members
	var target models.User
	if err := app.DB.First(&target, targetUID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch target user"})
		return
	}
	var sub models.Subnotery
	if err := app.DB.First(&sub, subnoteryID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subnotery not found"})
		return
	}
	if err := app.DB.Model(&sub).Association("Members").Delete(&target); err != nil {
		subnoteryLog.Log("REMOVE_MEMBER", "Failed to remove member", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove member"})
		return
	}

	subnoteryLog.Log("REMOVE_MEMBER", "Member removed",
		"subnoteryID", subnoteryID, "targetUserID", targetUID, "requesterID", requesterID)
	c.JSON(http.StatusOK, gin.H{"message": "Member removed successfully"})
}

// GetSubnoteryMembers returns a paginated list of members for a subnotery.
//
// Returns member ID, username, and whether they are an admin.
// Publicly visible — no auth required.
//
// DB: SELECT from user_memberships JOIN users, COUNT for pagination.
// Helpers: helpers.MustParseSubnoteryID, helpers.ParsePagination.
//
// Route: GET /api/v1/subnoteries/:subnotery_id/members
func (app *App) GetSubnoteryMembers(c *gin.Context) {
	subnoteryLog.Log("MEMBERS", "Processing list members request")

	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		return
	}
	pag := helpers.ParsePagination(c)

	// Get total member count
	var total int64
	app.DB.Table("user_memberships").Where("subnotery_id = ?", subnoteryID).Count(&total)

	// Get member IDs with pagination
	type membershipRow struct {
		UserID uint64
	}
	var memberRows []membershipRow
	app.DB.Table("user_memberships").
		Select("user_id").
		Where("subnotery_id = ?", subnoteryID).
		Order("user_id ASC").
		Offset(pag.Offset).Limit(pag.Limit).
		Scan(&memberRows)

	if len(memberRows) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"members": []interface{}{},
			"total":   total,
			"page":    pag.Page,
			"limit":   pag.Limit,
		})
		return
	}

	memberIDs := make([]uint64, len(memberRows))
	for i, r := range memberRows {
		memberIDs[i] = r.UserID
	}

	// Fetch user info
	var users []models.User
	app.DB.Select("id", "username", "avatar_url").Where("id IN ?", memberIDs).Find(&users)

	// Get admin IDs for this subnotery
	type adminRow struct {
		UserID uint64
	}
	var adminRows []adminRow
	app.DB.Table("user_admins").Select("user_id").Where("subnotery_id = ?", subnoteryID).Scan(&adminRows)
	adminSet := make(map[uint64]bool)
	for _, a := range adminRows {
		adminSet[a.UserID] = true
	}

	// Build response
	type memberInfo struct {
		ID        uint   `json:"id"`
		Username  string `json:"username"`
		AvatarURL string `json:"avatar_url"`
		IsAdmin   bool   `json:"is_admin"`
	}
	members := make([]memberInfo, len(users))
	for i, u := range users {
		members[i] = memberInfo{
			ID:        u.ID,
			Username:  u.Username,
			AvatarURL: u.AvatarURL,
			IsAdmin:   adminSet[uint64(u.ID)],
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"members": members,
		"total":   total,
		"page":    pag.Page,
		"limit":   pag.Limit,
	})
}

// NotificationLogger is a helper for vote handlers to lookup note/comment metadata.
// This avoids circular dependencies by keeping notification creation in the handler layer.

// checkAndSendNoteMilestone checks if a note has crossed an upvote milestone after voting.
func (app *App) checkAndSendNoteMilestone(noteID uint, oldUpvotes, newUpvotes uint64, creatorID uint64, noteTitle string) {
	milestone, shouldNotify := models.ShouldNotifyMilestone(oldUpvotes, newUpvotes)
	if !shouldNotify {
		return
	}
	app.SendUpvoteMilestoneNotification(creatorID, uint64(noteID), "note", noteTitle, milestone)
}

// checkAndSendCommentMilestone checks if a comment has crossed an upvote milestone after voting.
func (app *App) checkAndSendCommentMilestone(commentID uint64, oldUpvotes, newUpvotes int64, authorID uint64, commentBody string) {
	if oldUpvotes < 0 {
		oldUpvotes = 0
	}
	if newUpvotes < 0 {
		newUpvotes = 0
	}
	milestone, shouldNotify := models.ShouldNotifyMilestone(uint64(oldUpvotes), uint64(newUpvotes))
	if !shouldNotify {
		return
	}
	// Use first 80 chars of comment as title
	excerpt := commentBody
	if len(excerpt) > 80 {
		excerpt = excerpt[:77] + "..."
	}
	app.SendUpvoteMilestoneNotification(authorID, commentID, "comment", excerpt, milestone)
}

// SendPurchaseNotification notifies the note owner that someone purchased their note.
// Runs asynchronously — caller should invoke via goroutine.
//
// Parameters:
//   - ownerID:    user who owns the note
//   - buyerID:    user who purchased
//   - noteID:     the note that was purchased
//   - noteTitle:  note title for the notification message
//   - buyerName:  buyer username for the notification message
func (app *App) SendPurchaseNotification(ownerID, buyerID uint64, noteID uint, noteTitle, buyerName string) {
	// Don't notify self-purchases
	if ownerID == buyerID {
		return
	}

	displayTitle := noteTitle
	if len(displayTitle) > 80 {
		displayTitle = displayTitle[:77] + "..."
	}

	metadata := fmt.Sprintf(`{"buyer_id":%d}`, buyerID)

	notif := models.Notification{
		UserID:        ownerID,
		Type:          models.NotifPurchase,
		Title:         fmt.Sprintf("%s purchased your note", buyerName),
		Message:       fmt.Sprintf(`%s purchased your note "%s"`, buyerName, displayTitle),
		ReferenceID:   uint64(noteID),
		ReferenceType: "note",
		ActionStatus:  models.NotifPending,
		ActorID:       buyerID,
		Metadata:      metadata,
	}

	if err := app.DB.Create(&notif).Error; err != nil {
		notifLog.Log("PURCHASE", "Failed to create purchase notification", "error", err)
		return
	}
	notifLog.Log("PURCHASE", "Purchase notification sent",
		"ownerID", ownerID, "buyerID", buyerID, "noteID", noteID)
}

// SendCommentNotification notifies the note owner that someone commented on their note.
// Does not notify if the commenter IS the note owner.
//
// Parameters:
//   - ownerID:      user who owns the note
//   - commenterID:  user who posted the comment
//   - noteID:       the note being commented on
//   - commentID:    the new comment ID
//   - noteTitle:    note title for the notification message
//   - commenterName: commenter username
func (app *App) SendCommentNotification(ownerID, commenterID uint64, noteID uint, commentID uint, noteTitle, commenterName string) {
	if ownerID == commenterID {
		return
	}

	displayTitle := noteTitle
	if len(displayTitle) > 80 {
		displayTitle = displayTitle[:77] + "..."
	}

	metadata := fmt.Sprintf(`{"comment_id":%d}`, commentID)

	notif := models.Notification{
		UserID:        ownerID,
		Type:          models.NotifComment,
		Title:         fmt.Sprintf("%s commented on your note", commenterName),
		Message:       fmt.Sprintf(`%s commented on "%s"`, commenterName, displayTitle),
		ReferenceID:   uint64(noteID),
		ReferenceType: "note",
		ActionStatus:  models.NotifPending,
		ActorID:       commenterID,
		Metadata:      metadata,
	}

	if err := app.DB.Create(&notif).Error; err != nil {
		notifLog.Log("COMMENT", "Failed to create comment notification", "error", err)
		return
	}
	notifLog.Log("COMMENT", "Comment notification sent",
		"ownerID", ownerID, "commenterID", commenterID, "noteID", noteID, "commentID", commentID)
}

// SendReplyNotification notifies a commenter that someone replied to their comment.
// Does not notify if the replier IS the parent comment author.
//
// Parameters:
//   - parentAuthorID: user who wrote the parent comment
//   - replierID:      user who wrote the reply
//   - parentCommentID: the parent comment being replied to
//   - replyID:        the new reply comment ID
//   - noteID:         the note the comment thread is on
//   - replierName:    replier username
func (app *App) SendReplyNotification(parentAuthorID, replierID uint64, parentCommentID uint, replyID uint, noteID uint, replierName string) {
	if parentAuthorID == replierID {
		return
	}

	metadata := fmt.Sprintf(`{"reply_id":%d,"note_id":%d}`, replyID, noteID)

	notif := models.Notification{
		UserID:        parentAuthorID,
		Type:          models.NotifReply,
		Title:         fmt.Sprintf("%s replied to your comment", replierName),
		Message:       fmt.Sprintf("%s replied to your comment", replierName),
		ReferenceID:   uint64(parentCommentID),
		ReferenceType: "comment",
		ActionStatus:  models.NotifPending,
		ActorID:       replierID,
		Metadata:      metadata,
	}

	if err := app.DB.Create(&notif).Error; err != nil {
		notifLog.Log("REPLY", "Failed to create reply notification", "error", err)
		return
	}
	notifLog.Log("REPLY", "Reply notification sent",
		"parentAuthorID", parentAuthorID, "replierID", replierID, "parentCommentID", parentCommentID, "replyID", replyID)
}

// notifMetadataJSON is a helper to parse notification metadata JSON safely.
func notifMetadataJSON(metadata string) map[string]interface{} {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(metadata), &result); err != nil {
		return map[string]interface{}{}
	}
	return result
}
