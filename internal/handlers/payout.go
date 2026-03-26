// payout.go — Stripe Connect onboarding and payout status endpoints.
//
// Creators must connect a Stripe Express account to receive payouts.
// If they don't, purchase proceeds are retained by Notery.
//
// Endpoints:
//   - POST /me/stripe/connect     — Create Connected Account + return onboarding URL
//   - GET  /me/stripe/status      — Check payout readiness
//   - POST /me/stripe/refresh-link — Regenerate onboarding link
package handlers

import (
	"net/http"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
	"github.com/gin-gonic/gin"
)

var payoutLog = helpers.NewLogger("PAYOUT")

// StripeConnect initiates Stripe Express onboarding for the authenticated user.
//
// If the user already has a StripeAccountID, a fresh onboarding/login link is
// returned so they can finish or manage their account.
//
// Route: POST /api/v1/me/stripe/connect
func (app *App) StripeConnect(c *gin.Context) {
	payoutLog.Log("CONNECT", "Processing Stripe Connect request")

	if app.Payment == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Payment service not configured"})
		return
	}

	userID := helpers.GetUserID(c)
	var user models.User
	if err := app.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load user"})
		return
	}

	// Create account if not already started
	if user.StripeAccountID == "" {
		acctID, err := app.Payment.CreateConnectedAccount(c.Request.Context(), user.Email)
		if err != nil {
			payoutLog.Log("CONNECT", "Failed to create connected account", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create payout account"})
			return
		}
		user.StripeAccountID = acctID
		if err := app.DB.Model(&user).Update("stripe_account_id", acctID).Error; err != nil {
			payoutLog.Log("CONNECT", "Failed to save account ID", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save payout account"})
			return
		}
		payoutLog.Log("CONNECT", "Created connected account", "acctID", acctID)
	}

	// Generate onboarding link
	returnURL := app.FrontendURL + "/profile?tab=settings&stripe=complete"
	refreshURL := app.FrontendURL + "/profile?tab=settings&stripe=refresh"

	link, err := app.Payment.CreateOnboardingLink(c.Request.Context(), user.StripeAccountID, returnURL, refreshURL)
	if err != nil {
		payoutLog.Log("CONNECT", "Failed to create onboarding link", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create onboarding link"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"onboarding_url": link,
		"account_id":     user.StripeAccountID,
	})
}

// StripeStatus returns the current payout status for the authenticated user.
//
// If the user has a Connected Account, it checks Stripe for charges_enabled
// and updates the local DB accordingly.
//
// Route: GET /api/v1/me/stripe/status
func (app *App) StripeStatus(c *gin.Context) {
	userID := helpers.GetUserID(c)
	var user models.User
	if err := app.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load user"})
		return
	}

	resp := gin.H{
		"has_account":         user.StripeAccountID != "",
		"payout_enabled":      user.PayoutEnabled,
		"onboarding_complete": user.StripeOnboardingComplete,
		"account_id":          user.StripeAccountID,
	}

	// If they have an account and payment service is available, refresh from Stripe
	if user.StripeAccountID != "" && app.Payment != nil {
		chargesEnabled, err := app.Payment.GetAccountStatus(c.Request.Context(), user.StripeAccountID)
		if err != nil {
			payoutLog.Log("STATUS", "Failed to check account status", "error", err)
			// Return cached state
			c.JSON(http.StatusOK, resp)
			return
		}

		// Update local cache if changed
		if chargesEnabled != user.PayoutEnabled || (chargesEnabled && !user.StripeOnboardingComplete) {
			updates := map[string]interface{}{
				"payout_enabled":             chargesEnabled,
				"stripe_onboarding_complete": chargesEnabled || user.StripeOnboardingComplete,
			}
			app.DB.Model(&user).Updates(updates)
		}

		resp["payout_enabled"] = chargesEnabled
		resp["onboarding_complete"] = chargesEnabled || user.StripeOnboardingComplete
	}

	c.JSON(http.StatusOK, resp)
}

// StripeRefreshLink generates a new onboarding link for a user who didn't finish.
//
// Route: POST /api/v1/me/stripe/refresh-link
func (app *App) StripeRefreshLink(c *gin.Context) {
	if app.Payment == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Payment service not configured"})
		return
	}

	userID := helpers.GetUserID(c)
	var user models.User
	if err := app.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load user"})
		return
	}

	if user.StripeAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No payout account found. Use /me/stripe/connect first."})
		return
	}

	returnURL := app.FrontendURL + "/profile?tab=settings&stripe=complete"
	refreshURL := app.FrontendURL + "/profile?tab=settings&stripe=refresh"

	link, err := app.Payment.CreateOnboardingLink(c.Request.Context(), user.StripeAccountID, returnURL, refreshURL)
	if err != nil {
		payoutLog.Log("REFRESH", "Failed to create onboarding link", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create onboarding link"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"onboarding_url": link})
}
