// subnotery.go — HTTP handlers for subnotery management (browse, join, leave, admin).
//
// ENDPOINTS:
//
//	GET    /subnoteries                           List all subnoteries (paginated)
//	GET    /subnoteries/:subnotery_id             Get a single subnotery with admin/member info
//	GET    /subnoteries/:subnotery_id/notes       List approved notes in a subnotery (paginated)
//	POST   /subnoteries/:subnotery_id/join        Join a subnotery as a member
//	POST   /subnoteries/:subnotery_id/leave       Leave a subnotery (admin succession if last admin)
//	POST   /subnoteries/:subnotery_id/admins      Grant admin privileges to a user
//	DELETE /subnoteries/:subnotery_id/admins/:uid  Remove admin (older admins can remove younger)
//	POST   /subnoteries/:subnotery_id/banner      Upload community banner image
//	DELETE /subnoteries/:subnotery_id/banner       Delete community banner image
//	GET    /subnoteries/:subnotery_id/banner       Proxy community banner image (public, cached)
//
// DESIGN:
//
//	Subnoteries are community containers for notes. Users join as members;
//	admins are promoted by existing admins. Membership and admin status are
//	stored via GORM many-to-many associations (user_memberships, user_admins).
//	Auto-creation of subnoteries happens in the note creation flow (note.go),
//	not here.
//
//	Admin hierarchy: older admins (by join-table row creation time) can remove
//	younger admins. When the last admin leaves, the oldest member is promoted.
//	If no members remain, the next person to join becomes admin automatically.
package handlers

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// subnoteryLog is the domain-specific logger for subnotery operations.
var subnoteryLog = helpers.SubnoteryLog

