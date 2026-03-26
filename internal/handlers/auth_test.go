package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Geetur/Notery/internal/email"
	"github.com/Geetur/Notery/internal/models"
)

// testAppWithMailer creates a test app with a MockMailer for email assertions.
func testAppWithMailer(t *testing.T) (*App, *email.MockMailer) {
	t.Helper()
	app := testApp(t)
	app.JWTSecret = "test-secret-key"
	app.BaseURL = "http://localhost:8080"
	app.FrontendURL = "http://localhost:3000"
	mock := &email.MockMailer{}
	app.Mailer = mock
	return app, mock
}

// extractRefreshCookie extracts the refresh token from the Set-Cookie header.
func extractRefreshCookie(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == refreshTokenCookieName {
			return c.Value
		}
	}
	t.Fatal("expected refresh token cookie in response")
	return ""
}

// ===== SIGNUP TESTS =====

func TestSignup_HappyPath(t *testing.T) {
	app, mock := testAppWithMailer(t)

	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"email":           "new@test.com",
			"password":        "Securepass1",
			"username":        "newuser",
			"agreed_to_terms": true,
		}), app.Signup)
	assertStatus(t, w, http.StatusCreated)
	r := respJSON(t, w)
	if r["user_id"] == nil {
		t.Fatal("expected user_id in response")
	}
	if r["access_token"] == nil {
		t.Fatal("expected access_token in response")
	}
	extractRefreshCookie(t, w) // verify cookie is set

	// Check verification email was sent (async goroutine — brief sleep needed)
	time.Sleep(100 * time.Millisecond)
	if len(mock.Sent) != 1 {
		t.Fatalf("expected 1 email sent, got %d", len(mock.Sent))
	}
	if mock.Sent[0].To != "new@test.com" {
		t.Fatalf("expected email to new@test.com, got %s", mock.Sent[0].To)
	}
}

func TestSignup_MissingEmail(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"password":        "Securepass1",
			"agreed_to_terms": true,
		}), app.Signup)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestSignup_MissingPassword(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"email":           "test@test.com",
			"agreed_to_terms": true,
		}), app.Signup)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestSignup_InvalidEmail(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"email":           "not-an-email",
			"password":        "Securepass1",
			"agreed_to_terms": true,
		}), app.Signup)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestSignup_DuplicateEmail(t *testing.T) {
	app, _ := testAppWithMailer(t)

	serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"email":           "dupe@test.com",
			"password":        "Securepass1",
			"agreed_to_terms": true,
		}), app.Signup)

	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"email":           "dupe@test.com",
			"password":        "Different1",
			"agreed_to_terms": true,
		}), app.Signup)
	assertStatus(t, w, http.StatusInternalServerError)
}

func TestSignup_EmptyBody(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/signup", "/signup", jsonBody(map[string]string{}), app.Signup)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestSignup_UsernameOptional(t *testing.T) {
	app, _ := testAppWithMailer(t)
	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"email":           "nouser@test.com",
			"password":        "Securepass1",
			"agreed_to_terms": true,
		}), app.Signup)
	assertStatus(t, w, http.StatusCreated)
}

func TestSignup_PasswordTooShort(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"email":           "short@test.com",
			"password":        "1234567",
			"agreed_to_terms": true,
		}), app.Signup)
	assertStatus(t, w, http.StatusBadRequest)
	r := respJSON(t, w)
	if r["error"] != "Password must be at least 8 characters" {
		t.Fatalf("unexpected error: %v", r["error"])
	}
}

func TestSignup_PasswordExactly8Chars(t *testing.T) {
	app, _ := testAppWithMailer(t)
	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"email":           "exact8@test.com",
			"password":        "Abcdefg1",
			"agreed_to_terms": true,
		}), app.Signup)
	assertStatus(t, w, http.StatusCreated)
}

