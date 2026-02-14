package handlers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/Geetur/Notery/internal/email"
	"github.com/Geetur/Notery/internal/models"
)

// ===== SIGNUP TESTS =====

func TestSignup_HappyPath(t *testing.T) {
	app := testApp(t)
	app.Mailer = &email.MockMailer{}

	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "new@test.com",
			"password": "securepass123",
			"username": "newuser",
		}), app.Signup)
	assertStatus(t, w, http.StatusCreated)
	r := respJSON(t, w)
	if r["user_id"] == nil {
		t.Fatal("expected user_id in response")
	}
	if r["message"] == nil {
		t.Fatal("expected message in response")
	}
}

func TestSignup_SendsVerificationEmail(t *testing.T) {
	app := testApp(t)
	mock := &email.MockMailer{}
	app.Mailer = mock

	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "verify@test.com",
			"password": "securepass123",
		}), app.Signup)
	assertStatus(t, w, http.StatusCreated)

	if len(mock.Sent) != 1 {
		t.Fatalf("expected 1 email sent, got %d", len(mock.Sent))
	}
	if mock.Sent[0].To != "verify@test.com" {
		t.Fatalf("email sent to wrong address: %s", mock.Sent[0].To)
	}
	if mock.Sent[0].Subject != "Verify your Notery account" {
		t.Fatalf("unexpected subject: %s", mock.Sent[0].Subject)
	}
}

func TestSignup_CreatesVerificationToken(t *testing.T) {
	app := testApp(t)
	app.Mailer = &email.MockMailer{}

	serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "tokencheck@test.com",
			"password": "securepass123",
		}), app.Signup)

	var count int64
	app.DB.Model(&models.EmailVerification{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 verification token, got %d", count)
	}
}

func TestSignup_UserNotVerifiedByDefault(t *testing.T) {
	app := testApp(t)
	app.Mailer = &email.MockMailer{}

	serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "unverified@test.com",
			"password": "securepass123",
		}), app.Signup)

	var user models.User
	app.DB.Where("email = ?", "unverified@test.com").First(&user)
	if user.EmailVerified {
		t.Fatal("user should NOT be verified after signup")
	}
}

func TestSignup_MissingEmail(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"password": "securepass123",
		}), app.Signup)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestSignup_MissingPassword(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email": "test@test.com",
		}), app.Signup)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestSignup_InvalidEmail(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "not-an-email",
			"password": "securepass123",
		}), app.Signup)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestSignup_DuplicateEmail(t *testing.T) {
	app := testApp(t)

	serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "dupe@test.com",
			"password": "securepass123",
		}), app.Signup)

	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "dupe@test.com",
			"password": "differentpass",
		}), app.Signup)
	assertStatus(t, w, http.StatusInternalServerError)
}

func TestSignup_EmptyBody(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/signup", "/signup", jsonBody(map[string]string{}), app.Signup)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestSignup_UsernameOptional(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "nouser@test.com",
			"password": "securepass123",
		}), app.Signup)
	assertStatus(t, w, http.StatusCreated)
}

func TestSignup_PasswordTooShort(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "short@test.com",
			"password": "1234567",
		}), app.Signup)
	assertStatus(t, w, http.StatusBadRequest)
	r := respJSON(t, w)
	if r["error"] != "Password must be at least 8 characters" {
		t.Fatalf("unexpected error: %v", r["error"])
	}
}

func TestSignup_PasswordExactly8Chars(t *testing.T) {
	app := testApp(t)
	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "exact8@test.com",
			"password": "12345678",
		}), app.Signup)
	assertStatus(t, w, http.StatusCreated)
}

// ===== LOGIN TESTS =====

func TestLogin_HappyPath(t *testing.T) {
	app := testApp(t)
	app.JWTSecret = "test-secret-key"

	seedUser(t, app.DB, "loginuser")

	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "loginuser@test.com",
			"password": "test123",
		}), app.Login)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)

	// New auth system returns access_token + refresh_token
	if r["access_token"] == nil {
		t.Fatal("expected access_token in response")
	}
	if r["refresh_token"] == nil {
		t.Fatal("expected refresh_token in response")
	}
	if r["token_type"] != "Bearer" {
		t.Fatalf("expected token_type=Bearer, got %v", r["token_type"])
	}
	if r["expires_in"] == nil {
		t.Fatal("expected expires_in in response")
	}
}

