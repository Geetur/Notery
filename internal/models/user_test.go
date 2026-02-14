package models

import (
	"testing"
)

func TestUserPasswordSecurity(t *testing.T) {
	user := &User{Email: "test@example.com"}
	password := "securepassword123"

	if err := user.SetPassword(password); err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	if user.Password != "" {
		t.Errorf("Password field should not be exposed, got: %s", user.Password)
	}
	if user.Hash == "" {
		t.Errorf("Hash field should be set after hashing the password")
	}
	if !user.CheckPassword(password) {
		t.Errorf("CheckPassword failed for correct password")
	}
	if user.CheckPassword("wrongpassword") {
		t.Errorf("CheckPassword should fail for incorrect password")
	}
}

func TestDisplayNamePrecedence(t *testing.T) {
	tests := []struct {
		name        string
		user        User
		wantDisplay string
	}{
		{"DisplayNameField wins", User{DisplayNameField: "Fancy", Username: "handle"}, "Fancy"},
		{"Username fallback", User{Username: "handle"}, "handle"},
		{"ID fallback", User{}, "User 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.user.DisplayName()
			if got != tt.wantDisplay {
				t.Errorf("DisplayName()=%q, want %q", got, tt.wantDisplay)
			}
		})
	}
}

func TestValidProfileVisibility(t *testing.T) {
	if !ValidProfileVisibility(ProfilePublic) {
		t.Error("ProfilePublic should be valid")
	}
	if !ValidProfileVisibility(ProfilePrivate) {
		t.Error("ProfilePrivate should be valid")
	}
	if ValidProfileVisibility("secret") {
		t.Error("'secret' should not be a valid visibility")
	}
}

func TestPublicProfile_HidesSensitiveFields(t *testing.T) {
	u := User{
		Email:             "secret@example.com",
		Hash:              "$2a$10$somehash",
		Username:          "pub_user",
		DisplayNameField:  "Public",
		Bio:               "My bio",
		AvatarURL:         "https://cdn.example.com/pic.png",
		ProfileVisibility: ProfilePublic,
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
		t.Errorf("expected bio in public profile, got %v", pub["bio"])
	}
}

func TestPublicProfile_PrivateHidesBioAndAvatar(t *testing.T) {
	u := User{
		Username:          "priv_user",
		Bio:               "Hidden bio",
		AvatarURL:         "https://cdn.example.com/hidden.png",
		ProfileVisibility: ProfilePrivate,
	}
	u.ID = 2

	pub := u.PublicProfile()
	if pub["bio"] != nil {
		t.Error("private profile should not expose bio")
	}
	if pub["avatar_url"] != nil {
		t.Error("private profile should not expose avatar_url")
	}
	// Username/display_name should still be visible
	if pub["username"] != "priv_user" {
		t.Errorf("username should be visible, got %v", pub["username"])
	}
}

func TestSelfProfile_IncludesAllFields(t *testing.T) {
	u := User{
		Email:             "me@test.com",
		Username:          "myself",
		DisplayNameField:  "Me",
		Bio:               "My bio",
		AvatarURL:         "https://cdn.example.com/me.png",
		ProfileVisibility: ProfilePublic,
	}
	u.ID = 1

	self := u.SelfProfile()
	if self["email"] != "me@test.com" {
		t.Error("self profile should include email")
	}
	if self["bio"] != "My bio" {
		t.Error("self profile should include bio")
	}
	if self["profile_visibility"] != ProfilePublic {
		t.Errorf("self profile visibility=%v", self["profile_visibility"])
	}
}
