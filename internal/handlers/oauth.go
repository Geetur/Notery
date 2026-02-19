// oauth.go — HTTP handlers for OAuth2 authentication (Google & GitHub).
//
// ENDPOINTS:
//
//	GET  /auth/oauth/google           Redirect to Google consent screen
//	GET  /auth/oauth/google/callback  Handle Google OAuth callback
//	GET  /auth/oauth/github           Redirect to GitHub consent screen
//	GET  /auth/oauth/github/callback  Handle GitHub OAuth callback
//
// FLOW:
//
//  1. User clicks "Sign in with Google/GitHub" on the frontend.
//  2. Frontend navigates to /api/v1/auth/oauth/{provider}.
//  3. Backend redirects to the provider's consent screen with a CSRF state param.
//  4. Provider redirects back to /api/v1/auth/oauth/{provider}/callback?code=...&state=...
//  5. Backend exchanges the code for user info, creates or finds the user,
//     issues access + refresh tokens, and redirects to the frontend with tokens.
//
// SECURITY:
//
//	CSRF protection via random state parameter stored in a short-lived cookie.
//	OAuth users are auto-verified (provider already verified the email).
//	OAuth users have no password hash — password login is disabled for them.
package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	oauthGithub "golang.org/x/oauth2/github"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// oauthLog is the domain-specific logger for OAuth operations.
var oauthLog = helpers.NewLogger("OAUTH")

// Google OAuth2 endpoints (not in the x/oauth2 package directly).
var googleOAuthEndpoint = oauth2.Endpoint{
	AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL: "https://oauth2.googleapis.com/token",
}

// googleUserInfo holds the response from Google's userinfo endpoint.
type googleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

// githubUserInfo holds the response from GitHub's user endpoint.
type githubUserInfo struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// githubEmail holds the response from GitHub's user/emails endpoint.
type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

// ----- HELPERS -----

// generateState creates a random CSRF state parameter for OAuth flows.
func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// googleOAuthConfig builds the Google OAuth2 config from app settings.
func (app *App) googleOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     app.GoogleClientID,
		ClientSecret: app.GoogleClientSecret,
		RedirectURL:  app.BaseURL + "/api/v1/auth/oauth/google/callback",
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     googleOAuthEndpoint,
	}
}

// githubOAuthConfig builds the GitHub OAuth2 config from app settings.
func (app *App) githubOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     app.GitHubClientID,
		ClientSecret: app.GitHubClientSecret,
		RedirectURL:  app.BaseURL + "/api/v1/auth/oauth/github/callback",
		Scopes:       []string{"user:email"},
		Endpoint:     oauthGithub.Endpoint,
	}
}

// oauthFindOrCreateUser looks up a user by OAuth provider+ID.
// If not found, creates a new user with the given email and username.
// Returns the user and any error.
func (app *App) oauthFindOrCreateUser(provider, oauthID, email, displayName string) (*models.User, error) {
	var user models.User

	// First, try to find by OAuth provider + ID
	err := app.DB.Where("oauth_provider = ? AND oauth_id = ?", provider, oauthID).First(&user).Error
	if err == nil {
		return &user, nil
	}

	// Next, try to find by email (link existing account to OAuth)
	err = app.DB.Where("email = ?", email).First(&user).Error
	if err == nil {
		// Link this OAuth provider to the existing account
		app.DB.Model(&user).Updates(map[string]interface{}{
			"oauth_provider": provider,
			"oauth_id":       oauthID,
			"email_verified": true,
		})
		return &user, nil
	}

	// Create new user — resolve duplicate usernames by appending numeric suffix
	baseUsername := sanitizeUsername(displayName)
	username := baseUsername
	for i := 1; ; i++ {
		var count int64
		app.DB.Model(&models.User{}).Where("username = ?", username).Count(&count)
		if count == 0 {
			break
		}
		username = fmt.Sprintf("%s%d", baseUsername, i)
		// Ensure suffixed name doesn't exceed max length
		if len(username) > models.MaxUsernameLength {
			baseUsername = baseUsername[:models.MaxUsernameLength-len(fmt.Sprintf("%d", i))]
			username = fmt.Sprintf("%s%d", baseUsername, i)
		}
	}
	user = models.User{
		Email:            email,
		Username:         username,
		DisplayNameField: displayName,
		OAuthProvider:    provider,
		OAuthID:          oauthID,
		EmailVerified:    true,
		Hash:             "", // No password for OAuth users
	}
	if result := app.DB.Create(&user); result.Error != nil {
		return nil, result.Error
	}
	oauthLog.Log("CREATE", "OAuth user created", "provider", provider, "userID", user.ID, "email", email, "username", username)
	return &user, nil
}

// sanitizeUsername creates a valid username from an OAuth display name.
// Strips non-alphanumeric chars and enforces length limits.
func sanitizeUsername(name string) string {
	var sb strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			sb.WriteRune(r)
		}
	}
	result := sb.String()
	if len(result) < models.MinUsernameLength {
		// Pad with random suffix
		suffix, _ := models.GenerateSecureToken(4)
		result = "user_" + suffix
	}
	if len(result) > models.MaxUsernameLength {
		result = result[:models.MaxUsernameLength]
	}
	return result
}

