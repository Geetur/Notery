package handlers

import (
	"net/http"
	
	"log"
	"github.com/Geetur/Notery/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NoteHandler structure
type NoteHandler struct {
	DB *gorm.DB
}

// CreateNoteHandler initializes a new NoteHandler with the given database connection
func CreateNoteHandler(db *gorm.DB) *NoteHandler {
	return &NoteHandler{DB: db}
}

// now we want to define different functions to handle CRUD operations for notes
// so, if we want to create a new note, and also get all notes, we call
// two seperate functions

// CreateNote is a method of NoteHandler that handles the creation of a new note.
// this allows us to do handler.DB.CreateNote(...)
func (handler *NoteHandler) CreateNote(c *gin.Context) {

	var note models.Note

	// if the structure of request body does not match the Note struct
	// we return a bad request error
	if err := c.ShouldBindJSON(&note); err != nil {
		// 400 Bad Request status code
		log.Println("Failed to bind JSON:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// if the structure is valid, we create the note in the database
	if err := handler.DB.Create(&note).Error; err != nil {
		// 500 Internal Server Error status code
		log.Println("Failed to create note:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to internally create note"})
		return
	}
	// 201 Created status code
	c.JSON(http.StatusCreated, note)
}


// even though we will use meilisearch, we still want
// to be able to get singular notes from the database directly

func (handler *NoteHandler) GetNoteByID(c *gin.Context) {
	noteID := c.Param("id")
	var note models.Note
	if err := handler.DB.First(&note, noteID).Error; err != nil {
		log.Printf("Failed to fetch note with ID: %s", noteID)
		c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
		return
	}
	c.JSON(http.StatusOK, note)
} 