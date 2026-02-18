// auth.go — HTTP handlers for authentication, session management, and account security.
//
// ENDPOINTS:
//
//	POST /auth/signup            Register a new user (issues tokens + verification email)
//	POST /auth/login             Authenticate and issue access + refresh tokens
//	POST /auth/refresh           Rotate refresh token and issue new access token
//	POST /auth/logout            Revoke a single refresh token
//	POST /auth/logout-all        Revoke all refresh tokens for the authenticated user
//	GET  /auth/verify-email      Validate an email verification token (from URL query)
//	POST /auth/resend-verification  Resend a verification email (authenticated)
//	POST /auth/forgot-password   Request a password reset email (anti-enumeration)
//	POST /auth/reset-password    Reset password using a token (single-use)
//	POST /auth/change-password   Change password while authenticated (revokes sessions)
//
// SECURITY:
//
//	Access tokens are short-lived JWTs (15 min). Refresh tokens are opaque
//	hex strings (30-day expiry) stored as SHA-256 hashes in the database.
//	Refresh token rotation with family-based theft detection: if a revoked
//	token is reused, the entire family is invalidated.
//
//	Password reset tokens are single-use with 1-hour expiry. The endpoint
//	always returns 200 regardless of whether the email exists to prevent
//	user enumeration.
//
//	Email verification is best-effort: send failures are logged but do not
//	block signup or other flows.
package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/Geetur/Notery/internal/email"
	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// authLog is the domain-specific logger for authentication operations.
var authLog = helpers.AuthLog

// AuthRequest represents the JSON body for signup and login requests.
type AuthRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	Username string `json:"username"` // optional on signup, ignored on login
}

// ----- TOKEN ISSUANCE HELPERS -----

// issueAccessToken creates a short-lived JWT access token for a user.
func (app *App) issueAccessToken(userID uint) (string, error) {
	secretKey := []byte(app.JWTSecret)
	if len(secretKey) == 0 {
		return "", fmt.Errorf("JWT_SECRET not configured")
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": fmt.Sprint(userID),
		"exp":     time.Now().Add(models.AccessTokenTTL).Unix(),
	})
	return token.SignedString(secretKey)
}

// issueRefreshToken generates, persists, and returns an opaque refresh token.
// familyID groups rotated tokens for theft detection. Pass "" to start a new family.
func (app *App) issueRefreshToken(userID uint64, familyID string) (string, error) {
	raw, err := models.GenerateSecureToken(models.RefreshTokenBytes)
	if err != nil {
		return "", err
	}
	if familyID == "" {
		fam, err := models.GenerateSecureToken(16)
		if err != nil {
			return "", err
		}
		familyID = fam
	}
	rt := models.RefreshToken{
		TokenHash: models.HashToken(raw),
		UserID:    userID,
		FamilyID:  familyID,
		ExpiresAt: time.Now().Add(models.RefreshTokenTTL),
	}
	if err := app.DB.Create(&rt).Error; err != nil {
		return "", err
	}
	return raw, nil
}

// ----- SIGNUP -----

