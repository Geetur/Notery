// Package models contains the definition of the User model
package models


import (
	"golang.org/x/crypto/bcrypt"
)

// because this is safety critical code, it's good practice
// to encapsulate password handling within the model itself

// User represents a user in the system
type User struct {
	ID 	 uint   `json:"id" gorm:"primaryKey"`
	Email string `json:"email" gorm:"unique; not null"`
	// the dash is to prevent the field from being exposed in JSON responses
	Password     string `json:"password" gorm:"-"`     // input only
    Hash string `json:"-" gorm:"not null"`     // persisted
	// all subnoteries a user is admin of
	AdminOf []Subnotery `json:"admin_of" gorm:"many2many:user_admins;"`

	IsGlobalAdmin bool   `json:"is_global_admin" gorm:"default:false"`
}
// SetPassword is a GORM hook that hashes the password before creating a new user record
func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Hash = string(hash)
	return nil
}

// CheckPassword compares the provided password with the stored hash
func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Hash), []byte(password))
	return err == nil
}