// comment.go — Comment-related helper utilities.
package helpers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MustParseCommentID extracts the "comment_id" parameter as a comment ID.
// On failure, sends a 400 response and returns 0, false.
func MustParseCommentID(c *gin.Context) (uint64, bool) {
	id, ok := ParseUintParam(c, "comment_id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid comment ID"})
		return 0, false
	}
	return id, true
}
