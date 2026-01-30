// Package models defines the domain entities and their database mappings.
package models

import (
	"time"
)

// Subnotery represents a note community with designated admins.
type Subnotery struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"unique; not null"`
	Admins  []User    `json:"admins" gorm:"many2many:user_admins;"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
