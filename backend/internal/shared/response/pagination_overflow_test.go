package response

import (
	"fmt"
	"math"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPaginationRejectsNumericOverflowAndClampsOffset(t *testing.T) {
	for _, query := range []string{"page=18446744073709551617", "page=999999999999999999999999999999999999", "page=12x", "page=-1", "page=+2"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/?"+query, nil)
		page, size := ParsePagination(c)
		if page != 1 || size != 20 {
			t.Fatalf("%s => %d/%d", query, page, size)
		}
	}
	for _, size := range []int{1, 20, 1000} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", fmt.Sprintf("/?page=%d&page_size=%d", math.MaxInt, size), nil)
		page, parsedSize := ParsePagination(c)
		if parsedSize != size || page < 1 || page > math.MaxInt/size {
			t.Fatalf("page=%d size=%d", page, parsedSize)
		}
		offset := (page - 1) * size
		if offset < 0 || offset > math.MaxInt-size {
			t.Fatalf("unsafe offset=%d", offset)
		}
	}
}
