// subnotery.go — HTTP handlers for subnotery management (browse, join, admin).
//
// ENDPOINTS:
//
//	GET  /subnoteries                       List all subnoteries (paginated)
//	GET  /subnoteries/:subnotery_id         Get a single subnotery with admin/member info
//	GET  /subnoteries/:subnotery_id/notes   List approved notes in a subnotery (paginated)
//	POST /subnoteries/:subnotery_id/join    Join a subnotery as a member
//	POST /subnoteries/:subnotery_id/admins  Grant admin privileges to a user
//
// DESIGN:
//
//	Subnoteries are community containers for notes. Users join as members;
//	admins are promoted by existing admins. Membership and admin status are
//	stored via GORM many-to-many associations (user_memberships, user_admins).
//	Auto-creation of subnoteries happens in the note creation flow (note.go),
//	not here.
package handlers

import (
	"net/http"

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

	subnoteryLog.Log("JOIN", "User joined successfully", "userID", userID, "subnoteryID", subnoteryID)
	c.JSON(http.StatusOK, gin.H{"message": "Joined subnotery successfully"})
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

	subnoteryLog.Log("DETAIL", "Subnotery detail retrieved", "subnoteryID", subnoteryID)
	c.JSON(http.StatusOK, gin.H{
		"id":           subnotery.ID,
		"name":         subnotery.Name,
		"admins":       admins,
		"member_count": len(subnotery.Members),
		"created_at":   subnotery.CreatedAt,
		"updated_at":   subnotery.UpdatedAt,
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

	var total int64
	query := app.DB.Model(&models.Note{}).
		Where("subnotery_id = ? AND status = ?", subnoteryID, models.StatusApproved)

	if err := query.Count(&total).Error; err != nil {
		subnoteryLog.Log("NOTES", "Failed to count notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notes"})
		return
	}

	var notes []models.Note
	if err := query.Offset(pag.Offset).Limit(pag.Limit).
		Order("created_at DESC").
		Find(&notes).Error; err != nil {
		subnoteryLog.Log("NOTES", "Failed to fetch notes", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notes"})
		return
	}

	subnoteryLog.Log("NOTES", "Subnotery notes listed", "subnoteryID", subnoteryID, "count", len(notes), "total", total)
	c.JSON(http.StatusOK, gin.H{
		"notes": notes,
		"total": total,
		"page":  pag.Page,
		"limit": pag.Limit,
	})
}
