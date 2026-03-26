// oauth_test.go — Tests for OAuth handler logic and endpoints.
package handlers

import (
	"errors"
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

func TestOAuthFindOrCreate_EmailConflictRejectsLink(t *testing.T) {
	app := testApp(t)

	// Create a regular (non-OAuth) user
	user := models.User{Email: "existing@test.com", Username: "existing"}
	if err := user.SetPassword("password123"); err != nil {
		t.Fatal(err)
	}
	app.DB.Create(&user)

	// OAuth login with same email should be rejected (no auto-linking)
	_, err := app.oauthFindOrCreateUser("google", "goog-789", "existing@test.com", "Existing User")
	if err == nil {
		t.Fatal("expected error for email conflict, got nil")
	}
	if !errors.Is(err, errOAuthEmailConflict) {
		t.Fatalf("expected errOAuthEmailConflict, got: %v", err)
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

// ===== OAUTH DUPLICATE USERNAME =====

func TestOAuthFindOrCreate_DuplicateUsername(t *testing.T) {
	app := testApp(t)

	// Create a user with username "TestUser"
	seedUser(t, app.DB, "TestUser")

	// OAuth user with same display name should get a suffixed username
	user, err := app.oauthFindOrCreateUser("google", "goog-dup1", "dup1@test.com", "TestUser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username == "TestUser" {
		t.Fatal("expected different username from TestUser, got same")
	}
	if user.Username != "TestUser1" {
		t.Fatalf("expected TestUser1, got %s", user.Username)
	}
	// display_name must also be deduped (unique index)
	if user.DisplayNameField != "TestUser1" {
		t.Fatalf("expected DisplayNameField=TestUser1, got %s", user.DisplayNameField)
	}
}

func TestOAuthFindOrCreate_MultipleDuplicates(t *testing.T) {
	app := testApp(t)

	// Create TestDup and TestDup1
	seedUser(t, app.DB, "TestDup")
	app.DB.Create(&models.User{Email: "dup1seed@test.com", Username: "TestDup1", DisplayNameField: "TestDup1"})

	// Should get TestDup2
	user, err := app.oauthFindOrCreateUser("github", "gh-dup2", "dup2@test.com", "TestDup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username != "TestDup2" {
		t.Fatalf("expected TestDup2, got %s", user.Username)
	}
	if user.DisplayNameField != "TestDup2" {
		t.Fatalf("expected DisplayNameField=TestDup2, got %s", user.DisplayNameField)
	}
}

func TestOAuthFindOrCreate_DisplayNameConflict(t *testing.T) {
	app := testApp(t)

	// Create a user where display_name is "OAuthName" but username differs
	app.DB.Create(&models.User{
		Email: "existing-dn@test.com", Username: "differentUser", DisplayNameField: "OAuthName",
	})

	// OAuth user whose sanitized display name collides with existing display_name
	user, err := app.oauthFindOrCreateUser("google", "goog-dn1", "newdn@test.com", "OAuthName")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Must not collide with the existing display_name
	if user.DisplayNameField == "OAuthName" {
		t.Fatal("expected display_name to be deduped, got OAuthName")
	}
	if user.Username != "OAuthName1" {
		t.Fatalf("expected username OAuthName1, got %s", user.Username)
	}
	if user.DisplayNameField != "OAuthName1" {
		t.Fatalf("expected DisplayNameField OAuthName1, got %s", user.DisplayNameField)
	}
}

func TestOAuthFindOrCreate_ThreeGoogleSameName(t *testing.T) {
	app := testApp(t)

	// Three different Google accounts all with display name "JohnDoe"
	u1, err := app.oauthFindOrCreateUser("google", "goog-j1", "john1@test.com", "JohnDoe")
	if err != nil {
		t.Fatalf("user1 error: %v", err)
	}
	if u1.Username != "JohnDoe" {
		t.Fatalf("expected JohnDoe, got %s", u1.Username)
	}

	u2, err := app.oauthFindOrCreateUser("google", "goog-j2", "john2@test.com", "JohnDoe")
	if err != nil {
		t.Fatalf("user2 error: %v", err)
	}
	if u2.Username != "JohnDoe1" {
		t.Fatalf("expected JohnDoe1, got %s", u2.Username)
	}

	u3, err := app.oauthFindOrCreateUser("google", "goog-j3", "john3@test.com", "JohnDoe")
	if err != nil {
		t.Fatalf("user3 error: %v", err)
	}
	if u3.Username != "JohnDoe2" {
		t.Fatalf("expected JohnDoe2, got %s", u3.Username)
	}

	// All three must have different IDs
	if u1.ID == u2.ID || u2.ID == u3.ID || u1.ID == u3.ID {
		t.Fatalf("expected 3 different users, got IDs %d, %d, %d", u1.ID, u2.ID, u3.ID)
	}
}

// ===== SIGNUP DUPLICATE USERNAME =====

func TestSignup_DuplicateUsername(t *testing.T) {
	app := testApp(t)
	app.JWTSecret = "test-secret-key"
	app.BaseURL = "http://localhost:8080"
	app.Mailer = &email.LogMailer{}

	// First signup
	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"email":           "first@test.com",
			"password":        "Securepass1",
			"username":        "dupname",
			"agreed_to_terms": true,
		}), app.Signup)
	assertStatus(t, w, http.StatusCreated)

	// Second signup with same username should fail
	w2 := serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"email":           "second@test.com",
			"password":        "Securepass1",
			"username":        "dupname",
			"agreed_to_terms": true,
		}), app.Signup)
	assertStatus(t, w2, http.StatusConflict)
}

// ===== DEV MODE AUTO-VERIFY =====

func TestSignup_DevModeAutoVerify(t *testing.T) {
	app := testApp(t)
	app.JWTSecret = "test-secret-key"
	app.BaseURL = "http://localhost:8080"
	// Use the real email.LogMailer to trigger the dev-mode auto-verify path
	app.Mailer = &email.LogMailer{}

	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]interface{}{
			"email":           "devuser@test.com",
			"password":        "Securepass1",
			"username":        "devuser",
			"agreed_to_terms": true,
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
		jsonBody(map[string]interface{}{
			"email":           "produser@test.com",
			"password":        "Securepass1",
			"username":        "produser",
			"agreed_to_terms": true,
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
