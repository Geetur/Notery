// ban.go — Ban model: community and site-wide user bans with variable durations.
//
// DESIGN:
//
//	Bans restrict users from interacting with a specific subnotery (or the entire
//	site, for global bans). Each ban records who issued it, an optional reason
//	message, and an expiry time. Permanent bans have a nil ExpiresAt.
//
//	Subnotery-scoped bans prevent joining, posting, commenting, and voting in that
//	community. Site-wide bans (SubnoteryID=0) block all write actions.
//
// DURATIONS:
//   - 1 day, 7 days, 30 days, 1 year, permanent
//
// STORAGE:
//
//	Unique constraint on (user_id, subnotery_id) ensures one active ban per user
//	per community. Site-wide bans use subnotery_id = 0.
package models

import "time"

// BanDuration represents a named ban duration for UI display and parsing.
type BanDuration string

const (
	BanDuration1Day    BanDuration = "1d"
	BanDuration7Days   BanDuration = "7d"
	BanDuration30Days  BanDuration = "30d"
	BanDuration1Year   BanDuration = "1y"
	BanDurationForever BanDuration = "permanent"
)

// ParseBanDuration converts a duration string to a time.Duration.
// Returns (0, false) for permanent bans or unrecognised values.
func ParseBanDuration(d BanDuration) (time.Duration, bool) {
	switch d {
	case BanDuration1Day:
		return 24 * time.Hour, true
	case BanDuration7Days:
		return 7 * 24 * time.Hour, true
	case BanDuration30Days:
		return 30 * 24 * time.Hour, true
	case BanDuration1Year:
		return 365 * 24 * time.Hour, true
	case BanDurationForever:
		return 0, false
	default:
		return 0, false
	}
}

// ValidBanDuration returns true if the duration is a recognised value.
func ValidBanDuration(d BanDuration) bool {
	switch d {
	case BanDuration1Day, BanDuration7Days, BanDuration30Days,
		BanDuration1Year, BanDurationForever:
		return true
	}
	return false
}

// Ban represents an active ban restricting a user from a subnotery or the site.
//
// Fields:
//   - UserID:       The banned user
//   - SubnoteryID:  The community (0 = site-wide ban)
//   - BannedBy:     Admin who issued the ban
//   - Reason:       Human-readable reason shown to the user
//   - ExpiresAt:    When the ban expires (nil = permanent)
type Ban struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	UserID      uint64     `json:"user_id" gorm:"not null;uniqueIndex:idx_bans_user_subnotery"`
	SubnoteryID uint       `json:"subnotery_id" gorm:"not null;uniqueIndex:idx_bans_user_subnotery;default:0"`
	BannedBy    uint64     `json:"banned_by" gorm:"not null"`
	Reason      string     `json:"reason" gorm:"type:text"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// IsExpired returns true if the ban has a set expiry that has elapsed.
// Permanent bans (ExpiresAt == nil) never expire.
func (b *Ban) IsExpired() bool {
	if b.ExpiresAt == nil {
		return false // permanent
	}
	return time.Now().After(*b.ExpiresAt)
}

// IsActive returns true if the ban is not expired.
func (b *Ban) IsActive() bool {
	return !b.IsExpired()
}
