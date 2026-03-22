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
//   - Name:        Unique community name (indexed)
//   - Description: Community description set by admins
//   - ContentType: Type of content hosted (e.g., "PDF Notes", "Lecture Summaries")
//   - Rules:       Community-specific marketplace rules (Markdown or plain text)
//   - Admins:      Users with admin privileges (many-to-many via user_admins)
//   - Members:     Users who have joined the community (many-to-many via user_memberships)
type Subnotery struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"unique; not null"`
	Description string    `json:"description" gorm:"type:text"`
	ContentType string    `json:"content_type" gorm:"type:varchar(100)"`
	Rules       string    `json:"rules" gorm:"type:text"`

	// BannerURL is the R2 object key for the community banner image.
	// Admins can upload/change banners via the settings panel.
	BannerURL string `json:"banner_url" gorm:"type:text;default:''"` 

	// BackgroundColor is a hex colour for the community's content background.
	// Admins set this via the settings panel; empty means default theme background.
	BackgroundColor string `json:"background_color" gorm:"type:varchar(7);default:''"`
	MinPostNotoriety    float64 `json:"min_post_notoriety" gorm:"default:0;not null"`
	MinCommentNotoriety float64 `json:"min_comment_notoriety" gorm:"default:0;not null"`

	// AutoApproveFreeNotes when true causes free (price=0) notes to skip the
	// Pending state and go directly to Approved on creation.
	AutoApproveFreeNotes bool `json:"auto_approve_free_notes" gorm:"default:false"`

	Admins      []User    `json:"admins" gorm:"many2many:user_admins;"`
	Members     []User    `json:"members" gorm:"many2many:user_memberships;"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserAdmin is the explicit join table model for user_admins.
// It adds a CreatedAt timestamp so that admin seniority (who was added first)
// can be determined reliably. GORM auto-populates CreatedAt when using
// db.SetupJoinTable + Association("Admins").Append.
type UserAdmin struct {
	UserID      uint64    `gorm:"primaryKey"`
	SubnoteryID uint      `gorm:"primaryKey"`
	CreatedAt   time.Time
}
