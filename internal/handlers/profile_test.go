package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Geetur/Notery/internal/helpers"
	"github.com/Geetur/Notery/internal/models"
)

// ===== GET /me/profile =====

func TestGetMyProfile_Success(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "profuser")

	// Set some profile fields
	app.DB.Model(&models.User{}).Where("id = ?", uid).Updates(map[string]interface{}{
		"display_name":       "Profile User",
		"bio":                "I write notes",
		"avatar_url":         "https://example.com/avatar.png",
		"profile_visibility": "public",
	})

	w := serve("GET", "/me/profile", "/me/profile", nil, app.GetMyProfile, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["email"] == nil {
		t.Fatal("self profile should include email")
	}
	if r["display_name"] != "Profile User" {
		t.Fatalf("display_name=%v, want 'Profile User'", r["display_name"])
	}
	if r["bio"] != "I write notes" {
		t.Fatalf("bio=%v, want 'I write notes'", r["bio"])
	}
	if r["profile_visibility"] != "public" {
		t.Fatalf("visibility=%v, want 'public'", r["profile_visibility"])
	}
}

func TestGetMyProfile_DefaultValues(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "defaultprof")

	w := serve("GET", "/me/profile", "/me/profile", nil, app.GetMyProfile, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	// Defaults should be empty strings / "public"
	if r["display_name"] != "" {
		t.Fatalf("default display_name=%v, want ''", r["display_name"])
	}
	if r["bio"] != "" {
		t.Fatalf("default bio=%v, want ''", r["bio"])
	}
}

// ===== PATCH /me/profile =====

func TestUpdateProfile_Bio(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "bioupdater")

	w := serve("PATCH", "/me/profile", "/me/profile",
		jsonBody(map[string]string{"bio": "I love studying."}),
		app.UpdateMyProfile, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["bio"] != "I love studying." {
		t.Fatalf("bio=%v, want 'I love studying.'", r["bio"])
	}
}

func TestUpdateProfile_AvatarURL(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "avatarupdater")

	w := serve("PATCH", "/me/profile", "/me/profile",
		jsonBody(map[string]string{"avatar_url": "https://cdn.example.com/me.jpg"}),
		app.UpdateMyProfile, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["avatar_url"] != "https://cdn.example.com/me.jpg" {
		t.Fatalf("avatar_url=%v", r["avatar_url"])
	}
}

func TestUpdateProfile_Visibility(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "visupdater")

	w := serve("PATCH", "/me/profile", "/me/profile",
		jsonBody(map[string]string{"profile_visibility": "private"}),
		app.UpdateMyProfile, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["profile_visibility"] != "private" {
		t.Fatalf("visibility=%v, want 'private'", r["profile_visibility"])
	}
}

func TestUpdateProfile_Username(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "usrnameupd")

	w := serve("PATCH", "/me/profile", "/me/profile",
		jsonBody(map[string]string{"username": "coolhandle"}),
		app.UpdateMyProfile, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["username"] != "coolhandle" {
		t.Fatalf("username=%v, want 'coolhandle'", r["username"])
	}
}

func TestUpdateProfile_MultipleFieldsAtOnce(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "multifld")

	w := serve("PATCH", "/me/profile", "/me/profile",
		jsonBody(map[string]string{
			"bio":                "Bio text here",
			"profile_visibility": "private",
		}),
		app.UpdateMyProfile, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["bio"] != "Bio text here" || r["profile_visibility"] != "private" {
		t.Fatalf("unexpected response: %v", r)
	}
}

func TestUpdateProfile_ClearFieldsToEmpty(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "clearer")

	// Set initial values
	app.DB.Model(&models.User{}).Where("id = ?", uid).Updates(map[string]interface{}{
		"bio": "Old bio",
	})

	// Clear them
	w := serve("PATCH", "/me/profile", "/me/profile",
		jsonBody(map[string]string{"bio": ""}),
		app.UpdateMyProfile, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["bio"] != "" {
		t.Fatalf("bio=%v, want ''", r["bio"])
	}
}

// ===== VALIDATION TESTS =====

