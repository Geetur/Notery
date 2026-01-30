// Package models defines the domain entities and their database mappings.
package models

import (
	"golang.org/x/crypto/bcrypt"
)

// User represents an authenticated user with optional admin privileges.
type User struct {
	ID            uint        `json:"id" gorm:"primaryKey"`
	Email         string      `json:"email" gorm:"unique; not null"`
	Password      string      `json:"password" gorm:"-"` // input only, not persisted
	Hash          string      `json:"-" gorm:"not null"` // bcrypt hash, not exposed in JSON
	AdminOf       []Subnotery `json:"admin_of" gorm:"many2many:user_admins;"`
	IsGlobalAdmin bool        `json:"is_global_admin" gorm:"default:false"`
}

// SetPassword hashes the given password and stores it in the Hash field.
func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
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
