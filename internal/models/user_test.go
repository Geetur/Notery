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