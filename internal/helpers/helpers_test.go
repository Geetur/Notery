// helpers_test.go — Tests for helpers package: pagination bounds.
package helpers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func paginationFromQuery(query string) Pagination {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?"+query, nil)
	return ParsePagination(c)
}

func TestParsePagination_Defaults(t *testing.T) {
	p := paginationFromQuery("")
	if p.Page != DefaultPage {
		t.Errorf("expected page %d, got %d", DefaultPage, p.Page)
	}
	if p.Limit != DefaultLimit {
		t.Errorf("expected limit %d, got %d", DefaultLimit, p.Limit)
	}
}

func TestParsePagination_PageCappedAtMax(t *testing.T) {
	p := paginationFromQuery("page=999999")
	if p.Page != MaxPage {
		t.Errorf("expected page capped at %d, got %d", MaxPage, p.Page)
	}
}

func TestParsePagination_NegativePage(t *testing.T) {
	p := paginationFromQuery("page=-5")
	if p.Page != 1 {
		t.Errorf("expected page 1 for negative input, got %d", p.Page)
	}
}

func TestParsePagination_LimitCappedAtMax(t *testing.T) {
	p := paginationFromQuery("limit=500")
	if p.Limit != DefaultLimit {
		t.Errorf("expected limit reset to default %d for over-max, got %d", DefaultLimit, p.Limit)
	}
}

func TestParsePagination_ValidValues(t *testing.T) {
	p := paginationFromQuery("page=3&limit=50")
	if p.Page != 3 {
		t.Errorf("expected page 3, got %d", p.Page)
	}
	if p.Limit != 50 {
		t.Errorf("expected limit 50, got %d", p.Limit)
	}
	if p.Offset != 100 {
		t.Errorf("expected offset 100, got %d", p.Offset)
	}
}