// issueTokensAndRedirect creates tokens and redirects the user to the frontend.
func (app *App) issueTokensAndRedirect(c *gin.Context, user *models.User) {
	accessToken, err := app.issueAccessToken(user.ID)
	if err != nil {
		oauthLog.Log("TOKEN", "Failed to issue access token", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=token_failed")
		return
	}

	refreshToken, err := app.issueRefreshToken(uint64(user.ID), "")
	if err != nil {
		oauthLog.Log("TOKEN", "Failed to issue refresh token", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=token_failed")
		return
	}

	redirectURL := fmt.Sprintf("%s/auth/callback?access_token=%s&refresh_token=%s",
		app.FrontendURL,
		url.QueryEscape(accessToken),
		url.QueryEscape(refreshToken),
	)
	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
}

// ----- GOOGLE -----

// OAuthGoogle redirects the user to Google's consent screen.
//
// Route: GET /api/v1/auth/oauth/google
func (app *App) OAuthGoogle(c *gin.Context) {
	if app.GoogleClientID == "" {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Google OAuth is not configured"})
		return
	}

	state, err := generateState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate state"})
		return
	}

	// Store state in a short-lived cookie for CSRF verification
	c.SetCookie("oauth_state", state, 600, "/", "", false, true)

	cfg := app.googleOAuthConfig()
	url := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "select_account"))
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// OAuthGoogleCallback handles the Google OAuth callback.
//
// Route: GET /api/v1/auth/oauth/google/callback
func (app *App) OAuthGoogleCallback(c *gin.Context) {
	// Verify CSRF state
	state, _ := c.Cookie("oauth_state")
	if state == "" || state != c.Query("state") {
		oauthLog.Log("GOOGLE", "State mismatch — possible CSRF")
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=invalid_state")
		return
	}
	// Clear state cookie
	c.SetCookie("oauth_state", "", -1, "/", "", false, true)

	code := c.Query("code")
	if code == "" {
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=no_code")
		return
	}

	cfg := app.googleOAuthConfig()
	token, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		oauthLog.Log("GOOGLE", "Code exchange failed", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=exchange_failed")
		return
	}

	// Fetch user info from Google
	client := cfg.Client(context.Background(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		oauthLog.Log("GOOGLE", "Failed to fetch user info", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=userinfo_failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=read_failed")
		return
	}

	var info googleUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=parse_failed")
		return
	}

	if info.Email == "" {
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=no_email")
		return
	}

	user, err := app.oauthFindOrCreateUser("google", info.ID, info.Email, info.Name)
	if err != nil {
		oauthLog.Log("GOOGLE", "Failed to find/create user", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=create_failed")
		return
	}

	oauthLog.Log("GOOGLE", "OAuth login successful", "userID", user.ID, "email", info.Email)
	app.issueTokensAndRedirect(c, user)
}

// ----- GITHUB -----

// OAuthGitHub redirects the user to GitHub's consent screen.
//
// Route: GET /api/v1/auth/oauth/github
func (app *App) OAuthGitHub(c *gin.Context) {
	if app.GitHubClientID == "" {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "GitHub OAuth is not configured"})
		return
	}

	state, err := generateState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate state"})
		return
	}

	c.SetCookie("oauth_state", state, 600, "/", "", false, true)

	cfg := app.githubOAuthConfig()
	url := cfg.AuthCodeURL(state, oauth2.SetAuthURLParam("prompt", "select_account"))
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// OAuthGitHubCallback handles the GitHub OAuth callback.
//
// Route: GET /api/v1/auth/oauth/github/callback
func (app *App) OAuthGitHubCallback(c *gin.Context) {
	// Verify CSRF state
	state, _ := c.Cookie("oauth_state")
	if state == "" || state != c.Query("state") {
		oauthLog.Log("GITHUB", "State mismatch — possible CSRF")
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=invalid_state")
		return
	}
	c.SetCookie("oauth_state", "", -1, "/", "", false, true)

	code := c.Query("code")
	if code == "" {
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=no_code")
		return
	}

	cfg := app.githubOAuthConfig()
	token, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		oauthLog.Log("GITHUB", "Code exchange failed", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=exchange_failed")
		return
	}

	// Fetch user info from GitHub
	client := cfg.Client(context.Background(), token)
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		oauthLog.Log("GITHUB", "Failed to fetch user info", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=userinfo_failed")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=read_failed")
		return
	}

	var info githubUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=parse_failed")
		return
	}

	// GitHub may not return email in the user endpoint — fetch from /user/emails
	userEmail := info.Email
	if userEmail == "" {
		userEmail = app.fetchGitHubPrimaryEmail(client)
	}
	if userEmail == "" {
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=no_email")
		return
	}

	displayName := info.Name
	if displayName == "" {
		displayName = info.Login
	}

	oauthID := fmt.Sprintf("%d", info.ID)
	user, err := app.oauthFindOrCreateUser("github", oauthID, userEmail, displayName)
	if err != nil {
		oauthLog.Log("GITHUB", "Failed to find/create user", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, app.FrontendURL+"/login?error=create_failed")
		return
	}

	oauthLog.Log("GITHUB", "OAuth login successful", "userID", user.ID, "email", userEmail)
	app.issueTokensAndRedirect(c, user)
}

// fetchGitHubPrimaryEmail fetches the primary verified email from GitHub's /user/emails API.
func (app *App) fetchGitHubPrimaryEmail(client *http.Client) string {
	resp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var emails []githubEmail
	if err := json.Unmarshal(body, &emails); err != nil {
		return ""
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email
		}
	}
	// Fall back to first verified email
	for _, e := range emails {
		if e.Verified {
			return e.Email
		}
	}
	return ""
}

// ----- PROVIDER AVAILABILITY -----

// OAuthProviders returns which OAuth providers are configured.
//
// Route: GET /api/v1/auth/oauth/providers
func (app *App) OAuthProviders(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"google": app.GoogleClientID != "",
		"github": app.GitHubClientID != "",
	})
}
