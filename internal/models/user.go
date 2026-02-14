// Package models defines the domain entities and their database mappings.
package models

import (
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// BcryptCost controls the bcrypt work factor. Tests can set this to bcrypt.MinCost
// for speed. Production uses bcrypt.DefaultCost.
var BcryptCost = bcrypt.DefaultCost

// User represents an authenticated user with optional admin privileges.
type User struct {
	ID            uint        `json:"id" gorm:"primaryKey"`
	Email         string      `json:"email" gorm:"unique; not null"` // NOTE: Exclude from public-facing responses
	Username      string      `json:"username" gorm:"default:''"`
	Password      string      `json:"-" gorm:"-"`                    // input-only field, never persisted or serialized
	Hash          string      `json:"-" gorm:"not null"`             // bcrypt hash, never exposed
	AdminOf       []Subnotery `json:"admin_of" gorm:"many2many:user_admins;"`
	IsGlobalAdmin bool        `json:"is_global_admin" gorm:"default:false"`

	// ----- Account Security Fields -----
	EmailVerified bool `json:"-" gorm:"default:false"` // true after email verification flow
	// ----- Karma Fields -----
	// NoteKarma is the total net upvotes received on the user's notes.
	NoteKarma int64 `json:"note_karma" gorm:"default:0"`
	// CommentKarma is the total net upvotes received on the user's comments.
	CommentKarma int64 `json:"comment_karma" gorm:"default:0"`
	// ----- Profile Fields -----
	DisplayNameField string          `json:"display_name" gorm:"column:display_name;default:''"`
	Bio              string          `json:"bio" gorm:"type:text;default:''"`
	AvatarURL        string          `json:"avatar_url" gorm:"default:''"`
	ProfileVisibility ProfileVisibility `json:"profile_visibility" gorm:"default:'public'"`
	ProfileUpdatedAt *time.Time      `json:"profile_updated_at"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// ProfileVisibility controls what data is shown on a user's public profile.
type ProfileVisibility string

const (
	// ProfilePublic makes the full profile visible to all users.
	ProfilePublic ProfileVisibility = "public"
	// ProfilePrivate hides bio and avatar from other users.
	ProfilePrivate ProfileVisibility = "private"
)

// ValidProfileVisibility returns true if the visibility value is recognized.
func ValidProfileVisibility(v ProfileVisibility) bool {
	return v == ProfilePublic || v == ProfilePrivate
}

// Profile field constraints.
const (
	MaxDisplayNameLength = 50
	MinDisplayNameLength = 2
	MaxBioLength         = 500
	MaxAvatarURLLength   = 2048
	MaxUsernameLength    = 30
	MinUsernameLength    = 3
)

// DisplayName returns the user's public display name.
// Prefers DisplayNameField, then Username, falls back to "User <ID>" to avoid leaking email.
func (u *User) DisplayName() string {
	if u.DisplayNameField != "" {
		return u.DisplayNameField
	}
	if u.Username != "" {
		return u.Username
	}
	return fmt.Sprintf("User %d", u.ID)
}

// PublicProfile returns a safe-to-expose profile DTO.
// Sensitive fields (email, hash, admin status) are excluded.
// TotalKarma returns the sum of note and comment karma.
func (u *User) TotalKarma() int64 {
	return u.NoteKarma + u.CommentKarma
}

// PublicProfile returns a safe-to-expose profile DTO.
// Sensitive fields (email, hash, admin status) are excluded.
func (u *User) PublicProfile() map[string]interface{} {
	profile := map[string]interface{}{
		"id":            u.ID,
		"username":      u.Username,
		"display_name":  u.DisplayNameField,
		"note_karma":    u.NoteKarma,
		"comment_karma": u.CommentKarma,
		"total_karma":   u.TotalKarma(),
		"created_at":    u.CreatedAt,
	}
	if u.ProfileVisibility == ProfilePublic {
		profile["bio"] = u.Bio
		profile["avatar_url"] = u.AvatarURL
	}
	return profile
}

// SelfProfile returns the full profile for the authenticated user.
// Includes private fields like email and visibility settings.
func (u *User) SelfProfile() map[string]interface{} {
	return map[string]interface{}{
		"id":                 u.ID,
		"email":              u.Email,
		"email_verified":     u.EmailVerified,
		"username":           u.Username,
		"display_name":       u.DisplayNameField,
		"bio":                u.Bio,
		"avatar_url":         u.AvatarURL,
		"profile_visibility": u.ProfileVisibility,
		"note_karma":         u.NoteKarma,
		"comment_karma":      u.CommentKarma,
		"total_karma":        u.TotalKarma(),
		"profile_updated_at": u.ProfileUpdatedAt,
		"created_at":         u.CreatedAt,
		"updated_at":         u.UpdatedAt,
	}
}

// SetPassword hashes the given password and stores it in the Hash field.
func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return err
	}
	u.Hash = string(hash)
	return nil
}

// CheckPassword returns true if the given password matches the stored hash.
func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Hash), []byte(password))
	return err == nil
}