func TestLogin_CreatesRefreshTokenInDB(t *testing.T) {
	app := testApp(t)
	app.JWTSecret = "test-secret-key"

	seedUser(t, app.DB, "loginrt")

	serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "loginrt@test.com",
			"password": "test123",
		}), app.Login)

	var count int64
	app.DB.Model(&models.RefreshToken{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 refresh token in DB, got %d", count)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	app := testApp(t)
	app.JWTSecret = "test-secret-key"

	seedUser(t, app.DB, "wrongpw")

	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "wrongpw@test.com",
			"password": "wrongpassword",
		}), app.Login)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestLogin_NonexistentUser(t *testing.T) {
	app := testApp(t)
	app.JWTSecret = "test-secret-key"

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

// ===== REFRESH TOKEN TESTS =====

func TestRefreshToken_HappyPath(t *testing.T) {
	app := testApp(t)
	app.JWTSecret = "test-secret-key"

	uid := seedUser(t, app.DB, "refreshuser")

	// Login to get a refresh token
	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "refreshuser@test.com",
			"password": "test123",
		}), app.Login)
	assertStatus(t, w, http.StatusOK)
	loginResp := respJSON(t, w)
	refreshToken := loginResp["refresh_token"].(string)

	// Use refresh token to get new tokens
	w = serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": refreshToken,
		}), app.RefreshToken)
	assertStatus(t, w, http.StatusOK)
	refreshResp := respJSON(t, w)

	if refreshResp["access_token"] == nil {
		t.Fatal("expected access_token in refresh response")
	}
	newRefresh := refreshResp["refresh_token"].(string)
	if newRefresh == refreshToken {
		t.Fatal("refresh token should have been rotated (new != old)")
	}
	_ = uid
}

func TestRefreshToken_OldTokenRevokedAfterRotation(t *testing.T) {
	app := testApp(t)
	app.JWTSecret = "test-secret-key"

	seedUser(t, app.DB, "rotateuser")

	// Login
	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "rotateuser@test.com",
			"password": "test123",
		}), app.Login)
	loginResp := respJSON(t, w)
	oldRefresh := loginResp["refresh_token"].(string)

	// Use refresh token (rotates it)
	serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": oldRefresh,
		}), app.RefreshToken)

	// Try to reuse old token — should fail
	w = serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": oldRefresh,
		}), app.RefreshToken)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestRefreshToken_TheftDetection_RevokeFamily(t *testing.T) {
	app := testApp(t)
	app.JWTSecret = "test-secret-key"

	seedUser(t, app.DB, "theftuser")

	// Login
	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "theftuser@test.com",
			"password": "test123",
		}), app.Login)
	loginResp := respJSON(t, w)
	firstRefresh := loginResp["refresh_token"].(string)

	// Legitimate rotation — get a new token
	w = serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": firstRefresh,
		}), app.RefreshToken)
	refreshResp := respJSON(t, w)
	secondRefresh := refreshResp["refresh_token"].(string)

	// Attacker reuses the first (revoked) token — family should be nuked
	w = serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": firstRefresh,
		}), app.RefreshToken)
	assertStatus(t, w, http.StatusUnauthorized)

	// Even the legitimate second token should now be revoked
	w = serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": secondRefresh,
		}), app.RefreshToken)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestRefreshToken_InvalidToken(t *testing.T) {
	app := testApp(t)
	app.JWTSecret = "test-secret-key"

	w := serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": "totally-invalid-token",
		}), app.RefreshToken)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestRefreshToken_MissingField(t *testing.T) {
	app := testApp(t)

	w := serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{}), app.RefreshToken)
	assertStatus(t, w, http.StatusBadRequest)
}

// ===== LOGOUT TESTS =====

func TestLogout_HappyPath(t *testing.T) {
	app := testApp(t)
	app.JWTSecret = "test-secret-key"

	seedUser(t, app.DB, "logoutuser")

	// Login
	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "logoutuser@test.com",
			"password": "test123",
		}), app.Login)
	loginResp := respJSON(t, w)
	refreshToken := loginResp["refresh_token"].(string)

	// Logout
	w = serve("POST", "/logout", "/logout",
		jsonBody(map[string]string{
			"refresh_token": refreshToken,
		}), app.Logout)
	assertStatus(t, w, http.StatusOK)

	// Refresh token should be revoked
	w = serve("POST", "/refresh", "/refresh",
		jsonBody(map[string]string{
			"refresh_token": refreshToken,
		}), app.RefreshToken)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestLogout_InvalidToken_StillReturnsOK(t *testing.T) {
	app := testApp(t)

	// Even with invalid token, logout should return 200 (don't reveal token existence)
	w := serve("POST", "/logout", "/logout",
		jsonBody(map[string]string{
			"refresh_token": "nonexistent-token",
		}), app.Logout)
	assertStatus(t, w, http.StatusOK)
}

// ===== LOGOUT ALL TESTS =====