// AddAdminToSubnotery grants admin privileges to a user for a specific subnotery.
//
// Looks up the subnotery by URL param ID and the target user by email from
// the request body. Appends the user to the subnotery's Admins association.
//
// DB: SELECT from subnoteries, SELECT from users (by email), INSERT into user_admins (GORM association).
// Technologies: PostgreSQL (GORM many-to-many association append).
// Helpers: helpers.MustParseSubnoteryID, helpers.BindJSON, helpers.FetchSubnotery, helpers.FetchUserByEmail.
//
// Route: POST /api/v1/subnoteries/:subnotery_id/admins
func (app *App) AddAdminToSubnotery(c *gin.Context) {
	subnoteryLog.Log("ADD_ADMIN", "Processing add admin request")

	// Parse subnotery ID from URL
	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		subnoteryLog.Log("ADD_ADMIN", "Invalid subnotery ID in URL")
		return
	}

	// Bind and validate request body
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if !helpers.BindJSON(c, &req) {
		subnoteryLog.Log("ADD_ADMIN", "Failed to bind JSON request")
		return
	}
	subnoteryLog.Log("ADD_ADMIN", "Request validated", "subnoteryID", subnoteryID, "email", req.Email)

	// Fetch subnotery from database
	subnotery, ok := helpers.FetchSubnotery(c, app.DB, subnoteryID)
	if !ok {
		subnoteryLog.Log("ADD_ADMIN", "Subnotery not found", "subnoteryID", subnoteryID)
		return
	}

	// Fetch user by email
	user, found := helpers.FetchUserByEmail(app.DB, req.Email)
	if !found {
		subnoteryLog.Log("ADD_ADMIN", "User not found", "email", req.Email)
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	subnoteryLog.Log("ADD_ADMIN", "User found", "userID", user.ID)

	// Add user as admin to subnotery
	if err := app.DB.Model(subnotery).Association("Admins").Append(user); err != nil {
		subnoteryLog.Log("ADD_ADMIN", "Failed to add admin", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add admin to subnotery"})
		return
	}

	subnoteryLog.Log("ADD_ADMIN", "Admin added successfully", "subnoteryID", subnoteryID, "userID", user.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Admin added to subnotery successfully"})
}

// JoinSubnotery adds the authenticated user as a member of the specified subnotery.
//
// Fetches the subnotery by URL param ID and the authenticated user from context.
// Appends the user to the subnotery's Members association. Idempotent — joining
// a subnotery the user is already a member of is a no-op at the DB level.
//
// DB: SELECT from subnoteries, SELECT from users, INSERT into user_memberships (GORM association).
// Technologies: PostgreSQL (GORM many-to-many association append).
// Helpers: helpers.MustParseSubnoteryID, helpers.FetchSubnotery, helpers.GetUserID, helpers.FetchUser.
//
// Route: POST /api/v1/subnoteries/:subnotery_id/join
func (app *App) JoinSubnotery(c *gin.Context) {
	subnoteryLog.Log("JOIN", "Processing join subnotery request")

	// Parse subnotery ID from URL
	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		subnoteryLog.Log("JOIN", "Invalid subnotery ID in URL")
		return
	}

	// Fetch subnotery from database
	subnotery, ok := helpers.FetchSubnotery(c, app.DB, subnoteryID)
	if !ok {
		subnoteryLog.Log("JOIN", "Subnotery not found", "subnoteryID", subnoteryID)
		return
	}

	// Get authenticated user ID
	userID := helpers.GetUserID(c)
	subnoteryLog.Log("JOIN", "User identified", "userID", userID, "subnoteryID", subnoteryID)

	// Fetch user record
	user, ok := helpers.FetchUser(c, app.DB, userID)
	if !ok {
		subnoteryLog.Log("JOIN", "User not found", "userID", userID)
		return
	}

	// Add user as member to subnotery
	if err := app.DB.Model(subnotery).Association("Members").Append(user); err != nil {
		subnoteryLog.Log("JOIN", "Failed to add member", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to join subnotery"})
		return
	}

	// If the subnotery has no admins, promote this user to admin automatically
	var adminCount int64
	app.DB.Table("user_admins").Where("subnotery_id = ?", subnoteryID).Count(&adminCount)
	if adminCount == 0 {
		if err := app.DB.Model(subnotery).Association("Admins").Append(user); err != nil {
			subnoteryLog.Log("JOIN", "Failed to auto-promote to admin", "error", err)
		} else {
			subnoteryLog.Log("JOIN", "Auto-promoted first joiner to admin",
				"userID", userID, "subnoteryID", subnoteryID)
		}
	}

	subnoteryLog.Log("JOIN", "User joined successfully", "userID", userID, "subnoteryID", subnoteryID)
	c.JSON(http.StatusOK, gin.H{"message": "Joined subnotery successfully"})
}

// LeaveSubnotery removes the authenticated user from a subnotery's member list.
//
// Admins cannot leave their own subnotery (they must be removed by another admin
// or global admin). Regular members can leave at any time. Leaving a subnotery
// the user is not a member of is a no-op.
//
// DB: SELECT from subnoteries + users, DELETE from user_memberships (GORM association).
// Technologies: PostgreSQL (GORM many-to-many association delete).
// Helpers: helpers.MustParseSubnoteryID, helpers.FetchSubnotery, helpers.GetUserID, helpers.FetchUser.
//
// Route: POST /api/v1/subnoteries/:subnotery_id/leave
func (app *App) LeaveSubnotery(c *gin.Context) {
	subnoteryLog.Log("LEAVE", "Processing leave subnotery request")

	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		return
	}

	userID := helpers.GetUserID(c)
	subnoteryLog.Log("LEAVE", "User identified", "userID", userID, "subnoteryID", subnoteryID)

	subnotery, ok := helpers.FetchSubnotery(c, app.DB, subnoteryID)
	if !ok {
		return
	}

	user, ok := helpers.FetchUser(c, app.DB, userID)
	if !ok {
		subnoteryLog.Log("LEAVE", "User not found", "userID", userID)
		return
	}

	// Check if the user is an admin
	var isAdmin int64
	app.DB.Table("user_admins").
		Where("user_id = ? AND subnotery_id = ?", userID, subnoteryID).
		Count(&isAdmin)

	if isAdmin > 0 {
		// Admin is leaving — handle succession
		var totalAdmins int64
		app.DB.Table("user_admins").Where("subnotery_id = ?", subnoteryID).Count(&totalAdmins)

		// Remove this user from admins
		if err := app.DB.Model(subnotery).Association("Admins").Delete(user); err != nil {
			subnoteryLog.Log("LEAVE", "Failed to remove admin", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to leave subnotery"})
			return
		}

		if totalAdmins <= 1 {
			// This was the last admin — promote the oldest remaining member
			type memberRow struct {
				UserID uint64
			}
			var oldest memberRow
			err := app.DB.Table("user_memberships").
				Select("user_id").
				Where("subnotery_id = ? AND user_id != ?", subnoteryID, userID).
				Order("user_id ASC").
				Limit(1).
				Scan(&oldest).Error
			if err == nil && oldest.UserID != 0 {
				var newAdmin models.User
				if err := app.DB.First(&newAdmin, oldest.UserID).Error; err == nil {
					app.DB.Model(subnotery).Association("Admins").Append(&newAdmin)
					subnoteryLog.Log("LEAVE", "Promoted oldest member to admin",
						"newAdminID", newAdmin.ID, "subnoteryID", subnoteryID)
				}
			} else {
				subnoteryLog.Log("LEAVE", "No members to promote — subnotery has no admin",
					"subnoteryID", subnoteryID)
			}
		}
	}

	// Remove user from members
	if err := app.DB.Model(subnotery).Association("Members").Delete(user); err != nil {
		subnoteryLog.Log("LEAVE", "Failed to remove member", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to leave subnotery"})
		return
	}

	subnoteryLog.Log("LEAVE", "User left successfully", "userID", userID, "subnoteryID", subnoteryID)
	c.JSON(http.StatusOK, gin.H{"message": "Left subnotery successfully"})
}

// ListSubnoteries returns a paginated list of all subnoteries with member and admin counts.
//
// Public endpoint. Returns subnoteries ordered by creation time descending with
// total count for frontend pagination.
//
// DB: COUNT + SELECT from subnoteries. Paginated with OFFSET/LIMIT.
// Technologies: PostgreSQL (GORM).
// Helpers: helpers.ParsePagination.
//
// Route: GET /api/v1/subnoteries
func (app *App) ListSubnoteries(c *gin.Context) {
	subnoteryLog.Log("LIST", "Processing list subnoteries request")
	pag := helpers.ParsePagination(c)

	var total int64
	if err := app.DB.Model(&models.Subnotery{}).Count(&total).Error; err != nil {
		subnoteryLog.Log("LIST", "Failed to count subnoteries", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subnoteries"})
		return
	}

	var subnoteries []models.Subnotery
	if err := app.DB.Preload("Admins").Preload("Members").
		Offset(pag.Offset).Limit(pag.Limit).
		Order("created_at DESC").
		Find(&subnoteries).Error; err != nil {
		subnoteryLog.Log("LIST", "Failed to fetch subnoteries", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subnoteries"})
		return
	}

	// Build response with counts to avoid exposing full user lists
	type subnoteryItem struct {
		ID           uint   `json:"id"`
		Name         string `json:"name"`
		AdminCount   int    `json:"admin_count"`
		MemberCount  int    `json:"member_count"`
		CreatedAt    string `json:"created_at"`
	}
	items := make([]subnoteryItem, len(subnoteries))
	for i, s := range subnoteries {
		items[i] = subnoteryItem{
			ID:          s.ID,
			Name:        s.Name,
			AdminCount:  len(s.Admins),
			MemberCount: len(s.Members),
			CreatedAt:   s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	subnoteryLog.Log("LIST", "Subnoteries listed", "count", len(items), "total", total)
	c.JSON(http.StatusOK, gin.H{
		"subnoteries": items,
		"total":       total,
		"page":        pag.Page,
		"limit":       pag.Limit,
	})
}

// GetSubnoteryDetail returns a single subnotery with its admins and member count.
//
// Public endpoint. Returns the subnotery name, admin usernames, and member count.
// Full member list is omitted to avoid exposing user data.
//
// DB: SELECT subnotery by ID with Preload for Admins and Members.
// Technologies: PostgreSQL (GORM).
// Helpers: helpers.MustParseSubnoteryID.
//
// Route: GET /api/v1/subnoteries/:subnotery_id
func (app *App) GetSubnoteryDetail(c *gin.Context) {
	subnoteryLog.Log("DETAIL", "Processing get subnotery detail request")

	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		return
	}

	var subnotery models.Subnotery
	if err := app.DB.Preload("Admins").Preload("Members").First(&subnotery, subnoteryID).Error; err != nil {
		subnoteryLog.Log("DETAIL", "Subnotery not found", "subnoteryID", subnoteryID, "error", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Subnotery not found"})
		return
	}

	// Build admin list with safe fields only
	type adminInfo struct {
		ID       uint   `json:"id"`
		Username string `json:"username"`
	}
	admins := make([]adminInfo, len(subnotery.Admins))
	for i, a := range subnotery.Admins {
		admins[i] = adminInfo{ID: a.ID, Username: a.Username}
	}

	// Check if the requesting user is a member (if authenticated)
	isMember := false
	if uid, exists := c.Get("user_id"); exists {
		userID := uid.(uint64)
		for _, m := range subnotery.Members {
			if uint64(m.ID) == userID {
				isMember = true
				break
			}
		}
	}

	subnoteryLog.Log("DETAIL", "Subnotery detail retrieved", "subnoteryID", subnoteryID)
	c.JSON(http.StatusOK, gin.H{
		"id":                    subnotery.ID,
		"name":                  subnotery.Name,
		"description":           subnotery.Description,
		"content_type":          subnotery.ContentType,
		"rules":                 subnotery.Rules,
		"banner_url":            subnotery.BannerURL,
		"background_color":      subnotery.BackgroundColor,
		"min_post_notoriety":    subnotery.MinPostNotoriety,
		"min_comment_notoriety": subnotery.MinCommentNotoriety,
		"admins":                admins,
		"member_count":          len(subnotery.Members),
		"is_member":             isMember,
		"created_at":            subnotery.CreatedAt,
		"updated_at":            subnotery.UpdatedAt,
	})
}

// GetSubnoteryNotes returns a paginated list of approved notes within a specific subnotery.
//
// Public endpoint. Only returns approved notes. Paginated with total count.
//
// DB: COUNT + SELECT from notes WHERE subnotery_id AND status = Approved.
// Technologies: PostgreSQL (GORM).
// Helpers: helpers.MustParseSubnoteryID, helpers.ParsePagination.
//
// Route: GET /api/v1/subnoteries/:subnotery_id/notes
func (app *App) GetSubnoteryNotes(c *gin.Context) {
	subnoteryLog.Log("NOTES", "Processing get subnotery notes request")

	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		return
	}

	pag := helpers.ParsePagination(c)
	sortParam := c.DefaultQuery("sort", "new")
	timeParam := c.DefaultQuery("time", "all")

	var total int64
	query := app.DB.Model(&models.Note{}).
		Where("subnotery_id = ? AND status = ?", subnoteryID, models.StatusApproved)

	// Apply time filter for top/controversial sorts
	if (sortParam == "top" || sortParam == "controversial") && timeParam != "all" {
		var since time.Time
		now := time.Now()
		switch timeParam {
		case "day":
			since = now.AddDate(0, 0, -1)
		case "week":
			since = now.AddDate(0, 0, -7)
		case "month":
			since = now.AddDate(0, -1, 0)
		case "year":
			since = now.AddDate(-1, 0, 0)
		}
		if !since.IsZero() {
			query = query.Where("created_at >= ?", since)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		subnoteryLog.Log("NOTES", "Failed to count notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notes"})
		return
	}

	// Apply sort order
	switch sortParam {
	case "top":
		query = query.Order("(upvotes - downvotes) DESC, created_at DESC")
	case "controversial":
		query = query.Order("(upvotes + downvotes) * 1.0 / CASE WHEN ABS(CAST(upvotes AS INTEGER) - CAST(downvotes AS INTEGER)) < 1 THEN 1 ELSE ABS(CAST(upvotes AS INTEGER) - CAST(downvotes AS INTEGER)) END DESC, created_at DESC")
	case "hot":
		query = query.Order("hotness DESC, created_at DESC")
	default: // "new"
		query = query.Order("created_at DESC")
	}

	var notes []models.Note
	if err := query.Offset(pag.Offset).Limit(pag.Limit).
		Find(&notes).Error; err != nil {
		subnoteryLog.Log("NOTES", "Failed to fetch notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notes"})
		return
	}

	subnoteryLog.Log("NOTES", "Subnotery notes listed", "subnoteryID", subnoteryID, "count", len(notes), "total", total, "sort", sortParam, "time", timeParam)

	// Populate subnotery names
	app.populateSubnoteryNames(notes)

	// Populate comment counts
	app.populateCommentCounts(notes)

	// Populate user votes if authenticated (endpoint is public, so check optionally)
	if userID, authenticated := helpers.TryGetUserID(c); authenticated {
		app.populateUserVotes(userID, notes)
	}

	c.JSON(http.StatusOK, gin.H{
		"notes": notes,
		"total": total,
		"page":  pag.Page,
		"limit": pag.Limit,
	})
}

// UpdateSubnoterySettings allows admins to update a subnotery's description,
// content type, and rules.
//
// Only subnotery admins and global admins can update settings.
//
// Route: PATCH /api/v1/subnoteries/:subnotery_id/settings
func (app *App) UpdateSubnoterySettings(c *gin.Context) {
	subnoteryLog.Log("SETTINGS", "Processing update settings request")

	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		return
	}

	userID := helpers.GetUserID(c)

	// Fetch subnotery
	var subnotery models.Subnotery
	if err := app.DB.First(&subnotery, subnoteryID).Error; err != nil {
		subnoteryLog.Log("SETTINGS", "Subnotery not found", "subnoteryID", subnoteryID, "error", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Subnotery not found"})
		return
	}

	// Check admin permission: global admin or subnotery admin
	var user models.User
	if err := app.DB.Select("id", "is_global_admin").First(&user, userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user"})
		return
	}
	isAllowed := user.IsGlobalAdmin
	if !isAllowed {
		var adminCount int64
		app.DB.Table("user_admins").
			Where("user_id = ? AND subnotery_id = ?", userID, subnoteryID).
			Count(&adminCount)
		isAllowed = adminCount > 0
	}
	if !isAllowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only admins can update subnotery settings"})
		return
	}

	// Bind request
	var req struct {
		Description         *string  `json:"description"`
		ContentType         *string  `json:"content_type"`
		Rules               *string  `json:"rules"`
		BannerURL           *string  `json:"banner_url"`
		BackgroundColor     *string  `json:"background_color"`
		MinPostNotoriety    *float64 `json:"min_post_notoriety"`
		MinCommentNotoriety *float64 `json:"min_comment_notoriety"`
	}
	if !helpers.BindJSON(c, &req) {
		return
	}

	// Apply updates
	updates := map[string]interface{}{}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.ContentType != nil {
		updates["content_type"] = *req.ContentType
	}
	if req.Rules != nil {
		updates["rules"] = *req.Rules
	}
	if req.BannerURL != nil {
		updates["banner_url"] = *req.BannerURL
	}
	if req.BackgroundColor != nil {
		updates["background_color"] = *req.BackgroundColor
	}
	if req.MinPostNotoriety != nil {
		updates["min_post_notoriety"] = *req.MinPostNotoriety
	}
	if req.MinCommentNotoriety != nil {
		updates["min_comment_notoriety"] = *req.MinCommentNotoriety
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No fields to update"})
		return
	}

	if err := app.DB.Model(&subnotery).Updates(updates).Error; err != nil {
		subnoteryLog.Log("SETTINGS", "Failed to update settings", "subnoteryID", subnoteryID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update settings"})
		return
	}

	subnoteryLog.Log("SETTINGS", "Settings updated", "subnoteryID", subnoteryID)
	c.JSON(http.StatusOK, gin.H{
		"message":      "Settings updated successfully",
		"description":  subnotery.Description,
		"content_type": subnotery.ContentType,
		"rules":        subnotery.Rules,
	})
}

// RemoveAdminFromSubnotery removes admin privileges from a user.
//
// Older admins (by user_admins row ID) can remove younger admins. Global admins
// can remove anyone. A user cannot remove themselves.
//
// Route: DELETE /api/v1/subnoteries/:subnotery_id/admins/:uid
func (app *App) RemoveAdminFromSubnotery(c *gin.Context) {
	subnoteryLog.Log("REMOVE_ADMIN", "Processing remove admin request")

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

	// Fetch requester
	var requester models.User
	if err := app.DB.Select("id", "is_global_admin").First(&requester, requesterID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user"})
		return
	}

	// Global admins can remove anyone
	if !requester.IsGlobalAdmin {
		// Check requester is admin of this subnotery
		type adminRow struct {
			UserID uint64
			ID     uint
		}
		var requesterRow adminRow
		app.DB.Table("user_admins").Select("user_id, id").
			Where("user_id = ? AND subnotery_id = ?", requesterID, subnoteryID).
			Scan(&requesterRow)
		if requesterRow.UserID == 0 {
			c.JSON(http.StatusForbidden, gin.H{"error": "You are not an admin of this community"})
			return
		}

		// Check target is admin
		var targetRow adminRow
		app.DB.Table("user_admins").Select("user_id, id").
			Where("user_id = ? AND subnotery_id = ?", targetUID, subnoteryID).
			Scan(&targetRow)
		if targetRow.UserID == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "Target user is not an admin of this community"})
			return
		}

		// Cannot remove yourself
		if requesterID == targetUID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "You cannot remove yourself — use the leave endpoint instead"})
			return
		}

		// Older admin (lower row ID) can remove younger admin (higher row ID)
		if requesterRow.ID >= targetRow.ID {
			c.JSON(http.StatusForbidden, gin.H{"error": "You can only remove admins who were added after you"})
			return
		}
	}

	// Remove the target from admins
	var target models.User
	if err := app.DB.First(&target, targetUID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	var sub models.Subnotery
	if err := app.DB.First(&sub, subnoteryID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subnotery not found"})
		return
	}
	if err := app.DB.Model(&sub).Association("Admins").Delete(&target); err != nil {
		subnoteryLog.Log("REMOVE_ADMIN", "Failed to remove admin", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove admin"})
		return
	}

	subnoteryLog.Log("REMOVE_ADMIN", "Admin removed", "subnoteryID", subnoteryID, "targetUserID", targetUID, "requesterID", requesterID)
	c.JSON(http.StatusOK, gin.H{"message": "Admin removed successfully"})
}

// UploadSubnoteryBanner handles banner image upload for a subnotery.
//
// Validates image type (JPEG/PNG/WebP/GIF) and magic bytes, stores in R2 at
// `banners/{subnotery_id}/banner.{ext}`, and updates the BannerURL field.
//
// Route: POST /api/v1/subnoteries/:subnotery_id/banner
func (app *App) UploadSubnoteryBanner(c *gin.Context) {
	subnoteryLog.Log("BANNER_UPLOAD", "Processing banner upload request")

	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		return
	}
	userID := helpers.GetUserID(c)

	// Verify admin permission
	if !app.isSubnoteryAdmin(userID, uint(subnoteryID)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only admins can upload banners"})
		return
	}

	if app.R2 == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "File storage not configured"})
		return
	}

	file, header, err := c.Request.FormFile("banner")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No banner file provided"})
		return
	}
	defer file.Close()

	if header.Size > 5*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Banner must be under 5 MB"})
		return
	}

	// Read file bytes for magic-byte validation
	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file"})
		return
	}

	ext := detectImageType(data)
	if ext == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid image type. Allowed: JPEG, PNG, WebP, GIF"})
		return
	}

	objectKey := fmt.Sprintf("banners/%d/banner.%s", subnoteryID, ext)
	contentType := "image/" + ext
	if ext == "jpg" {
		contentType = "image/jpeg"
	}

	_, err = app.R2.S3Client.PutObject(c.Request.Context(), &s3.PutObjectInput{
		Bucket:        aws.String(app.R2.BucketName),
		Key:           aws.String(objectKey),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		subnoteryLog.Log("BANNER_UPLOAD", "R2 upload failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload banner"})
		return
	}

	// Update the BannerURL in the database
	if err := app.DB.Model(&models.Subnotery{}).Where("id = ?", subnoteryID).
		Update("banner_url", objectKey).Error; err != nil {
		subnoteryLog.Log("BANNER_UPLOAD", "DB update failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update banner"})
		return
	}

	subnoteryLog.Log("BANNER_UPLOAD", "Banner uploaded", "subnoteryID", subnoteryID, "key", objectKey)
	c.JSON(http.StatusOK, gin.H{"message": "Banner uploaded successfully", "banner_url": objectKey})
}

