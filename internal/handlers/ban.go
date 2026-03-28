// ban.go — HTTP handlers for banning and unbanning users from subnoteries and the site.
//
// Ban durations: 1d, 7d, 30d, 1y, permanent.
// Subnotery admins can ban for their community.
// Global admins can issue site-wide bans.
//
// Bans are enforced at: join, post, comment, and vote time.
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

var banLog = helpers.NewLogger("BAN")

// BanUser bans a user from a subnotery.
//
// Only subnotery admins or global admins can ban. Seniority-checked:
// younger admins cannot ban older admins. Cannot ban yourself.
//
// Route: POST /api/v1/subnoteries/:subnotery_id/bans
func (app *App) BanUser(c *gin.Context) {
	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		return
	}
	actorID := helpers.GetUserID(c)

	var req struct {
		UserID   uint64 `json:"user_id" binding:"required"`
		Duration string `json:"duration" binding:"required"`
		Reason   string `json:"reason"`
	}
	if !helpers.BindJSON(c, &req) {
		return
	}

	if !models.ValidBanDuration(models.BanDuration(req.Duration)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid duration. Use: 1d, 7d, 30d, 1y, permanent"})
		return
	}
	if req.UserID == actorID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "You cannot ban yourself"})
		return
	}

	// Verify subnotery exists
	var subnotery models.Subnotery
	if err := app.DB.First(&subnotery, subnoteryID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subnotery not found"})
		return
	}

	// Check actor is admin
	var actor models.User
	if err := app.DB.Select("id", "is_global_admin").First(&actor, actorID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load user"})
		return
	}

	isGlobal := actor.IsGlobalAdmin
	var actorAdmin models.UserAdmin
	isSubAdmin := false
	if !isGlobal {
		if err := app.DB.Table("user_admins").
			Where("user_id = ? AND subnotery_id = ?", actorID, subnoteryID).
			First(&actorAdmin).Error; err == nil {
			isSubAdmin = true
		}
	}
	if !isGlobal && !isSubAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only admins can ban users"})
		return
	}

	// Seniority check: younger admins can't ban older admins
	if !isGlobal {
		var targetAdmin models.UserAdmin
		if err := app.DB.Table("user_admins").
			Where("user_id = ? AND subnotery_id = ?", req.UserID, subnoteryID).
			First(&targetAdmin).Error; err == nil {
			// Target is also an admin — check seniority
			if !actorAdmin.CreatedAt.Before(targetAdmin.CreatedAt) {
				c.JSON(http.StatusForbidden, gin.H{"error": "Cannot ban an admin with equal or greater seniority"})
				return
			}
		}
	}

	// Check for existing active ban
	var existingBan models.Ban
	if err := app.DB.Where("user_id = ? AND subnotery_id = ?", req.UserID, subnoteryID).
		First(&existingBan).Error; err == nil {
		if existingBan.IsActive() {
			c.JSON(http.StatusConflict, gin.H{"error": "User is already banned from this community"})
			return
		}
		// Expired ban — delete it so we can create a fresh one
		app.DB.Delete(&existingBan)
	}

	// Create ban
	var expiresAt *time.Time
	if dur, ok := models.ParseBanDuration(models.BanDuration(req.Duration)); ok {
		t := time.Now().Add(dur)
		expiresAt = &t
	} // else permanent

	ban := models.Ban{
		UserID:      req.UserID,
		SubnoteryID: uint(subnoteryID),
		BannedBy:    actorID,
		Reason:      req.Reason,
		ExpiresAt:   expiresAt,
	}
	if err := app.DB.Create(&ban).Error; err != nil {
		banLog.Log("BAN", "Failed to create ban", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to ban user"})
		return
	}

	// Remove user from members
	app.DB.Exec("DELETE FROM user_memberships WHERE user_id = ? AND subnotery_id = ?", req.UserID, subnoteryID)
	// Also strip admin if applicable
	app.DB.Exec("DELETE FROM user_admins WHERE user_id = ? AND subnotery_id = ?", req.UserID, subnoteryID)

	// Send ban notification
	durationLabel := req.Duration
	if req.Duration == "permanent" {
		durationLabel = "permanently"
	}
	metadata, _ := json.Marshal(map[string]string{
		"duration": req.Duration,
		"expires_at": func() string {
			if expiresAt != nil {
				return expiresAt.Format(time.RFC3339)
			}
			return ""
		}(),
	})
	notification := models.Notification{
		UserID:        req.UserID,
		Type:          models.NotifBan,
		Title:         fmt.Sprintf("Banned from n/%s", subnotery.Name),
		Message:       fmt.Sprintf("You have been banned %s. Reason: %s", durationLabel, req.Reason),
		ReferenceID:   uint64(subnoteryID),
		ReferenceType: "subnotery",
		ActorID:       actorID,
		Metadata:      string(metadata),
	}
	app.DB.Create(&notification)

	banLog.Log("BAN", "User banned", "userID", req.UserID, "subnoteryID", subnoteryID, "duration", req.Duration)
	c.JSON(http.StatusOK, gin.H{"message": "User banned", "ban": ban})
}

