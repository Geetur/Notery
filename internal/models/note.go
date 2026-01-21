// Package models/note.go contains the Note model definition
package models

import "time"

// Note represents a note entity in the system
type Note struct {
	// the primary key for all intensive purposes
	ID uint `json:"id" gorm:"primaryKey"`
	// title and author obviously will be the main source of querying
	Title string `json:"title" gorm:"index"`
	Author string `json:"author" gorm:"index"`
	Status string `json:"status" gorm:"index"`
	SubnoteryID uint `json:"subnotery_id" gorm:"index"`
	Price float64 `json:"price"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

}