// DeleteSubnoteryBanner removes the banner image for a subnotery.
//
// Route: DELETE /api/v1/subnoteries/:subnotery_id/banner
func (app *App) DeleteSubnoteryBanner(c *gin.Context) {
	subnoteryLog.Log("BANNER_DELETE", "Processing banner delete request")

	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		return
	}
	userID := helpers.GetUserID(c)

	if !app.isSubnoteryAdmin(userID, uint(subnoteryID)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only admins can delete banners"})
		return
	}

	// Clear banner URL in DB
	if err := app.DB.Model(&models.Subnotery{}).Where("id = ?", subnoteryID).
		Update("banner_url", "").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete banner"})
		return
	}

	// Best-effort R2 cleanup
	if app.R2 != nil {
		ctx := c.Request.Context()
		for _, ext := range []string{"jpg", "png", "webp", "gif"} {
			_, _ = app.R2.S3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(app.R2.BucketName),
				Key:    aws.String(fmt.Sprintf("banners/%d/banner.%s", subnoteryID, ext)),
			})
		}
	}

	subnoteryLog.Log("BANNER_DELETE", "Banner deleted", "subnoteryID", subnoteryID)
	c.JSON(http.StatusOK, gin.H{"message": "Banner deleted successfully"})
}

// GetSubnoteryBanner proxies the banner image from R2 with 24h cache headers.
//
// Route: GET /api/v1/subnoteries/:subnotery_id/banner
func (app *App) GetSubnoteryBanner(c *gin.Context) {
	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		return
	}

	var sub models.Subnotery
	if err := app.DB.Select("id", "banner_url").First(&sub, subnoteryID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subnotery not found"})
		return
	}

	if sub.BannerURL == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "No banner set"})
		return
	}

	if app.R2 == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "File storage not configured"})
		return
	}

	result, err := app.R2.S3Client.GetObject(c.Request.Context(), &s3.GetObjectInput{
		Bucket: aws.String(app.R2.BucketName),
		Key:    aws.String(sub.BannerURL),
	})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Banner not found"})
		return
	}
	defer result.Body.Close()

	contentLength := int64(0)
	if result.ContentLength != nil {
		contentLength = *result.ContentLength
	}
	contentType := "application/octet-stream"
	if result.ContentType != nil {
		contentType = *result.ContentType
	}

	c.Header("Cache-Control", "public, max-age=86400")
	c.DataFromReader(http.StatusOK, contentLength, contentType, result.Body, nil)
}

// isSubnoteryAdmin checks if a user is an admin of a specific subnotery (or global admin).
func (app *App) isSubnoteryAdmin(userID uint64, subnoteryID uint) bool {
	var user models.User
	if err := app.DB.Select("id", "is_global_admin").First(&user, userID).Error; err != nil {
		return false
	}
	if user.IsGlobalAdmin {
		return true
	}
	var count int64
	app.DB.Table("user_admins").
		Where("user_id = ? AND subnotery_id = ?", userID, subnoteryID).
		Count(&count)
	return count > 0
}

// detectImageType returns the file extension based on magic bytes, or "" if unknown.
func detectImageType(data []byte) string {
	if len(data) < 4 {
		return ""
	}
	// JPEG: FF D8 FF
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "jpg"
	}
	// PNG: 89 50 4E 47
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "png"
	}
	// GIF: GIF87a or GIF89a
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 {
		return "gif"
	}
	// WebP: RIFF....WEBP
	if len(data) >= 12 && data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		return "webp"
	}
	return ""
}