func TestSignup_NoMailer_StillSucceeds(t *testing.T) {
	app := testApp(t)
	app.JWTSecret = "test-secret-key"
	// Mailer is nil — signup should still succeed without sending email

	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"email":           "nomailer@test.com",
			"password":        "Securepass1",
			"agreed_to_terms": true,
		}), app.Signup)
	assertStatus(t, w, http.StatusCreated)
	r := respJSON(t, w)
	if r["access_token"] == nil {
		t.Fatal("expected access_token even without mailer")
	}
}

// ===== LOGIN TESTS =====

func TestLogin_HappyPath(t *testing.T) {
	app, _ := testAppWithMailer(t)

	seedUser(t, app.DB, "loginuser")

	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "loginuser@test.com",
			"password": "test123",
		}), app.Login)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["access_token"] == nil {
		t.Fatal("expected access_token in response")
	}
	extractRefreshCookie(t, w) // verify cookie is set
	accessToken, ok := r["access_token"].(string)
	if !ok || accessToken == "" {
		t.Fatal("access_token should be a non-empty string")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	app, _ := testAppWithMailer(t)

	seedUser(t, app.DB, "wrongpw")

	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "wrongpw@test.com",
			"password": "wrongpassword",
		}), app.Login)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestLogin_NonexistentUser(t *testing.T) {
	app, _ := testAppWithMailer(t)

	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "ghost@test.com",
			"password": "securepass123",
		}), app.Login)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestLogin_MissingEmail(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"password": "securepass123",
		}), app.Login)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestLogin_MissingPassword(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email": "test@test.com",
		}), app.Login)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestLogin_EmptyJWTSecret(t *testing.T) {
	app := testApp(t)
	app.JWTSecret = ""

	seedUser(t, app.DB, "emptykey")

	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "emptykey@test.com",
			"password": "test123",
		}), app.Login)
	assertStatus(t, w, http.StatusInternalServerError)
}

func TestLogin_EmptyBody(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/login", "/login", jsonBody(map[string]string{}), app.Login)
	assertStatus(t, w, http.StatusBadRequest)
}

// ===== PASSWORD HASHING EDGE CASES =====

func TestSignup_PasswordStoredAsHash(t *testing.T) {
	app, _ := testAppWithMailer(t)

	serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"email":           "hashcheck@test.com",
			"password":        "Securepass1",
			"username":        "hashcheck",
			"agreed_to_terms": true,
		}), app.Signup)

	var user models.User
	app.DB.Where("email = ?", "hashcheck@test.com").First(&user)
	if user.Hash == "" {
		t.Fatal("password hash should be stored")
	}
	if user.Hash == "Securepass1" {
		t.Fatal("password must not be stored in plaintext")
	}
}

func TestSignup_MissingTermsAgreement(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"email":    "notos@test.com",
			"password": "Securepass1",
		}), app.Signup)
	assertStatus(t, w, http.StatusBadRequest)
	r := respJSON(t, w)
	if r["error"] != "You must agree to the Terms of Service" {
		t.Fatalf("unexpected error: %v", r["error"])
	}
}

func TestSignup_MultipleUsers_UniqueIDs(t *testing.T) {
	app, _ := testAppWithMailer(t)

	users := make(map[float64]bool)
	for i := 0; i < 5; i++ {
		w := serve("POST", "/signup", "/signup",
			jsonBody(map[string]interface{}{
				"email":           fmt.Sprintf("multi%d@test.com", i),
				"password":        "Securepass1",
				"agreed_to_terms": true,
			}), app.Signup)
		assertStatus(t, w, http.StatusCreated)
		r := respJSON(t, w)
		uid := r["user_id"].(float64)
		if users[uid] {
			t.Fatalf("duplicate user_id: %v", uid)
		}
		users[uid] = true
	}
}

// ===== REFRESH TOKEN TESTS =====