// UnbanUser removes a ban from a subnotery.
//
// Route: DELETE /api/v1/subnoteries/:subnotery_id/bans/:uid
func (app *App) UnbanUser(c *gin.Context) {
	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		return
	}
	actorID := helpers.GetUserID(c)
	targetUID, err := strconv.ParseUint(c.Param("uid"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Check actor is admin
	isAllowed := false
	var actor models.User
	if app.DB.Select("id", "is_global_admin").First(&actor, actorID).Error == nil {
		isAllowed = actor.IsGlobalAdmin
	}
	if !isAllowed {
		var count int64
		app.DB.Table("user_admins").Where("user_id = ? AND subnotery_id = ?", actorID, subnoteryID).Count(&count)
		isAllowed = count > 0
	}
	if !isAllowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only admins can unban users"})
		return
	}

	result := app.DB.Where("user_id = ? AND subnotery_id = ?", targetUID, subnoteryID).Delete(&models.Ban{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "No ban found for this user"})
		return
	}

	banLog.Log("UNBAN", "User unbanned", "userID", targetUID, "subnoteryID", subnoteryID)
	c.JSON(http.StatusOK, gin.H{"message": "User unbanned"})
}

// ListBans returns paginated list of active bans for a subnotery.
//
// Route: GET /api/v1/subnoteries/:subnotery_id/bans
func (app *App) ListBans(c *gin.Context) {
	subnoteryID, ok := helpers.MustParseSubnoteryID(c)
	if !ok {
		return
	}
	actorID := helpers.GetUserID(c)

	// Check actor is admin
	isAllowed := false
	var actor models.User
	if app.DB.Select("id", "is_global_admin").First(&actor, actorID).Error == nil {
		isAllowed = actor.IsGlobalAdmin
	}
	if !isAllowed {
		var count int64
		app.DB.Table("user_admins").Where("user_id = ? AND subnotery_id = ?", actorID, subnoteryID).Count(&count)
		isAllowed = count > 0
	}
	if !isAllowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only admins can view bans"})
		return
	}

	pag := helpers.ParsePagination(c)

	var total int64
	app.DB.Model(&models.Ban{}).Where("subnotery_id = ?", subnoteryID).Count(&total)

	var bans []models.Ban
	app.DB.Where("subnotery_id = ?", subnoteryID).
		Offset(pag.Offset).Limit(pag.Limit).
		Order("created_at DESC").
		Find(&bans)

	// Enrich with usernames
	type banItem struct {
		ID        uint       `json:"id"`
		UserID    uint64     `json:"user_id"`
		Username  string     `json:"username"`
		Reason    string     `json:"reason"`
		Duration  string     `json:"duration"`
		ExpiresAt *time.Time `json:"expires_at"`
		CreatedAt time.Time  `json:"created_at"`
		IsExpired bool       `json:"is_expired"`
	}
	items := make([]banItem, 0, len(bans))
	for _, b := range bans {
		username := ""
		var u models.User
		if app.DB.Select("username").First(&u, b.UserID).Error == nil {
			username = u.Username
		}
		dur := "permanent"
		if b.ExpiresAt != nil {
			diff := time.Until(*b.ExpiresAt)
			switch {
			case diff <= 2*24*time.Hour:
				dur = "1d"
			case diff <= 8*24*time.Hour:
				dur = "7d"
			case diff <= 31*24*time.Hour:
				dur = "30d"
			default:
				dur = "1y"
			}
		}
		items = append(items, banItem{
			ID:        b.ID,
			UserID:    b.UserID,
			Username:  username,
			Reason:    b.Reason,
			Duration:  dur,
			ExpiresAt: b.ExpiresAt,
			CreatedAt: b.CreatedAt,
			IsExpired: b.IsExpired(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"bans":  items,
		"total": total,
		"page":  pag.Page,
		"limit": pag.Limit,
	})
}

// SiteWideBan bans a user from the entire site (global admin only).
//
// Route: POST /api/v1/admin/bans
func (app *App) SiteWideBan(c *gin.Context) {
	actorID := helpers.GetUserID(c)

	var req struct {
		UserID   uint64 `json:"user_id" binding:"required"`
		Duration string `json:"duration" binding:"required"`
		Reason   string `json:"reason"`
	}
	if !helpers.BindJSON(c, &req) {
		return
	}

	if !models.ValidBanDuration(models.BanDuration(req.Duration)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid duration"})
		return
	}
	if req.UserID == actorID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "You cannot ban yourself"})
		return
	}

	// Check existing site-wide ban
	var existing models.Ban
	if err := app.DB.Where("user_id = ? AND subnotery_id = 0", req.UserID).First(&existing).Error; err == nil {
		if existing.IsActive() {
			c.JSON(http.StatusConflict, gin.H{"error": "User already has an active site-wide ban"})
			return
		}
		app.DB.Delete(&existing)
	}

	var expiresAt *time.Time
	if dur, ok := models.ParseBanDuration(models.BanDuration(req.Duration)); ok {
		t := time.Now().Add(dur)
		expiresAt = &t
	}

	ban := models.Ban{
		UserID:      req.UserID,
		SubnoteryID: 0,
		BannedBy:    actorID,
		Reason:      req.Reason,
		ExpiresAt:   expiresAt,
	}
	if err := app.DB.Create(&ban).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create ban"})
		return
	}

	// Notification
	durationLabel := req.Duration
	if req.Duration == "permanent" {
		durationLabel = "permanently"
	}
	notification := models.Notification{
		UserID:        req.UserID,
		Type:          models.NotifBan,
		Title:         "Banned from Notery",
		Message:       fmt.Sprintf("You have been banned %s. Reason: %s", durationLabel, req.Reason),
		ReferenceID:   0,
		ReferenceType: "site",
		ActorID:       actorID,
	}
	app.DB.Create(&notification)

	banLog.Log("SITE_BAN", "User site-banned", "userID", req.UserID, "duration", req.Duration)
	c.JSON(http.StatusOK, gin.H{"message": "User banned site-wide", "ban": ban})
}

// RemoveSiteWideBan removes a site-wide ban (global admin only).
//
// Route: DELETE /api/v1/admin/bans/:uid
func (app *App) RemoveSiteWideBan(c *gin.Context) {
	targetUID, err := strconv.ParseUint(c.Param("uid"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	result := app.DB.Where("user_id = ? AND subnotery_id = 0", targetUID).Delete(&models.Ban{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "No site-wide ban found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Site-wide ban removed"})
}
