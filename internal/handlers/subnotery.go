// subnotery.go — HTTP handlers for subnotery management (join, admin).
//
// ENDPOINTS:
//
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
