// subnotery.go — Subnotery model: community containers for notes.
//
// DESIGN:
//
//	Subnoteries are Reddit-style communities where notes are posted. Each subnotery
//	has a unique name, a list of admins (many-to-many via user_admins join table),
//	and a list of members (many-to-many via user_memberships join table).
//
//	Subnoteries are auto-created during note creation if they don't exist; the
//	creator becomes the first admin and member. Users can join existing subnoteries
//	and global/subnotery admins can add other users as admins.
package models

import (
	"time"
)

// Subnotery represents a note community with designated admins and members.
//
// Fields:
//   - Name:    Unique community name (indexed)
//   - Admins:  Users with admin privileges (many-to-many via user_admins)
//   - Members: Users who have joined the community (many-to-many via user_memberships)
type Subnotery struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"unique; not null"`
	Admins    []User    `json:"admins" gorm:"many2many:user_admins;"`
	Members   []User    `json:"members" gorm:"many2many:user_memberships;"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
