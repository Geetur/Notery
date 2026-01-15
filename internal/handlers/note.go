// Package handlers/note.go contains the HTTP handlers for note operations
package handlers

import (
	"errors"
	"net/http"
	"log"
	"strconv"

	"github.com/Geetur/Notery/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/meilisearch/meilisearch-go"
	"gorm.io/gorm"
)

// NoteHandler structure
type NoteHandler struct {
	DB *gorm.DB
	Search      meilisearch.ServiceManager
	SearchIndex string
}

// CreateNoteHandler initializes a new NoteHandler with the given database connection
func CreateNoteHandler(db *gorm.DB, search meilisearch.ServiceManager, indexName string) *NoteHandler {
	return &NoteHandler{
		DB:          db,
		Search:      search,
		SearchIndex: indexName,
	}
}

// now we want to define different functions to handle CRUD operations for notes
// so, if we want to create a new note, and also get all notes, we call
// two seperate functions, for example

// CreateNote is a method of NoteHandler that handles the creation of a new note.
// CreateNote interacts purely with the database to create a new note record
// CreateNote interacts with no other handler methods
func (handler *NoteHandler) CreateNote(c *gin.Context) {

	// declare a note variable to hold the incoming note data
	var note models.Note
	// if the structure of request body does not match the Note struct
	// we return a bad request error
	note.Status = "Pending" // default status
	if err := c.ShouldBindJSON(&note); err != nil {
		// 400 Bad Request status code
		log.Println("Failed to bind JSON:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	log.Println("Trying to create note with title:", note.Title)
	// if the structure is valid, we create the note in the database
	if err := handler.DB.Create(&note).Error; err != nil {
		// 500 Internal Server Error status code
		log.Println("Failed to create note:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to internally create note"})
		return
	}
	log.Println("Successfully created note with ID:", note.ID)
	// 201 Created status code
	c.JSON(http.StatusCreated, note)
}

// DeleteNote is a method of NoteHandler that handles the deletion of a note by ID.
// DeleteNote removes the note from both the database and Meilisearch if approved.
// DeleteNote interacts with the removeNoteFromIndex and indexNote handler methods.
func (handler *NoteHandler) DeleteNote(c *gin.Context) {
	noteID := c.Param("id")
	// declare a note variable to hold the fetched note
	var note models.Note
	// first we need to fetch the note to check its status
	log.Printf("Fetching note with ID: %s for deletion", noteID)
	if err := handler.DB.First(&note, noteID).Error; err != nil {
		// check if the error is record not found
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Note with ID: %s not found for deletion", noteID)
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
			return
		}
		// other errors
		log.Printf("Failed to fetch note with ID for deletion: %s", noteID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch note"})
		return
	}
	log.Printf("Note with ID: %s fetched successfully for deletion", noteID)
	// if the note is approved, we need to remove it from Meilisearch first
	log.Printf("Trying to delete note with ID from meilisearch: %s", noteID)
	if note.Status == "Approved" {
		if err := handler.removeNoteFromIndex(note.ID); err != nil {
			log.Printf("Failed to remove approved note from Meilisearch: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove approved note from search index"})
			return
		}
	}
	log.Printf("Successfully deleted note with ID from meilisearch: %s", noteID)
	// now we can delete the note from the database
	log.Printf("Deleting note with ID: %s from database", noteID)
	if err := handler.DB.Delete(&note).Error; err != nil {
		log.Printf("Failed to delete note with ID: %s", noteID)
		if note.Status == "Approved" {
			if reindexErr := handler.indexNote(note); reindexErr != nil {
				log.Printf("Failed to re-index note after delete error: %v", reindexErr)
			}
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete note"})
		return
	}
	log.Printf("Successfully deleted note with ID: %s from database", noteID)
	// successful deletion
	c.JSON(http.StatusOK, gin.H{"message": "Note deleted successfully"})
}

// RejectNote is a method of NoteHandler that handles rejecting a note by ID.
// RejectNote interacts with the database to update the note's status to "Rejected".
// RejectNote does not interact with any handler methods.
func (handler *NoteHandler) RejectNote(c *gin.Context) {
	noteID := c.Param("id")
	// update the note's status to "Rejected"
	log.Printf("Trying to reject note with ID: %s", noteID)
	if err := handler.DB.Model(&models.Note{}).Where("id = ?", noteID).Update("status", "Rejected").Error; err != nil {
		log.Printf("Failed to reject note with ID: %s", noteID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject note"})
		return
	}
	log.Printf("Successfully rejected note with ID: %s", noteID)
	// successful rejection
	c.JSON(http.StatusOK, gin.H{"message": "Note rejected successfully"})
}

// ApproveNote is a method of NoteHandler that handles approving a note by ID.
// ApproveNote updates the note's status to "Approved" and indexes it in Meilisearch.
// ApproveNote interacts with the indexNote handler method.
func (handler *NoteHandler) ApproveNote(c *gin.Context) {
	noteID := c.Param("id")
	var note models.Note
	// fetch the note to be approved
	log.Printf("Fetching note with ID: %s for approval", noteID)
	if err := handler.DB.First(&note, noteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
			return
		}
		log.Printf("Failed to fetch note with ID: %s", noteID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch note"})
		return
	}
	log.Printf("Successfully fetched note with ID: %s for approval", noteID)
	// update the note's status to "Approved"

	log.Printf("Trying to approve note with ID: %s", noteID)
	previousStatus := note.Status
	wasApproved := previousStatus == "Approved"
	if !wasApproved {
		if err := handler.DB.Model(&note).Update("status", "Approved").Error; err != nil {
			log.Printf("Failed to approve note with ID: %s", noteID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve note"})
			return
		}
		note.Status = "Approved"
		log.Printf("Successfully approved note with ID: %s", noteID)
	}
	log.Printf("trying to update search index with approved note with ID: %s", noteID)
	if err := handler.indexNote(note); err != nil {
		log.Printf("Failed to index approved note: %v", err)
		if !wasApproved {
			if rollbackErr := handler.DB.Model(&note).Update("status", previousStatus).Error; rollbackErr != nil {
				log.Printf("Failed to rollback note approval after indexing error: %v", rollbackErr)
			}
		}
		log.Printf("Failed to index approved note with ID: %s", noteID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to index approved note"})
		return
	}
	log.Printf("Successfully updated search index with approved note with ID: %s", noteID)
	// successful approval
	c.JSON(http.StatusOK, gin.H{"message": "Note approved successfully"})
}

// even though we will use meilisearch, we still want
// to be able to get singular notes from the database directly

// GetNoteByID is a method of NoteHandler that retrieves a note by its ID.
// GetNoteByID interacts with the database to fetch the note record.
// GetNoteByID does not interact with any handler methods.
func (handler *NoteHandler) GetNoteByID(c *gin.Context) {
	noteID := c.Param("id")
	var note models.Note
	// fetch the note by ID
	log.Printf("Fetching note with ID: %s", noteID)
	if err := handler.DB.First(&note, noteID).Error; err != nil {
		log.Printf("Failed to fetch note with ID: %s", noteID)
		c.JSON(http.StatusNotFound, gin.H{"error": "Note not found"})
		return
	}
	log.Printf("Successfully fetched note with ID: %s", noteID)
	c.JSON(http.StatusOK, note)
} 

// GetPendingNotes retrieves all notes with status "Pending"
// GetPendingNotes interacts with the database to fetch pending notes.
// getPendingNotes does not interact with any handler methods.
func (handler *NoteHandler) GetPendingNotes(c *gin.Context) {
	var notes []models.Note
	log.Println("Trying to fetch pending notes")
	if err := handler.DB.Where("status = ?", "Pending").Find(&notes).Error; err != nil {
		log.Println("Failed to fetch pending notes:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch pending notes"})
		return
	}
	log.Println("Successfully fetched pending notes")
	c.JSON(http.StatusOK, notes)
}

// GetApprovedNotes retrieves all notes with status "Approved"
// GetApprovedNotes interacts with the database to fetch approved notes.
// getApprovedNotes does not interact with any handler methods.
func (handler *NoteHandler) GetApprovedNotes(c *gin.Context) {
	var notes []models.Note
	log.Println("Trying to fetch approved notes")
	if err := handler.DB.Where("status = ?", "Approved").Find(&notes).Error; err != nil {
		log.Println("Failed to fetch approved notes:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch approved notes"})
		return
	}
	log.Println("Successfully fetched approved notes")
	c.JSON(http.StatusOK, notes)
}

// indexNote adds or updates a note in the Meilisearch index
// indexNote interacts with Meilisearch to index the approved note.
// indexNote interacts with no other handler methods.
func (handler *NoteHandler) indexNote(note models.Note) error {

	log.Printf("trying to index note with ID: %d in Meilisearch", note.ID)
	if handler.Search == nil || handler.SearchIndex == "" {
		return errors.New("meilisearch is not configured")
	}
	index := handler.Search.Index(handler.SearchIndex)
	_, err := index.AddDocuments([]models.Note{note}, &meilisearch.DocumentOptions{
		PrimaryKey: meilisearch.StringPtr("id"),
	})
	log.Printf("successfully indexed note with ID: %d in Meilisearch", note.ID)
	return err
}
// removeNoteFromIndex removes a note from the Meilisearch index
// removeNoteFromIndex interacts with Meilisearch to remove the note.
// removeNoteFromIndex interacts with no other handler methods.
func (handler *NoteHandler) removeNoteFromIndex(noteID uint) error {
	log.Printf("trying to remove note with ID: %d from Meilisearch index", noteID)
	if handler.Search == nil || handler.SearchIndex == "" {
		return errors.New("meilisearch is not configured")
	}
	index := handler.Search.Index(handler.SearchIndex)
	_, err := index.DeleteDocument(strconv.FormatUint(uint64(noteID), 10), nil)
	log.Printf("successfully removed note with ID: %d from Meilisearch index", noteID)
	return err
}