func TestRefresh_HappyPath(t *testing.T) {
	app, _ := testAppWithMailer(t)

	seedUser(t, app.DB, "refreshuser")

	// Login to get a refresh token
	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "refreshuser@test.com",
			"password": "test123",
		}), app.Login)
	assertStatus(t, w, http.StatusOK)
	refreshToken := extractRefreshCookie(t, w)

	// Use the refresh token to get new tokens
	w = serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": refreshToken,
		}), app.RefreshAccessToken)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["access_token"] == nil {
		t.Fatal("expected new access_token")
	}
	newRefresh := extractRefreshCookie(t, w)
	if newRefresh == refreshToken {
		t.Fatal("new refresh token should differ from old one (rotation)")
	}
}

func TestRefresh_InvalidToken(t *testing.T) {
	app := testApp(t)

	w := serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": "nonexistent-token",
		}), app.RefreshAccessToken)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestRefresh_RevokedTokenTriggersTheftDetection(t *testing.T) {
	app, _ := testAppWithMailer(t)

	seedUser(t, app.DB, "theftuser")

	// Login to get refresh token
	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "theftuser@test.com",
			"password": "test123",
		}), app.Login)
	originalRefresh := extractRefreshCookie(t, w)

	// Rotate the token (use it once)
	w = serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": originalRefresh,
		}), app.RefreshAccessToken)
	assertStatus(t, w, http.StatusOK)
	newRefresh := extractRefreshCookie(t, w)

	// Reuse the original (now revoked) token → theft detection
	w = serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": originalRefresh,
		}), app.RefreshAccessToken)
	assertStatus(t, w, http.StatusUnauthorized)
	r := respJSON(t, w)
	if r["error"] == nil {
		t.Fatal("expected error about token reuse")
	}

	// The new token should also be revoked (family invalidated)
	w = serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": newRefresh,
		}), app.RefreshAccessToken)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestRefresh_MissingField(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{}), app.RefreshAccessToken)
	assertStatus(t, w, http.StatusUnauthorized)
}

// ===== LOGOUT TESTS =====

func TestLogout_HappyPath(t *testing.T) {
	app, _ := testAppWithMailer(t)

	seedUser(t, app.DB, "logoutuser")

	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "logoutuser@test.com",
			"password": "test123",
		}), app.Login)
	refreshToken := extractRefreshCookie(t, w)

	// Logout
	w = serve("POST", "/logout", "/logout",
		jsonBody(map[string]string{
			"refresh_token": refreshToken,
		}), app.Logout)
	assertStatus(t, w, http.StatusOK)

	// Refresh with the logged-out token should fail
	w = serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": refreshToken,
		}), app.RefreshAccessToken)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestLogout_InvalidToken_StillReturns200(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/logout", "/logout",
		jsonBody(map[string]string{
			"refresh_token": "fake-token",
		}), app.Logout)
	// Always returns 200 to prevent token enumeration
	assertStatus(t, w, http.StatusOK)
}

func TestLogoutAll_RevokesAllSessions(t *testing.T) {
	app, _ := testAppWithMailer(t)

	uid := seedUser(t, app.DB, "logoutalluser")

	// Login twice to create 2 sessions
	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "logoutalluser@test.com",
			"password": "test123",
		}), app.Login)
	refresh1 := extractRefreshCookie(t, w)

	w = serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "logoutalluser@test.com",
			"password": "test123",
		}), app.Login)
	refresh2 := extractRefreshCookie(t, w)

	// Logout all (requires auth)
	w = serve("POST", "/logout-all", "/logout-all",
		nil, app.LogoutAll, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Both tokens should be revoked
	w = serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": refresh1,
		}), app.RefreshAccessToken)
	assertStatus(t, w, http.StatusUnauthorized)

	w = serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": refresh2,
		}), app.RefreshAccessToken)
	assertStatus(t, w, http.StatusUnauthorized)
}

// ===== EMAIL VERIFICATION TESTS =====