// Signup handles user registration and account creation.
//
// After a successful signup, the user is sent a verification email (best-effort).
// The response contains an access token and refresh token so the user can start
// using the app immediately.
//
// Route: POST /api/v1/auth/signup
func (app *App) Signup(c *gin.Context) {
	authLog.Log("SIGNUP", "Processing signup request")

	var authReq AuthRequest
	if !helpers.BindJSON(c, &authReq) {
		authLog.Log("SIGNUP", "Failed to bind JSON request")
		return
	}
	authLog.Log("SIGNUP", "Request validated", "email", authReq.Email)

	if len(authReq.Password) < 8 {
		authLog.Log("SIGNUP", "Password too short")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be at least 8 characters"})
		return
	}

	user := &models.User{Email: authReq.Email, Username: authReq.Username}
	if err := user.SetPassword(authReq.Password); err != nil {
		authLog.Log("SIGNUP", "Failed to hash password", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	if result := app.DB.Create(user); result.Error != nil {
		authLog.Log("SIGNUP", "Failed to create user in database", "error", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user (email already exists?)"})
		return
	}

	authLog.Log("SIGNUP", "User created successfully", "userID", user.ID, "email", authReq.Email)

	// Issue access + refresh tokens
	accessToken, err := app.issueAccessToken(user.ID)
	if err != nil {
		authLog.Log("SIGNUP", "Failed to issue access token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	refreshToken, err := app.issueRefreshToken(uint64(user.ID), "")
	if err != nil {
		authLog.Log("SIGNUP", "Failed to issue refresh token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	// Dev mode (no SMTP): auto-verify user. Production: send verification email.
	if _, isLog := app.Mailer.(*email.LogMailer); isLog {
		app.DB.Model(user).Update("email_verified", true)
		authLog.Log("SIGNUP", "Dev mode — auto-verified user", "userID", user.ID)
	} else {
		app.sendVerificationEmail(uint64(user.ID), user.Email)
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":       "User created successfully",
		"user_id":       user.ID,
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// ----- LOGIN -----

// Login handles user authentication and returns an access + refresh token pair.
//
// Route: POST /api/v1/auth/login
func (app *App) Login(c *gin.Context) {
	authLog.Log("LOGIN", "Processing login request")

	var authReq AuthRequest
	if !helpers.BindJSON(c, &authReq) {
		authLog.Log("LOGIN", "Failed to bind JSON request")
		return
	}
	authLog.Log("LOGIN", "Request validated", "email", authReq.Email)

	user, found := helpers.FetchUserByEmail(app.DB, authReq.Email)
	if !found {
		authLog.Log("LOGIN", "User not found", "email", authReq.Email)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	if !user.CheckPassword(authReq.Password) {
		authLog.Log("LOGIN", "Invalid password", "email", authReq.Email)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}
	authLog.Log("LOGIN", "Password verified", "userID", user.ID)

	accessToken, err := app.issueAccessToken(user.ID)
	if err != nil {
		authLog.Log("LOGIN", "Failed to issue access token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	refreshToken, err := app.issueRefreshToken(uint64(user.ID), "")
	if err != nil {
		authLog.Log("LOGIN", "Failed to issue refresh token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	authLog.Log("LOGIN", "Login successful", "userID", user.ID, "email", authReq.Email)
	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// ----- REFRESH TOKEN ROTATION -----

// RefreshRequest represents the JSON body for a token refresh or logout request.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// RefreshAccessToken validates a refresh token, rotates it (revokes old, issues new),
// and returns a new access + refresh token pair.
//
// Theft detection: if a revoked token is reused, the entire token family is
// invalidated. This forces the legitimate user to re-authenticate but prevents
// the attacker from using any stolen tokens.
//
// Route: POST /api/v1/auth/refresh
func (app *App) RefreshAccessToken(c *gin.Context) {
	var req RefreshRequest
	if !helpers.BindJSON(c, &req) {
		return
	}

	hash := models.HashToken(req.RefreshToken)

	var rt models.RefreshToken
	if err := app.DB.Where("token_hash = ?", hash).First(&rt).Error; err != nil {
		authLog.Log("REFRESH", "token not found")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	// Theft detection: revoked token reuse → revoke entire family
	if rt.Revoked {
		authLog.Log("REFRESH", "revoked token reuse — revoking family", "family", rt.FamilyID, "user", rt.UserID)
		app.DB.Model(&models.RefreshToken{}).Where("family_id = ?", rt.FamilyID).Update("revoked", true)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token reuse detected — all sessions in this family have been revoked"})
		return
	}

	if rt.IsExpired() {
		authLog.Log("REFRESH", "token expired", "user", rt.UserID)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token expired"})
		return
	}

	// Revoke old token
	app.DB.Model(&rt).Update("revoked", true)

	// Issue new pair (same family for rotation tracking)
	accessToken, err := app.issueAccessToken(uint(rt.UserID))
	if err != nil {
		authLog.Log("REFRESH", "failed to issue access token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	newRefresh, err := app.issueRefreshToken(rt.UserID, rt.FamilyID)
	if err != nil {
		authLog.Log("REFRESH", "failed to issue refresh token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	authLog.Log("REFRESH", "token rotated", "user", rt.UserID)
	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": newRefresh,
	})
}

// ----- LOGOUT -----

// Logout revokes a single refresh token.
// Always returns 200 to prevent token enumeration.
//
// Route: POST /api/v1/auth/logout
func (app *App) Logout(c *gin.Context) {
	var req RefreshRequest
	if !helpers.BindJSON(c, &req) {
		return
	}

	hash := models.HashToken(req.RefreshToken)
	app.DB.Model(&models.RefreshToken{}).Where("token_hash = ?", hash).Update("revoked", true)
	c.JSON(http.StatusOK, gin.H{"message": "Logged out"})
}

// LogoutAll revokes all refresh tokens for the authenticated user.
//
// Route: POST /api/v1/auth/logout-all
func (app *App) LogoutAll(c *gin.Context) {
	userID := helpers.GetUserID(c)
	app.DB.Model(&models.RefreshToken{}).Where("user_id = ?", userID).Update("revoked", true)
	authLog.Log("LOGOUT_ALL", "all sessions revoked", "user", userID)
	c.JSON(http.StatusOK, gin.H{"message": "All sessions revoked"})
}

// ----- EMAIL VERIFICATION -----

// sendVerificationEmail generates a verification token and emails it to the user.
// Best-effort: failures are logged but do not block the caller.
func (app *App) sendVerificationEmail(userID uint64, userEmail string) {
	if app.Mailer == nil {
		authLog.Log("VERIFY", "no mailer configured, skipping verification email")
		return
	}

	raw, err := models.GenerateSecureToken(models.EmailVerificationTokenBytes)
	if err != nil {
		authLog.Log("VERIFY", "failed to generate verification token", "error", err)
		return
	}

	ev := models.EmailVerification{
		UserID:    userID,
		TokenHash: models.HashToken(raw),
		ExpiresAt: time.Now().Add(models.EmailVerificationTTL),
	}
	if err := app.DB.Create(&ev).Error; err != nil {
		authLog.Log("VERIFY", "failed to save verification token", "error", err)
		return
	}

	subject, body := email.VerificationEmail(app.BaseURL, raw)
	if err := app.Mailer.Send(userEmail, subject, body); err != nil {
		authLog.Log("VERIFY", "failed to send verification email", "error", err, "email", userEmail)
	}
}

// VerifyEmail validates an email verification token from the URL query string.
//
// Route: GET /api/v1/auth/verify-email?token=...
func (app *App) VerifyEmail(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing verification token"})
		return
	}

	hash := models.HashToken(token)
	var ev models.EmailVerification
	if err := app.DB.Where("token_hash = ?", hash).First(&ev).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired verification token"})
		return
	}

	if ev.IsExpired() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Verification token has expired"})
		return
	}

	// Mark user as verified
	app.DB.Model(&models.User{}).Where("id = ?", ev.UserID).Update("email_verified", true)

	// Delete used verification token (and any others for this user)
	app.DB.Where("user_id = ?", ev.UserID).Delete(&models.EmailVerification{})

	authLog.Log("VERIFY", "email verified", "user", ev.UserID)
	c.JSON(http.StatusOK, gin.H{"message": "Email verified successfully"})
}

// ResendVerification generates and sends a new verification email.
// Requires authentication. Deletes old tokens first.
//
// Route: POST /api/v1/auth/resend-verification
func (app *App) ResendVerification(c *gin.Context) {
	userID := helpers.GetUserID(c)

	var user models.User
	if err := app.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if user.EmailVerified {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email is already verified"})
		return
	}

	// Delete old tokens for this user
	app.DB.Where("user_id = ?", userID).Delete(&models.EmailVerification{})

	app.sendVerificationEmail(userID, user.Email)
	c.JSON(http.StatusOK, gin.H{"message": "Verification email sent"})
}

// ----- PASSWORD RESET -----

// ForgotPasswordRequest represents the JSON body for a forgot-password request.
type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ForgotPassword generates a password reset token and emails it to the user.
// Anti-enumeration: always returns 200 regardless of whether the email exists.
//
// Route: POST /api/v1/auth/forgot-password
func (app *App) ForgotPassword(c *gin.Context) {
	var req ForgotPasswordRequest
	if !helpers.BindJSON(c, &req) {
		return
	}

	// Always return 200 to prevent email enumeration
	user, found := helpers.FetchUserByEmail(app.DB, req.Email)
	if !found {
		authLog.Log("FORGOT_PW", "email not found (anti-enum, returning 200)", "email", req.Email)
		c.JSON(http.StatusOK, gin.H{"message": "If that email is registered, a reset link has been sent"})
		return
	}

	// Invalidate any existing reset tokens for this user
	app.DB.Where("user_id = ?", user.ID).Delete(&models.PasswordReset{})

	raw, err := models.GenerateSecureToken(models.PasswordResetTokenBytes)
	if err != nil {
		authLog.Log("FORGOT_PW", "failed to generate reset token", "error", err)
		c.JSON(http.StatusOK, gin.H{"message": "If that email is registered, a reset link has been sent"})
		return
	}

	pr := models.PasswordReset{
		UserID:    uint64(user.ID),
		TokenHash: models.HashToken(raw),
		ExpiresAt: time.Now().Add(models.PasswordResetTTL),
	}
	if err := app.DB.Create(&pr).Error; err != nil {
		authLog.Log("FORGOT_PW", "failed to save reset token", "error", err)
		c.JSON(http.StatusOK, gin.H{"message": "If that email is registered, a reset link has been sent"})
		return
	}

	if app.Mailer != nil {
		subject, body := email.PasswordResetEmail(app.BaseURL, raw)
		if err := app.Mailer.Send(user.Email, subject, body); err != nil {
			authLog.Log("FORGOT_PW", "failed to send reset email", "error", err)
		}
	}

	authLog.Log("FORGOT_PW", "reset token created", "user", user.ID)
	c.JSON(http.StatusOK, gin.H{"message": "If that email is registered, a reset link has been sent"})
}

// ResetPasswordRequest represents the JSON body for a password reset request.
type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// ResetPassword validates a password reset token and sets a new password.
// On success, all refresh tokens for the user are revoked (forces re-login).
// The reset token is marked as used (single-use).
//
// Route: POST /api/v1/auth/reset-password
func (app *App) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if !helpers.BindJSON(c, &req) {
		return
	}

	if len(req.NewPassword) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be at least 8 characters"})
		return
	}

	hash := models.HashToken(req.Token)
	var pr models.PasswordReset
	if err := app.DB.Where("token_hash = ? AND used = ?", hash, false).First(&pr).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired reset token"})
		return
	}

	if pr.IsExpired() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Reset token has expired"})
		return
	}

	var user models.User
	if err := app.DB.First(&user, pr.UserID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find user"})
		return
	}

	if err := user.SetPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	app.DB.Model(&user).Update("hash", user.Hash)

	// Mark token as used
	app.DB.Model(&pr).Update("used", true)

	// Revoke all refresh tokens (force re-login everywhere)
	app.DB.Model(&models.RefreshToken{}).Where("user_id = ?", pr.UserID).Update("revoked", true)

	authLog.Log("RESET_PW", "password reset and sessions revoked", "user", pr.UserID)
	c.JSON(http.StatusOK, gin.H{"message": "Password has been reset. Please log in again."})
}

// ----- CHANGE PASSWORD (authenticated) -----

// ChangePasswordRequest represents the JSON body for an authenticated password change.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

// ChangePassword allows an authenticated user to update their password.
// Requires the current password for verification. On success, all other
// refresh tokens are revoked, forcing other sessions to re-authenticate.
//
// Route: POST /api/v1/auth/change-password
func (app *App) ChangePassword(c *gin.Context) {
	userID := helpers.GetUserID(c)

	var req ChangePasswordRequest
	if !helpers.BindJSON(c, &req) {
		return
	}

	if len(req.NewPassword) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "New password must be at least 8 characters"})
		return
	}

	if req.CurrentPassword == req.NewPassword {
		c.JSON(http.StatusBadRequest, gin.H{"error": "New password must be different from current password"})
		return
	}

	var user models.User
	if err := app.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if !user.CheckPassword(req.CurrentPassword) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Current password is incorrect"})
		return
	}

	if err := user.SetPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	app.DB.Model(&user).Update("hash", user.Hash)

	// Revoke all refresh tokens for this user (force re-login everywhere)
	app.DB.Model(&models.RefreshToken{}).Where("user_id = ?", userID).Update("revoked", true)

	authLog.Log("CHANGE_PW", "password changed and sessions revoked", "user", userID)
	c.JSON(http.StatusOK, gin.H{"message": "Password changed successfully. All other sessions have been revoked."})
}
