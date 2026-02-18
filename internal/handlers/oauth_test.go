// oauth_test.go — Tests for OAuth handler logic and endpoints.
package handlers

import (
	"net/http"
	"testing"

	"github.com/Geetur/Notery/internal/email"
	"github.com/Geetur/Notery/internal/models"
)

// ===== PROVIDER AVAILABILITY =====

func TestOAuthProviders_NoneConfigured(t *testing.T) {
	app := testApp(t)
	w := serve("GET", "/providers", "/providers", nil, app.OAuthProviders)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["google"] != false {
		t.Fatal("expected google=false when not configured")
	}
	if r["github"] != false {
		t.Fatal("expected github=false when not configured")
	}
}

func TestOAuthProviders_GoogleConfigured(t *testing.T) {
	app := testApp(t)
	app.GoogleClientID = "test-google-id"
	w := serve("GET", "/providers", "/providers", nil, app.OAuthProviders)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["google"] != true {
		t.Fatal("expected google=true when configured")
	}
	if r["github"] != false {
		t.Fatal("expected github=false when not configured")
	}
}

func TestOAuthProviders_BothConfigured(t *testing.T) {
	app := testApp(t)
	app.GoogleClientID = "test-google-id"
	app.GitHubClientID = "test-github-id"
	w := serve("GET", "/providers", "/providers", nil, app.OAuthProviders)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["google"] != true {
		t.Fatal("expected google=true")
	}
	if r["github"] != true {
		t.Fatal("expected github=true")
	}
}

// ===== OAUTH REDIRECT (NOT CONFIGURED) =====

func TestOAuthGoogle_NotConfigured(t *testing.T) {
	app := testApp(t)
	w := serve("GET", "/google", "/google", nil, app.OAuthGoogle)
	assertStatus(t, w, http.StatusNotImplemented)
}

func TestOAuthGitHub_NotConfigured(t *testing.T) {
	app := testApp(t)
	w := serve("GET", "/github", "/github", nil, app.OAuthGitHub)
	assertStatus(t, w, http.StatusNotImplemented)
}

// ===== FIND OR CREATE USER =====

func TestOAuthFindOrCreate_NewUser(t *testing.T) {
	app := testApp(t)
	user, err := app.oauthFindOrCreateUser("google", "goog-123", "oauth@test.com", "OAuth User")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Email != "oauth@test.com" {
		t.Fatalf("expected email oauth@test.com, got %s", user.Email)
	}
	if user.OAuthProvider != "google" {
		t.Fatalf("expected provider google, got %s", user.OAuthProvider)
	}
	if user.OAuthID != "goog-123" {
		t.Fatalf("expected oauth_id goog-123, got %s", user.OAuthID)
	}
	if !user.EmailVerified {
		t.Fatal("expected email_verified=true for OAuth user")
	}
}

func TestOAuthFindOrCreate_ExistingOAuthUser(t *testing.T) {
	app := testApp(t)

	// Create initial OAuth user
	first, _ := app.oauthFindOrCreateUser("github", "gh-456", "githubuser@test.com", "GH User")

	// Same OAuth credentials should return same user
	second, err := app.oauthFindOrCreateUser("github", "gh-456", "githubuser@test.com", "GH User")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected same user ID %d, got %d", first.ID, second.ID)
	}
}

func TestOAuthFindOrCreate_LinkExistingEmailUser(t *testing.T) {
	app := testApp(t)

	// Create a regular (non-OAuth) user
	user := models.User{Email: "existing@test.com", Username: "existing"}
	if err := user.SetPassword("password123"); err != nil {
		t.Fatal(err)
	}
	app.DB.Create(&user)

	// OAuth login with same email should link to existing account
	linked, err := app.oauthFindOrCreateUser("google", "goog-789", "existing@test.com", "Existing User")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if linked.ID != user.ID {
		t.Fatalf("expected linked to existing user ID %d, got %d", user.ID, linked.ID)
	}

	// Verify the OAuth fields were updated
	var updated models.User
	app.DB.First(&updated, user.ID)
	if updated.OAuthProvider != "google" {
		t.Fatalf("expected oauth_provider=google on linked user, got %s", updated.OAuthProvider)
	}
	if !updated.EmailVerified {
		t.Fatal("expected email_verified=true after OAuth link")
	}
}

// ===== SANITIZE USERNAME =====

func TestSanitizeUsername_Normal(t *testing.T) {
	result := sanitizeUsername("JohnDoe")
	if result != "JohnDoe" {
		t.Fatalf("expected JohnDoe, got %s", result)
	}
}

func TestSanitizeUsername_WithSpaces(t *testing.T) {
	result := sanitizeUsername("John Doe")
	if result != "JohnDoe" {
		t.Fatalf("expected JohnDoe, got %s", result)
	}
}

func TestSanitizeUsername_TooShort(t *testing.T) {
	result := sanitizeUsername("AB")
	if len(result) < models.MinUsernameLength {
		t.Fatalf("expected username at least %d chars, got %d: %s", models.MinUsernameLength, len(result), result)
	}
}

func TestSanitizeUsername_TooLong(t *testing.T) {
	long := "abcdefghijklmnopqrstuvwxyz1234567890abcdefghijklmnopqrstuvwxyz"
	result := sanitizeUsername(long)
	if len(result) > models.MaxUsernameLength {
		t.Fatalf("expected username at most %d chars, got %d: %s", models.MaxUsernameLength, len(result), result)
	}
}

// ===== DEV MODE AUTO-VERIFY =====

func TestSignup_DevModeAutoVerify(t *testing.T) {
	app := testApp(t)
	app.JWTSecret = "test-secret-key"
	app.BaseURL = "http://localhost:8080"
	// Use the real email.LogMailer to trigger the dev-mode auto-verify path
	app.Mailer = &email.LogMailer{}

	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "devuser@test.com",
			"password": "securepass123",
			"username": "devuser",
		}), app.Signup)
	assertStatus(t, w, http.StatusCreated)

	// User should be auto-verified in dev mode
	var user models.User
	app.DB.Where("email = ?", "devuser@test.com").First(&user)
	if !user.EmailVerified {
		t.Fatal("expected user to be auto-verified in dev mode (LogMailer)")
	}
}

func TestSignup_ProductionSendsVerificationEmail(t *testing.T) {
	app, mock := testAppWithMailer(t)

	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "produser@test.com",
			"password": "securepass123",
			"username": "produser",
		}), app.Signup)
	assertStatus(t, w, http.StatusCreated)

	// MockMailer (simulating SMTP) should send verification email
	if len(mock.Sent) != 1 {
		t.Fatalf("expected 1 verification email, got %d", len(mock.Sent))
	}

	// User should NOT be auto-verified (must click email link)
	var user models.User
	app.DB.Where("email = ?", "produser@test.com").First(&user)
	if user.EmailVerified {
		t.Fatal("expected user NOT to be auto-verified with MockMailer (production mode)")
	}
}