func TestVerifyEmail_HappyPath(t *testing.T) {
	app, mock := testAppWithMailer(t)

	// Signup to trigger verification email
	serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"email":           "verify@test.com",
			"password":        "Securepass1",
			"agreed_to_terms": true,
		}), app.Signup)

	// Extract token from the email body (it's in the URL)
	time.Sleep(100 * time.Millisecond)
	if len(mock.Sent) < 1 {
		t.Fatal("expected verification email to be sent")
	}

	// Find the token in the DB instead of parsing email
	var ev models.EmailVerification
	if err := app.DB.Where("user_id = ?", 1).First(&ev).Error; err != nil {
		t.Fatalf("expected email verification record: %v", err)
	}

	// Generate the raw token to use — we need to find it by querying with hash
	// Since we can't reverse the hash, let's use a direct approach:
	// Create a new token manually for testing
	rawToken, _ := models.GenerateSecureToken(models.EmailVerificationTokenBytes)
	app.DB.Model(&ev).Update("token_hash", models.HashToken(rawToken))

	w := serve("GET", "/verify-email", "/verify-email?token="+rawToken,
		nil, app.VerifyEmail)
	assertStatus(t, w, http.StatusOK)

	// Verify user is now marked as verified
	var user models.User
	app.DB.First(&user, ev.UserID)
	if !user.EmailVerified {
		t.Fatal("user should be marked as email_verified")
	}
}

