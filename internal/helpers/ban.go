// ban.go — Ban check helpers for enforcing community and site-wide bans.
package helpers

import (
	"errors"

	"github.com/Geetur/Notery/internal/models"
	"gorm.io/gorm"
)

// CheckSubnoteryBan returns the active ban for a user in a specific subnotery, or nil.
// Expired bans are cleaned up on read: if found expired, deleted and nil returned.
func CheckSubnoteryBan(db *gorm.DB, userID uint64, subnoteryID uint) (*models.Ban, error) {
	var ban models.Ban
	err := db.Where("user_id = ? AND subnotery_id = ?", userID, subnoteryID).First(&ban).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if ban.IsExpired() {
		db.Delete(&ban)
		return nil, nil
	}
	return &ban, nil
}

// CheckSiteBan returns the active site-wide ban (subnotery_id=0) for a user, or nil.
func CheckSiteBan(db *gorm.DB, userID uint64) (*models.Ban, error) {
	var ban models.Ban
	err := db.Where("user_id = ? AND subnotery_id = 0", userID).First(&ban).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if ban.IsExpired() {
		db.Delete(&ban)
		return nil, nil
	}
	return &ban, nil
}

// CheckAnyBan checks for both site-wide and subnotery bans.
// Returns the most relevant active ban, or nil if unbanned.
func CheckAnyBan(db *gorm.DB, userID uint64, subnoteryID uint) (*models.Ban, error) {
	// Site-wide ban takes priority
	siteBan, err := CheckSiteBan(db, userID)
	if err != nil {
		return nil, err
	}
	if siteBan != nil {
		return siteBan, nil
	}
	return CheckSubnoteryBan(db, userID, subnoteryID)
}