func TestLogoutAll_RevokesAllSessions(t *testing.T) {
	app := testApp(t)
	app.JWTSecret = "test-secret-key"

	uid := seedUser(t, app.DB, "logoutalluser")

	// Login multiple times to create multiple sessions
	var refreshTokens []string
	for i := 0; i < 3; i++ {
		w := serve("POST", "/login", "/login",
			jsonBody(map[string]string{
				"email":    "logoutalluser@test.com",
				"password": "test123",
			}), app.Login)
		r := respJSON(t, w)
		refreshTokens = append(refreshTokens, r["refresh_token"].(string))
	}

	// Logout all
	w := serve("POST", "/logout-all", "/logout-all", nil, app.LogoutAll, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["sessions_revoked"].(float64) != 3 {
		t.Fatalf("expected 3 revoked sessions, got %v", r["sessions_revoked"])
	}

	// All refresh tokens should be revoked
	for i, rt := range refreshTokens {
		w = serve("POST", "/refresh", "/refresh",
			jsonBody(map[string]string{
				"refresh_token": rt,
			}), app.RefreshToken)
		assertStatus(t, w, http.StatusUnauthorized)
		_ = i
	}
}

// ===== EMAIL VERIFICATION TESTS =====

func TestVerifyEmail_HappyPath(t *testing.T) {
	app := testApp(t)
	mock := &email.MockMailer{}
	app.Mailer = mock

	// Signup (creates verification token)
	serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "verifytest@test.com",
			"password": "securepass123",
		}), app.Signup)

	// Extract raw token from the verification email body
	rawToken := extractTokenFromEmailBody(t, mock.Sent[0].Body)

	// Verify email
	w := serve("GET", "/verify", "/verify?token="+rawToken, nil, app.VerifyEmail)
	assertStatus(t, w, http.StatusOK)

	// Check that user is now verified
	var user models.User
	app.DB.Where("email = ?", "verifytest@test.com").First(&user)
	if !user.EmailVerified {
		t.Fatal("user should be verified after successful verification")
	}
}

func TestVerifyEmail_InvalidToken(t *testing.T) {
	app := testApp(t)

	w := serve("GET", "/verify", "/verify?token=invalid-token", nil, app.VerifyEmail)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestVerifyEmail_MissingToken(t *testing.T) {
	app := testApp(t)

	w := serve("GET", "/verify", "/verify", nil, app.VerifyEmail)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestVerifyEmail_DeletesAllTokensForUser(t *testing.T) {
	app := testApp(t)
	mock := &email.MockMailer{}
	app.Mailer = mock

	serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "multitok@test.com",
			"password": "securepass123",
		}), app.Signup)

	rawToken := extractTokenFromEmailBody(t, mock.Sent[0].Body)

	// Verify
	serve("GET", "/verify", "/verify?token="+rawToken, nil, app.VerifyEmail)

	// All verification tokens for this user should be deleted
	var count int64
	app.DB.Model(&models.EmailVerification{}).Where("user_id = ?", 1).Count(&count)
	if count != 0 {
		t.Fatalf("expected 0 remaining verification tokens, got %d", count)
	}
}

// ===== RESEND VERIFICATION TESTS =====

func TestResendVerification_HappyPath(t *testing.T) {
	app := testApp(t)
	mock := &email.MockMailer{}
	app.Mailer = mock

	uid := seedUser(t, app.DB, "resenduser")

	w := serve("POST", "/resend", "/resend", nil, app.ResendVerification, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	if len(mock.Sent) != 1 {
		t.Fatalf("expected 1 email sent, got %d", len(mock.Sent))
	}
}

func TestResendVerification_AlreadyVerified(t *testing.T) {
	app := testApp(t)
	mock := &email.MockMailer{}
	app.Mailer = mock

	uid := seedUser(t, app.DB, "alreadyverified")
	app.DB.Model(&models.User{}).Where("id = ?", uid).Update("email_verified", true)

	w := serve("POST", "/resend", "/resend", nil, app.ResendVerification, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
	r := respJSON(t, w)
	if r["error"] != "Email already verified" {
		t.Fatalf("unexpected error: %v", r["error"])
	}
}

// ===== PASSWORD HASHING EDGE CASES =====

func TestSignup_PasswordStoredAsHash(t *testing.T) {
	app := testApp(t)

	serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "hashcheck@test.com",
			"password": "securepass123",
			"username": "hashcheck",
		}), app.Signup)

	var user models.User
	app.DB.Where("email = ?", "hashcheck@test.com").First(&user)
	if user.Hash == "" {
		t.Fatal("password hash should be stored")
	}
	if user.Hash == "securepass123" {
		t.Fatal("password must not be stored in plaintext")
	}
}

func TestSignup_MultipleUsers_UniqueIDs(t *testing.T) {
	app := testApp(t)

	users := make(map[float64]bool)
	for i := 0; i < 5; i++ {
		w := serve("POST", "/signup", "/signup",
			jsonBody(map[string]string{
				"email":    fmt.Sprintf("multi%d@test.com", i),
				"password": "securepass123",
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

// ===== TEST HELPERS =====

// extractTokenFromEmailBody extracts the verification token from the email body URL.
// The URL format is: .../auth/verify-email?token=<token>
func extractTokenFromEmailBody(t *testing.T, body string) string {
	t.Helper()
	const prefix = "token="
	for i := 0; i < len(body); i++ {
		if i+len(prefix) < len(body) && body[i:i+len(prefix)] == prefix {
			start := i + len(prefix)
			end := start
			for end < len(body) && isHexChar(body[end]) {
				end++
			}
			if end > start {
				return body[start:end]
			}
		}
	}
	t.Fatal("could not extract token from email body")
	return ""
}

func isHexChar(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