func TestVerifyEmail_InvalidToken(t *testing.T) {
	app := testApp(t)
	w := serve("GET", "/verify-email", "/verify-email?token=invalid",
		nil, app.VerifyEmail)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestVerifyEmail_MissingToken(t *testing.T) {
	app := testApp(t)
	w := serve("GET", "/verify-email", "/verify-email",
		nil, app.VerifyEmail)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestVerifyEmail_ExpiredToken(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "expiredverify")

	rawToken, _ := models.GenerateSecureToken(models.EmailVerificationTokenBytes)
	ev := models.EmailVerification{
		UserID:    uid,
		TokenHash: models.HashToken(rawToken),
		ExpiresAt: models.ZeroTime(), // expired
	}
	app.DB.Create(&ev)

	w := serve("GET", "/verify-email", "/verify-email?token="+rawToken,
		nil, app.VerifyEmail)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestResendVerification_HappyPath(t *testing.T) {
	app, mock := testAppWithMailer(t)
	uid := seedUser(t, app.DB, "resenduser")

	// User is not verified by default
	w := serve("POST", "/resend", "/resend",
		nil, app.ResendVerification, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Check email was sent (async goroutine — brief sleep needed)
	time.Sleep(100 * time.Millisecond)
	if len(mock.Sent) < 1 {
		t.Fatal("expected verification email to be sent")
	}
}

func TestResendVerification_AlreadyVerified(t *testing.T) {
	app, _ := testAppWithMailer(t)
	uid := seedUser(t, app.DB, "alreadyverified")
	app.DB.Model(&models.User{}).Where("id = ?", uid).Update("email_verified", true)

	w := serve("POST", "/resend", "/resend",
		nil, app.ResendVerification, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

// ===== PASSWORD RESET TESTS =====

func TestForgotPassword_HappyPath(t *testing.T) {
	app, mock := testAppWithMailer(t)
	seedUser(t, app.DB, "forgotuser")

	w := serve("POST", "/forgot", "/forgot",
		jsonBody(map[string]string{
			"email": "forgotuser@test.com",
		}), app.ForgotPassword)
	assertStatus(t, w, http.StatusOK)

	time.Sleep(100 * time.Millisecond)
	if len(mock.Sent) < 1 {
		t.Fatal("expected password reset email to be sent")
	}
	if mock.Sent[0].To != "forgotuser@test.com" {
		t.Fatalf("expected email to forgotuser@test.com, got %s", mock.Sent[0].To)
	}
}

func TestForgotPassword_NonexistentEmail_StillReturns200(t *testing.T) {
	app, mock := testAppWithMailer(t)

	w := serve("POST", "/forgot", "/forgot",
		jsonBody(map[string]string{
			"email": "noone@test.com",
		}), app.ForgotPassword)
	// Anti-enumeration: always 200
	assertStatus(t, w, http.StatusOK)

	// No email should be sent
	if len(mock.Sent) != 0 {
		t.Fatal("no email should be sent for nonexistent users")
	}
}

func TestForgotPassword_EmailContainsFrontendURL(t *testing.T) {
	app, mock := testAppWithMailer(t)
	seedUser(t, app.DB, "reseturl")

	w := serve("POST", "/forgot", "/forgot",
		jsonBody(map[string]string{
			"email": "reseturl@test.com",
		}), app.ForgotPassword)
	assertStatus(t, w, http.StatusOK)

	time.Sleep(100 * time.Millisecond)
	if len(mock.Sent) < 1 {
		t.Fatal("expected password reset email to be sent")
	}

	body := mock.Sent[0].Body
	// Must link to the frontend, NOT the backend API
	if !strings.Contains(body, app.FrontendURL+"/reset-password?token=") {
		t.Fatalf("password reset email must contain frontend URL (%s), got body: %s", app.FrontendURL, body)
	}
	// Must NOT contain the backend API URL in reset links
	if strings.Contains(body, app.BaseURL+"/reset-password") {
		t.Fatal("password reset email must NOT contain backend BaseURL for reset links")
	}
}

func TestForgotPassword_EmailNotSentWithNilMailer(t *testing.T) {
	app := testApp(t)
	app.Mailer = nil
	app.FrontendURL = "http://localhost:3000"
	seedUser(t, app.DB, "nilmailer")

	w := serve("POST", "/forgot", "/forgot",
		jsonBody(map[string]string{
			"email": "nilmailer@test.com",
		}), app.ForgotPassword)
	assertStatus(t, w, http.StatusOK) // Still 200 (anti-enumeration)
}

func TestVerificationEmail_UsesBackendURL(t *testing.T) {
	app, mock := testAppWithMailer(t)

	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"email":           "verifyurl@test.com",
			"password":        "Securepass1",
			"username":        "verifyurl",
			"agreed_to_terms": true,
		}), app.Signup)
	assertStatus(t, w, http.StatusCreated)

	// sendVerificationEmail fires the mailer in a goroutine; give it time to complete.
	time.Sleep(100 * time.Millisecond)

	if len(mock.Sent) < 1 {
		t.Fatal("expected verification email to be sent")
	}
	body := mock.Sent[0].Body
	// Verification links must point to the backend API endpoint
	if !strings.Contains(body, app.BaseURL+"/api/v1/auth/verify-email?token=") {
		t.Fatalf("verification email must contain backend URL (%s), got body: %s", app.BaseURL, body)
	}
}

func TestResetPassword_HappyPath(t *testing.T) {
	app, _ := testAppWithMailer(t)
	uid := seedUser(t, app.DB, "resetuser")

	// Create a reset token manually
	rawToken, _ := models.GenerateSecureToken(models.PasswordResetTokenBytes)
	pr := models.PasswordReset{
		UserID:    uid,
		TokenHash: models.HashToken(rawToken),
		ExpiresAt: models.FutureTime(),
	}
	app.DB.Create(&pr)

	w := serve("POST", "/reset", "/reset",
		jsonBody(map[string]string{
			"token":        rawToken,
			"new_password": "Newpassword1",
		}), app.ResetPassword)
	assertStatus(t, w, http.StatusOK)

	// Verify old password no longer works
	var user models.User
	app.DB.First(&user, uid)
	if user.CheckPassword("test123") {
		t.Fatal("old password should not work after reset")
	}
	if !user.CheckPassword("Newpassword1") {
		t.Fatal("new password should work after reset")
	}
}

func TestResetPassword_InvalidToken(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/reset", "/reset",
		jsonBody(map[string]string{
			"token":        "invalid-token",
			"new_password": "Newpassword1",
		}), app.ResetPassword)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestResetPassword_ExpiredToken(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "expiredreset")

	rawToken, _ := models.GenerateSecureToken(models.PasswordResetTokenBytes)
	pr := models.PasswordReset{
		UserID:    uid,
		TokenHash: models.HashToken(rawToken),
		ExpiresAt: models.ZeroTime(), // expired
	}
	app.DB.Create(&pr)

	w := serve("POST", "/reset", "/reset",
		jsonBody(map[string]string{
			"token":        rawToken,
			"new_password": "Newpassword1",
		}), app.ResetPassword)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestResetPassword_UsedToken(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "usedreset")

	rawToken, _ := models.GenerateSecureToken(models.PasswordResetTokenBytes)
	pr := models.PasswordReset{
		UserID:    uid,
		TokenHash: models.HashToken(rawToken),
		Used:      true,
		ExpiresAt: models.FutureTime(),
	}
	app.DB.Create(&pr)

	w := serve("POST", "/reset", "/reset",
		jsonBody(map[string]string{
			"token":        rawToken,
			"new_password": "Newpassword1",
		}), app.ResetPassword)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestResetPassword_ShortPassword(t *testing.T) {
	app := testApp(t)

	w := serve("POST", "/reset", "/reset",
		jsonBody(map[string]string{
			"token":        "any-token",
			"new_password": "short",
		}), app.ResetPassword)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestResetPassword_RevokesAllSessions(t *testing.T) {
	app, _ := testAppWithMailer(t)
	uid := seedUser(t, app.DB, "resetrevoke")

	// Login to create a session
	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "resetrevoke@test.com",
			"password": "test123",
		}), app.Login)
	refreshToken := extractRefreshCookie(t, w)

	// Create reset token and reset password
	rawToken, _ := models.GenerateSecureToken(models.PasswordResetTokenBytes)
	pr := models.PasswordReset{
		UserID:    uid,
		TokenHash: models.HashToken(rawToken),
		ExpiresAt: models.FutureTime(),
	}
	app.DB.Create(&pr)

	serve("POST", "/reset", "/reset",
		jsonBody(map[string]string{
			"token":        rawToken,
			"new_password": "Newpassword1",
		}), app.ResetPassword)

	// Old refresh token should be revoked
	w = serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": refreshToken,
		}), app.RefreshAccessToken)
	assertStatus(t, w, http.StatusUnauthorized)
}