func TestUpdateProfile_BioTooLong(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "longbio")

	long := strings.Repeat("x", models.MaxBioLength+1)
	w := serve("PATCH", "/me/profile", "/me/profile",
		jsonBody(map[string]string{"bio": long}),
		app.UpdateMyProfile, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestUpdateProfile_AvatarMustBeHTTPS(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "httpavatar")

	w := serve("PATCH", "/me/profile", "/me/profile",
		jsonBody(map[string]string{"avatar_url": "http://example.com/evil.png"}),
		app.UpdateMyProfile, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestUpdateProfile_AvatarTooLong(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "longavatar")

	long := "https://example.com/" + strings.Repeat("a", models.MaxAvatarURLLength)
	w := serve("PATCH", "/me/profile", "/me/profile",
		jsonBody(map[string]string{"avatar_url": long}),
		app.UpdateMyProfile, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestUpdateProfile_InvalidVisibility(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "badvis")

	w := serve("PATCH", "/me/profile", "/me/profile",
		jsonBody(map[string]string{"profile_visibility": "secret"}),
		app.UpdateMyProfile, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestUpdateProfile_UsernameTooShort(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "shortun")

	w := serve("PATCH", "/me/profile", "/me/profile",
		jsonBody(map[string]string{"username": "ab"}),
		app.UpdateMyProfile, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestUpdateProfile_UsernameTooLong(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "longun")

	long := strings.Repeat("x", models.MaxUsernameLength+1)
	w := serve("PATCH", "/me/profile", "/me/profile",
		jsonBody(map[string]string{"username": long}),
		app.UpdateMyProfile, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestUpdateProfile_UsernameInvalidChars(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "badun")

	for _, bad := range []string{"hello world", "a@b", "user!name", ".dotstart"} {
		w := serve("PATCH", "/me/profile", "/me/profile",
			jsonBody(map[string]string{"username": bad}),
			app.UpdateMyProfile, authMW(uid))
		if w.Code != http.StatusBadRequest {
			t.Errorf("username %q should be rejected, got %d", bad, w.Code)
		}
	}
}

func TestUpdateProfile_WhitespaceNormalization(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "normws")

	w := serve("PATCH", "/me/profile", "/me/profile",
		jsonBody(map[string]string{"bio": "  Hello   World  "}),
		app.UpdateMyProfile, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["bio"] != "Hello   World" {
		t.Fatalf("expected trimmed bio, got %v", r["bio"])
	}
}

func TestUpdateProfile_EmptyBody(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "emptybody")

	w := serve("PATCH", "/me/profile", "/me/profile",
		jsonBody(map[string]string{}),
		app.UpdateMyProfile, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

// ===== GET /users/:id/profile =====

func TestGetUserProfile_PublicVisibility(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "pubuser")

	app.DB.Model(&models.User{}).Where("id = ?", uid).Updates(map[string]interface{}{
		"display_name":       "Public Person",
		"bio":                "Visible bio",
		"avatar_url":         "https://example.com/pub.png",
		"profile_visibility": "public",
	})

	w := serve("GET", "/users/:id/profile",
		"/users/1/profile", nil, app.GetUserProfile)
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	if r["bio"] != "Visible bio" {
		t.Fatalf("expected visible bio, got %v", r["bio"])
	}
	if r["avatar_url"] != "https://example.com/pub.png" {
		t.Fatalf("expected avatar, got %v", r["avatar_url"])
	}
	// Must NOT include email or hash
	if r["email"] != nil {
		t.Fatal("public profile should not include email")
	}
	if r["hash"] != nil {
		t.Fatal("public profile should not include hash")
	}
}

func TestGetUserProfile_PrivateVisibility(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "privuser")

	app.DB.Model(&models.User{}).Where("id = ?", uid).Updates(map[string]interface{}{
		"display_name":       "Private Person",
		"bio":                "Secret bio",
		"avatar_url":         "https://example.com/priv.png",
		"profile_visibility": "private",
	})

	w := serve("GET", "/users/:id/profile",
		"/users/1/profile", nil, app.GetUserProfile)
	assertStatus(t, w, http.StatusOK)

	r := respJSON(t, w)
	// Username/display_name always visible
	if r["display_name"] != "Private Person" {
		t.Fatalf("display_name should be visible even on private, got %v", r["display_name"])
	}
	// Bio and avatar should NOT be present for private profiles
	if r["bio"] != nil {
		t.Fatal("private profile should not expose bio")
	}
	if r["avatar_url"] != nil {
		t.Fatal("private profile should not expose avatar_url")
	}
}

func TestGetUserProfile_NotFound(t *testing.T) {
	app := testApp(t)

	w := serve("GET", "/users/:id/profile",
		"/users/99999/profile", nil, app.GetUserProfile)
	assertStatus(t, w, http.StatusNotFound)
}

func TestGetUserProfile_InvalidID(t *testing.T) {
	app := testApp(t)

	w := serve("GET", "/users/:id/profile",
		"/users/abc/profile", nil, app.GetUserProfile)
	assertStatus(t, w, http.StatusBadRequest)
}

// ===== MODEL TESTS =====

func TestUser_DisplayNamePrecedence(t *testing.T) {
	// DisplayNameField > Username > "User <ID>"
	u := models.User{Username: "handle", DisplayNameField: "Fancy Name"}
	u.ID = 42
	if got := u.DisplayName(); got != "Fancy Name" {
		t.Errorf("DisplayName=%q, want 'Fancy Name'", got)
	}

	u2 := models.User{Username: "handle"}
	u2.ID = 42
	if got := u2.DisplayName(); got != "handle" {
		t.Errorf("DisplayName=%q, want 'handle'", got)
	}

	u3 := models.User{}
	u3.ID = 42
	if got := u3.DisplayName(); got != "User 42" {
		t.Errorf("DisplayName=%q, want 'User 42'", got)
	}
}

func TestUser_PublicProfileHidesSensitiveFields(t *testing.T) {
	u := models.User{
		Email:             "secret@example.com",
		Hash:              "$2a$10$somehash",
		Username:          "public_user",
		DisplayNameField:  "Pubs",
		Bio:               "My bio",
		AvatarURL:         "https://cdn.example.com/pic.png",
		ProfileVisibility: models.ProfilePublic,
	}
	u.ID = 1

	pub := u.PublicProfile()
	if pub["email"] != nil {
		t.Error("public profile leaks email")
	}
	if pub["hash"] != nil {
		t.Error("public profile leaks hash")
	}
	if pub["bio"] != "My bio" {
		t.Errorf("public profile missing bio: %v", pub["bio"])
	}
}

func TestUser_SelfProfileIncludesAllFields(t *testing.T) {
	u := models.User{
		Email:             "me@example.com",
		Username:          "me",
		DisplayNameField:  "Myself",
		Bio:               "My bio",
		AvatarURL:         "https://cdn.example.com/me.png",
		ProfileVisibility: models.ProfilePublic,
	}
	u.ID = 1

	self := u.SelfProfile()
	if self["email"] != "me@example.com" {
		t.Error("self profile should include email")
	}
	if self["bio"] != "My bio" {
		t.Error("self profile should include bio")
	}
	if self["profile_visibility"] != models.ProfilePublic {
		t.Errorf("self profile visibility=%v", self["profile_visibility"])
	}
}

func TestValidateUsername(t *testing.T) {
	valid := []string{"abc", "user123", "my-handle", "under_score", "A1b"}
	for _, u := range valid {
		if err := helpers.ValidateUsername(u); err != nil {
			t.Errorf("username %q should be valid: %v", u, err)
		}
	}

	invalid := []string{"ab", "A1", "", strings.Repeat("x", 31), "_start", "-start", "has space", "a@b"}
	for _, u := range invalid {
		if err := helpers.ValidateUsername(u); err == nil {
			t.Errorf("username %q should be invalid", u)
		}
	}
}

func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"  hello  ", "hello"},
		{"  John   Doe  ", "John Doe"},
		{"\t\nnewlines\t\n", "newlines"},
		{"no change", "no change"},
		{"   ", ""},
	}
	for _, tt := range tests {
		got := helpers.NormalizeWhitespace(tt.in)
		if got != tt.want {
			t.Errorf("NormalizeWhitespace(%q)=%q, want %q", tt.in, got, tt.want)
		}
	}
}
