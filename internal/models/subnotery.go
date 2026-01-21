// Package models/subnotery.go contains the Subnotery model definition
package models

import (
	"time"
)

// Subnotery represents a subnotery in the system
type Subnotery struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"unique; not null"`
	Admins  []User    `json:"admins" gorm:"many2many:user_admins;"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