// ===== CHANGE PASSWORD TESTS =====

func TestChangePassword_HappyPath(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "changeuser")

	w := serve("POST", "/change", "/change",
		jsonBody(map[string]string{
			"current_password": "test123",
			"new_password":     "Newpassword1",
		}), app.ChangePassword, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Verify new password works
	var user models.User
	app.DB.First(&user, uid)
	if !user.CheckPassword("Newpassword1") {
		t.Fatal("new password should work")
	}
}

func TestChangePassword_WrongCurrentPassword(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "wrongcurrent")

	w := serve("POST", "/change", "/change",
		jsonBody(map[string]string{
			"current_password": "wrongpassword",
			"new_password":     "Newpassword1",
		}), app.ChangePassword, authMW(uid))
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestChangePassword_SameAsOld(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "samepass")

	w := serve("POST", "/change", "/change",
		jsonBody(map[string]string{
			"current_password": "test123",
			"new_password":     "test123",
		}), app.ChangePassword, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestChangePassword_TooShort(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "shortchange")

	w := serve("POST", "/change", "/change",
		jsonBody(map[string]string{
			"current_password": "test123",
			"new_password":     "short",
		}), app.ChangePassword, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestChangePassword_RevokesAllSessions(t *testing.T) {
	app, _ := testAppWithMailer(t)
	uid := seedUser(t, app.DB, "changerevoke")

	// Login to create a session
	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "changerevoke@test.com",
			"password": "test123",
		}), app.Login)
	refreshToken := extractRefreshCookie(t, w)

	// Change password
	serve("POST", "/change", "/change",
		jsonBody(map[string]string{
			"current_password": "test123",
			"new_password":     "Newpassword1",
		}), app.ChangePassword, authMW(uid))

	// Old refresh token should be revoked
	w = serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": refreshToken,
		}), app.RefreshAccessToken)
	assertStatus(t, w, http.StatusUnauthorized)
}
