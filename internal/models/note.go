package models
import "time"

type Note struct {
	// the primary key for all intensive purposes
	id uint `json:"id" gorm:"primaryKey"`
	// title and author obviously will be the main source of querying
	title string `json:"title" gorm:"index"`
	author string `json:"title" gorm:"index"`
	price float64 `json:"price"`
	createdAt time.Time `json:"created_at"`
	updatedAt time.Time `json:"updated_at"`

}