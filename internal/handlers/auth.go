// Package handlers/auth.go contains the HTTP handlers for authentication operations.
//
// AUTH ARCHITECTURE:
//
//	Signup:   Creates user (unverified) → sends verification email → returns user_id
//	Login:    Verifies credentials → issues short-lived access JWT (15 min) + opaque refresh token (30 days)
//	Refresh:  Validates refresh token → rotates it (revoke old, issue new in same family) → returns new access + refresh
//	          If a revoked token is reused → entire family revoked (token theft detection)
//	Logout:   Revokes a single refresh token
//	LogoutAll: Revokes all refresh tokens for the user
//	VerifyEmail:    Validates verification token → marks user as verified
//	ResendVerify:   Rate-limited re-send of verification email
package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	emailpkg "github.com/Geetur/Notery/internal/email"
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

// Signup handles user registration and account creation.
// Creates the user with EmailVerified=false and sends a verification email.
func (app *App) Signup(c *gin.Context) {
	authLog.Log("SIGNUP", "Processing signup request")

	var authReq AuthRequest
	if !helpers.BindJSON(c, &authReq) {
		authLog.Log("SIGNUP", "Failed to bind JSON request")
		return
	}
	authLog.Log("SIGNUP", "Request validated", "email", authReq.Email)

	// Password strength validation
	if len(authReq.Password) < 8 {
		authLog.Log("SIGNUP", "Password too short")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be at least 8 characters"})
		return
	}

	// Build the user model and hash password
	user := &models.User{Email: authReq.Email, Username: authReq.Username}
	if err := user.SetPassword(authReq.Password); err != nil {
		authLog.Log("SIGNUP", "Failed to hash password", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	// Persist user to database
	if result := app.DB.Create(user); result.Error != nil {
		authLog.Log("SIGNUP", "Failed to create user in database", "error", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user (email already exists?)"})
		return
	}

	// Send verification email (best-effort — signup succeeds even if email fails)
	app.sendVerificationEmail(user)

	authLog.Log("SIGNUP", "User created successfully", "userID", user.ID, "email", authReq.Email)
	c.JSON(http.StatusCreated, gin.H{
		"message": "User created successfully. Please check your email to verify your account.",
		"user_id": user.ID,
	})
}

// Login handles user authentication and issues access + refresh token pair.
func (app *App) Login(c *gin.Context) {
	authLog.Log("LOGIN", "Processing login request")

	var authReq AuthRequest
	if !helpers.BindJSON(c, &authReq) {
		authLog.Log("LOGIN", "Failed to bind JSON request")
		return
	}
	authLog.Log("LOGIN", "Request validated", "email", authReq.Email)

	// Find user by email
	user, found := helpers.FetchUserByEmail(app.DB, authReq.Email)
	if !found {
		authLog.Log("LOGIN", "User not found", "email", authReq.Email)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// Verify password
	if !user.CheckPassword(authReq.Password) {
		authLog.Log("LOGIN", "Invalid password", "email", authReq.Email)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}
	authLog.Log("LOGIN", "Password verified", "userID", user.ID)

	// Issue access token (short-lived JWT)
	accessToken, err := app.issueAccessToken(user.ID)
	if err != nil {
		authLog.Log("LOGIN", "Failed to issue access token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	// Issue refresh token (long-lived opaque token stored as hash in DB)
	refreshToken, err := app.createRefreshToken(uint64(user.ID))
	if err != nil {
		authLog.Log("LOGIN", "Failed to issue refresh token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	authLog.Log("LOGIN", "Login successful", "userID", user.ID, "email", authReq.Email)
	c.JSON(http.StatusOK, gin.H{
		"access_token":   accessToken,
		"refresh_token":  refreshToken,
		"token_type":     "Bearer",
		"expires_in":     int(models.AccessTokenTTL.Seconds()),
		"email_verified": user.EmailVerified,
	})
}

// RefreshToken validates a refresh token, rotates it, and issues a new access token.
// If a revoked token is reused (token theft), the entire family is revoked.
func (app *App) RefreshToken(c *gin.Context) {
	authLog.Log("REFRESH", "Processing token refresh")

	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if !helpers.BindJSON(c, &req) {
		return
	}

	tokenHash := models.HashToken(req.RefreshToken)

	var storedToken models.RefreshToken
	if err := app.DB.Where("token_hash = ?", tokenHash).First(&storedToken).Error; err != nil {
		authLog.Log("REFRESH", "Token not found")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	// Token theft detection: if a revoked token is reused, revoke the ENTIRE family
	if storedToken.Revoked {
		authLog.Log("REFRESH", "REVOKED token reused — revoking family", "familyID", storedToken.FamilyID, "userID", storedToken.UserID)
		app.DB.Model(&models.RefreshToken{}).
			Where("family_id = ?", storedToken.FamilyID).
			Update("revoked", true)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token has been revoked. Please log in again."})
		return
	}

	// Check expiry
	if storedToken.IsExpired() {
		authLog.Log("REFRESH", "Token expired", "userID", storedToken.UserID)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token expired. Please log in again."})
		return
	}

	// Revoke old token
	app.DB.Model(&storedToken).Update("revoked", true)

	// Issue new refresh token in the same family
	newRawToken, err := models.GenerateSecureToken(models.RefreshTokenBytes)
	if err != nil {
		authLog.Log("REFRESH", "Failed to generate new token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to refresh token"})
		return
	}

	newToken := models.RefreshToken{
		TokenHash: models.HashToken(newRawToken),
		UserID:    storedToken.UserID,
		FamilyID:  storedToken.FamilyID, // same family — enables theft detection
		ExpiresAt: time.Now().Add(models.RefreshTokenTTL),
	}
	if err := app.DB.Create(&newToken).Error; err != nil {
		authLog.Log("REFRESH", "Failed to store rotated token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to refresh token"})
		return
	}

	// Issue new access token
	accessToken, err := app.issueAccessToken(uint(storedToken.UserID))
	if err != nil {
		authLog.Log("REFRESH", "Failed to issue access token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	authLog.Log("REFRESH", "Token rotated successfully", "userID", storedToken.UserID)
	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": newRawToken,
		"token_type":    "Bearer",
		"expires_in":    int(models.AccessTokenTTL.Seconds()),
	})
}

// Logout revokes a single refresh token.
func (app *App) Logout(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if !helpers.BindJSON(c, &req) {
		return
	}

	tokenHash := models.HashToken(req.RefreshToken)
	result := app.DB.Model(&models.RefreshToken{}).
		Where("token_hash = ?", tokenHash).
		Update("revoked", true)

	if result.RowsAffected == 0 {
		// Don't reveal whether the token existed — still return success
		authLog.Log("LOGOUT", "Token not found or already revoked")
	} else {
		authLog.Log("LOGOUT", "Token revoked successfully")
	}

	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

// LogoutAll revokes all refresh tokens for the authenticated user.
// Requires authentication (the access token is still valid until it expires).
func (app *App) LogoutAll(c *gin.Context) {
	userID := c.GetUint64("user_id")

	result := app.DB.Model(&models.RefreshToken{}).
		Where("user_id = ? AND revoked = ?", userID, false).
		Update("revoked", true)

	authLog.Log("LOGOUT_ALL", "All sessions revoked", "userID", userID, "count", result.RowsAffected)
	c.JSON(http.StatusOK, gin.H{
		"message":          "All sessions revoked",
		"sessions_revoked": result.RowsAffected,
	})
}

// VerifyEmail validates an email verification token and marks the user as verified.
func (app *App) VerifyEmail(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Verification token required"})
		return
	}

	tokenHash := models.HashToken(token)

	var ev models.EmailVerification
	if err := app.DB.Where("token_hash = ?", tokenHash).First(&ev).Error; err != nil {
		authLog.Log("VERIFY", "Token not found")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired verification token"})
		return
	}

	if ev.IsExpired() {
		// Clean up expired token
		app.DB.Delete(&ev)
		authLog.Log("VERIFY", "Token expired", "userID", ev.UserID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Verification token has expired. Please request a new one."})
		return
	}

	// Mark user as verified + delete all verification tokens for this user
	err := app.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", ev.UserID).Update("email_verified", true).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", ev.UserID).Delete(&models.EmailVerification{}).Error
	})
	if err != nil {
		authLog.Log("VERIFY", "Failed to verify user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify email"})
		return
	}

	authLog.Log("VERIFY", "Email verified successfully", "userID", ev.UserID)
	c.JSON(http.StatusOK, gin.H{"message": "Email verified successfully"})
}

// ResendVerification sends a new verification email to the authenticated user.
func (app *App) ResendVerification(c *gin.Context) {
	userID := c.GetUint64("user_id")

	var user models.User
	if err := app.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if user.EmailVerified {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email already verified"})
		return
	}

	// Delete existing verification tokens for this user
	app.DB.Where("user_id = ?", userID).Delete(&models.EmailVerification{})

	// Send new verification email
	app.sendVerificationEmail(&user)

	c.JSON(http.StatusOK, gin.H{"message": "Verification email sent"})
}

// ===== INTERNAL HELPERS =====

// issueAccessToken creates a short-lived JWT access token.
func (app *App) issueAccessToken(userID uint) (string, error) {
	secretKey := []byte(app.JWTSecret)
	if len(secretKey) == 0 {
		authLog.Log("TOKEN", "JWT_SECRET not configured")
		return "", fmt.Errorf("JWT secret not configured")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": fmt.Sprint(userID),
		"exp":     time.Now().Add(models.AccessTokenTTL).Unix(),
		"iat":     time.Now().Unix(),
	})
	return token.SignedString(secretKey)
}

// createRefreshToken generates an opaque refresh token, stores its hash in the DB,
// and returns the raw token to send to the client.
func (app *App) createRefreshToken(userID uint64) (string, error) {
	rawToken, err := models.GenerateSecureToken(models.RefreshTokenBytes)
	if err != nil {
		return "", err
	}

	// Each new login starts a new token family
	familyID, err := models.GenerateSecureToken(16)
	if err != nil {
		return "", err
	}

	rt := models.RefreshToken{
		TokenHash: models.HashToken(rawToken),
		UserID:    userID,
		FamilyID:  familyID,
		ExpiresAt: time.Now().Add(models.RefreshTokenTTL),
	}
	if err := app.DB.Create(&rt).Error; err != nil {
		return "", err
	}

	return rawToken, nil
}

// sendVerificationEmail generates a verification token and sends the email.
// This is best-effort: signup/resend succeeds even if the email fails to send.
func (app *App) sendVerificationEmail(user *models.User) {
	if app.Mailer == nil {
		authLog.Log("VERIFY", "Mailer not configured, skipping verification email")
		return
	}

	rawToken, err := models.GenerateSecureToken(models.EmailVerificationTokenBytes)
	if err != nil {
		authLog.Log("VERIFY", "Failed to generate verification token", "error", err)
		return
	}

	ev := models.EmailVerification{
		UserID:    uint64(user.ID),
		TokenHash: models.HashToken(rawToken),
		ExpiresAt: time.Now().Add(models.EmailVerificationTTL),
	}
	if err := app.DB.Create(&ev).Error; err != nil {
		authLog.Log("VERIFY", "Failed to store verification token", "error", err)
		return
	}

	baseURL := "http://localhost:8080"
	if app.Config != nil {
		baseURL = app.Config.BaseURL
	}

	subject, body := emailpkg.VerificationEmail(baseURL, rawToken)
	if err := app.Mailer.Send(user.Email, subject, body); err != nil {
		authLog.Log("VERIFY", "Failed to send verification email", "error", err, "email", user.Email)
	} else {
		authLog.Log("VERIFY", "Verification email sent", "email", user.Email)
	}
}

// ===== PASSWORD RESET FLOW =====

// ForgotPassword initiates the password reset flow by sending a reset email.
// Always returns 200 to prevent email enumeration (even if user doesn't exist).
func (app *App) ForgotPassword(c *gin.Context) {
	authLog.Log("FORGOT_PASSWORD", "Processing forgot password request")

	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if !helpers.BindJSON(c, &req) {
		return
	}

	// Always return success to prevent email enumeration
	successResponse := gin.H{"message": "If an account with that email exists, a password reset link has been sent."}

	user, found := helpers.FetchUserByEmail(app.DB, req.Email)
	if !found {
		authLog.Log("FORGOT_PASSWORD", "Email not found (returning success to prevent enumeration)", "email", req.Email)
		c.JSON(http.StatusOK, successResponse)
		return
	}

	if app.Mailer == nil {
		authLog.Log("FORGOT_PASSWORD", "Mailer not configured")
		c.JSON(http.StatusOK, successResponse)
		return
	}

	// Delete any existing reset tokens for this user (only one active at a time)
	app.DB.Where("user_id = ?", user.ID).Delete(&models.PasswordReset{})

	// Generate reset token
	rawToken, err := models.GenerateSecureToken(models.PasswordResetTokenBytes)
	if err != nil {
		authLog.Log("FORGOT_PASSWORD", "Failed to generate reset token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process request"})
		return
	}

	pr := models.PasswordReset{
		UserID:    uint64(user.ID),
		TokenHash: models.HashToken(rawToken),
		ExpiresAt: time.Now().Add(models.PasswordResetTTL),
	}
	if err := app.DB.Create(&pr).Error; err != nil {
		authLog.Log("FORGOT_PASSWORD", "Failed to store reset token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process request"})
		return
	}

	baseURL := "http://localhost:8080"
	if app.Config != nil {
		baseURL = app.Config.BaseURL
	}

	subject, body := emailpkg.PasswordResetEmail(baseURL, rawToken)
	if err := app.Mailer.Send(user.Email, subject, body); err != nil {
		authLog.Log("FORGOT_PASSWORD", "Failed to send reset email", "error", err)
	} else {
		authLog.Log("FORGOT_PASSWORD", "Reset email sent", "email", user.Email)
	}

	c.JSON(http.StatusOK, successResponse)
}

// ResetPassword validates a reset token and sets a new password.
// Also revokes all refresh tokens for the user (force re-login everywhere).
func (app *App) ResetPassword(c *gin.Context) {
	authLog.Log("RESET_PASSWORD", "Processing password reset")

	var req struct {
		Token       string `json:"token" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if !helpers.BindJSON(c, &req) {
		return
	}

	// Password strength validation
	if len(req.NewPassword) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be at least 8 characters"})
		return
	}

	tokenHash := models.HashToken(req.Token)

	var pr models.PasswordReset
	if err := app.DB.Where("token_hash = ?", tokenHash).First(&pr).Error; err != nil {
		authLog.Log("RESET_PASSWORD", "Token not found")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired reset token"})
		return
	}

	if pr.Used {
		authLog.Log("RESET_PASSWORD", "Token already used", "userID", pr.UserID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "This reset token has already been used"})
		return
	}

	if pr.IsExpired() {
		app.DB.Delete(&pr)
		authLog.Log("RESET_PASSWORD", "Token expired", "userID", pr.UserID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Reset token has expired. Please request a new one."})
		return
	}

	// Fetch user and update password in a transaction
	err := app.DB.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.First(&user, pr.UserID).Error; err != nil {
			return err
		}

		if err := user.SetPassword(req.NewPassword); err != nil {
			return err
		}

		if err := tx.Model(&user).Update("hash", user.Hash).Error; err != nil {
			return err
		}

		// Mark token as used
		if err := tx.Model(&pr).Update("used", true).Error; err != nil {
			return err
		}

		// Delete all reset tokens for this user
		if err := tx.Where("user_id = ?", pr.UserID).Delete(&models.PasswordReset{}).Error; err != nil {
			return err
		}

		// Revoke all refresh tokens (force re-login everywhere)
		if err := tx.Model(&models.RefreshToken{}).
			Where("user_id = ? AND revoked = ?", pr.UserID, false).
			Update("revoked", true).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		authLog.Log("RESET_PASSWORD", "Failed to reset password", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset password"})
		return
	}

	authLog.Log("RESET_PASSWORD", "Password reset successfully", "userID", pr.UserID)
	c.JSON(http.StatusOK, gin.H{"message": "Password reset successfully. Please log in with your new password."})
}

// ChangePassword allows an authenticated user to change their password.
// Requires the current password for verification.
func (app *App) ChangePassword(c *gin.Context) {
	userID := c.GetUint64("user_id")
	authLog.Log("CHANGE_PASSWORD", "Processing password change", "userID", userID)

	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required"`
	}
	if !helpers.BindJSON(c, &req) {
		return
	}

	// New password strength validation
	if len(req.NewPassword) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "New password must be at least 8 characters"})
		return
	}

	var user models.User
	if err := app.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Verify current password
	if !user.CheckPassword(req.CurrentPassword) {
		authLog.Log("CHANGE_PASSWORD", "Current password incorrect", "userID", userID)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Current password is incorrect"})
		return
	}

	// Prevent setting the same password
	if user.CheckPassword(req.NewPassword) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "New password must be different from current password"})
		return
	}

	// Update password
	if err := user.SetPassword(req.NewPassword); err != nil {
		authLog.Log("CHANGE_PASSWORD", "Failed to hash new password", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to change password"})
		return
	}

	if err := app.DB.Model(&user).Update("hash", user.Hash).Error; err != nil {
		authLog.Log("CHANGE_PASSWORD", "Failed to update password", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to change password"})
		return
	}

	// Revoke all refresh tokens except current session (force re-login on other devices)
	app.DB.Model(&models.RefreshToken{}).
		Where("user_id = ? AND revoked = ?", userID, false).
		Update("revoked", true)

	authLog.Log("CHANGE_PASSWORD", "Password changed successfully", "userID", userID)
	c.JSON(http.StatusOK, gin.H{"message": "Password changed successfully. All other sessions have been revoked."})
}
