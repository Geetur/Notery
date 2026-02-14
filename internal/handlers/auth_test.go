package handlers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/Geetur/Notery/internal/models"
)

// ===== SIGNUP TESTS =====

func TestSignup_HappyPath(t *testing.T) {
	app := testApp(t)

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

	// First signup
	serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "dupe@test.com",
			"password": "securepass123",
		}), app.Signup)

	// Second signup with same email
	w := serve("POST", "/signup", "/signup",
		jsonBody(map[string]string{
			"email":    "dupe@test.com",
			"password": "differentpass",
		}), app.Signup)
	assertStatus(t, w, http.StatusInternalServerError) // GORM returns unique constraint error
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
			"password": "1234567", // 7 chars, minimum is 8
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
			"password": "12345678", // exactly 8 chars
		}), app.Signup)
	assertStatus(t, w, http.StatusCreated)
}

// ===== LOGIN TESTS =====

func TestLogin_HappyPath(t *testing.T) {
	app := testApp(t)
	app.JWTSecret = "test-secret-key"

	// Create user first
	seedUser(t, app.DB, "loginuser")

	w := serve("POST", "/login", "/login",
		jsonBody(map[string]string{
			"email":    "loginuser@test.com",
			"password": "test123",
		}), app.Login)
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["token"] == nil {
		t.Fatal("expected token in response")
	}
	token, ok := r["token"].(string)
	if !ok || token == "" {
		t.Fatal("token should be a non-empty string")
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
	app.JWTSecret = "" // empty secret

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
