// report.go — Bug report handler. Sends bug reports via email.
package handlers

import (
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// reportLog is the domain-specific logger for report operations.
var reportLog = helpers.NewLogger("REPORT")

const (
	maxBugReportLength = 5000
	bugReportRecipient = "noteryapp@outlook.com"
)

// SubmitBugReport receives a bug report from an authenticated user and sends
// an email to the Notery support address.
//
// Route: POST /api/v1/reports/bug
func (app *App) SubmitBugReport(c *gin.Context) {
	reportLog.Log("BUG", "Processing bug report")

	var req struct {
		Description string `json:"description" binding:"required"`
		Page        string `json:"page"`
	}
	if !helpers.BindJSON(c, &req) {
		return
	}

	if utf8.RuneCountInString(req.Description) > maxBugReportLength {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Description too long", "max": maxBugReportLength})
		return
	}

	userID := helpers.GetUserID(c)

	var user models.User
	if err := app.DB.Select("id", "username", "email").First(&user, userID).Error; err != nil {
		reportLog.Log("BUG", "Failed to fetch user", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit bug report"})
		return
	}

	subject := fmt.Sprintf("[Notery Bug] Report from %s (#%d)", user.Username, user.ID)
	body := fmt.Sprintf(
		"Bug Report\n"+
			"──────────\n"+
			"User: %s (ID: %d, Email: %s)\n"+
			"Page: %s\n\n"+
			"Description:\n%s\n",
		user.Username, user.ID, user.Email,
		req.Page, req.Description,
	)

	// Send email asynchronously so the HTTP response is instant
	go func() {
		if err := app.Mailer.Send(bugReportRecipient, subject, body); err != nil {
			reportLog.Log("BUG", "Failed to send bug report email", "user_id", userID, "error", err)
		} else {
			reportLog.Log("BUG", "Bug report email sent", "user_id", userID)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "Bug report submitted. Thank you!"})
}
