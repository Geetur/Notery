// Package handlers provides HTTP request handlers for the Notery API.
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// subnoteryLog is the domain-specific logger for subnotery operations.
var subnoteryLog = helpers.SubnoteryLog

// ListSubnoteries returns all subnoteries with member/note counts.
// Supports pagination via page/limit query params.
//
// Route: GET /api/v1/subnoteries
func (app *App) ListSubnoteries(c *gin.Context) {
	subnoteryLog.Log("LIST", "Processing list subnoteries request")

	pg := helpers.ParsePagination(c)

	// SubnoteryListItem is the response DTO with counts.
	type SubnoteryListItem struct {
		ID          uint   `json:"id"`
		Name        string `json:"name"`
		MemberCount int64  `json:"member_count"`
		NoteCount   int64  `json:"note_count"`
	}

	var total int64
	if err := app.DB.Model(&models.Subnotery{}).Count(&total).Error; err != nil {
		subnoteryLog.Log("LIST", "Failed to count subnoteries", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subnoteries"})
		return
	}

	var subnoteries []models.Subnotery
	if err := app.DB.Order("name ASC").
		Offset(pg.Offset).Limit(pg.Limit).
		Find(&subnoteries).Error; err != nil {
		subnoteryLog.Log("LIST", "Failed to fetch subnoteries", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subnoteries"})
		return
	}

	// Build response with counts
	items := make([]SubnoteryListItem, len(subnoteries))
	for i, sub := range subnoteries {
		var memberCount int64
		app.DB.Table("user_memberships").Where("subnotery_id = ?", sub.ID).Count(&memberCount)
		var noteCount int64
		app.DB.Model(&models.Note{}).Where("subnotery_id = ? AND status = ?", sub.ID, models.StatusApproved).Count(&noteCount)

		items[i] = SubnoteryListItem{
			ID:          sub.ID,
			Name:        sub.Name,
			MemberCount: memberCount,
			NoteCount:   noteCount,
		}
	}

	subnoteryLog.Log("LIST", "Subnoteries retrieved", "count", len(items), "total", total)
	c.JSON(http.StatusOK, gin.H{
		"subnoteries": items,
		"total":       total,
		"page":        pg.Page,
		"limit":       pg.Limit,
	})
}

// GetSubnotery returns a single subnotery by ID with member/note counts.
//
// Route: GET /api/v1/subnoteries/:subnotery_id
func (app *App) GetSubnotery(c *gin.Context) {
	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		return
	}

	subnotery, ok := helpers.FetchSubnotery(c, app.DB, subnoteryID)
	if !ok {
		return
	}

	var memberCount int64
	app.DB.Table("user_memberships").Where("subnotery_id = ?", subnotery.ID).Count(&memberCount)
	var noteCount int64
	app.DB.Model(&models.Note{}).Where("subnotery_id = ? AND status = ?", subnotery.ID, models.StatusApproved).Count(&noteCount)

	// Check if requesting user is a member (if authenticated)
	isMember := false
	if userID, authenticated := helpers.TryGetUserID(c); authenticated {
		var count int64
		app.DB.Table("user_memberships").Where("user_id = ? AND subnotery_id = ?", userID, subnotery.ID).Count(&count)
		isMember = count > 0
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           subnotery.ID,
		"name":         subnotery.Name,
		"member_count": memberCount,
		"note_count":   noteCount,
		"is_member":    isMember,
		"created_at":   subnotery.CreatedAt,
	})
}

// LeaveSubnotery removes the authenticated user from a subnotery's member list.
// Admins cannot leave a subnotery they administer (must be removed or demoted first).
//
// Route: DELETE /api/v1/subnoteries/:subnotery_id/membership
func (app *App) LeaveSubnotery(c *gin.Context) {
	subnoteryLog.Log("LEAVE", "Processing leave subnotery request")

	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		return
	}

	subnotery, ok := helpers.FetchSubnotery(c, app.DB, subnoteryID)
	if !ok {
		return
	}

	userID := helpers.GetUserID(c)
	subnoteryLog.Log("LEAVE", "User identified", "userID", userID, "subnoteryID", subnoteryID)

	// Check if user is a member
	var memberCount int64
	app.DB.Table("user_memberships").
		Where("user_id = ? AND subnotery_id = ?", userID, subnotery.ID).
		Count(&memberCount)
	if memberCount == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "You are not a member of this subnotery"})
		return
	}

	// Prevent admins from leaving (must be demoted first)
	var adminCount int64
	app.DB.Table("user_admins").
		Where("user_id = ? AND subnotery_id = ?", userID, subnotery.ID).
		Count(&adminCount)
	if adminCount > 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admins cannot leave a subnotery. Remove your admin role first."})
		return
	}

	// Remove user from members
	user, ok := helpers.FetchUser(c, app.DB, userID)
	if !ok {
		return
	}

	if err := app.DB.Model(subnotery).Association("Members").Delete(user); err != nil {
		subnoteryLog.Log("LEAVE", "Failed to remove member", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to leave subnotery"})
		return
	}

	subnoteryLog.Log("LEAVE", "User left successfully", "userID", userID, "subnoteryID", subnoteryID)
	c.JSON(http.StatusOK, gin.H{"message": "Left subnotery successfully"})
}

// AddAdminToSubnotery grants admin privileges to a user for a specific subnotery.
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

	subnoteryLog.Log("JOIN", "User joined successfully", "userID", userID, "subnoteryID", subnoteryID)
	c.JSON(http.StatusOK, gin.H{"message": "Joined subnotery successfully"})
}
