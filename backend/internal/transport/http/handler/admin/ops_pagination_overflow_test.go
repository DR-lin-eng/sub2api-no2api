//go:build unit

package admin

import (
	"fmt"
	"math"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRankedStatsRejectOverflowPage(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", fmt.Sprintf("/?page=%d&page_size=100", math.MaxInt), nil)
	_, err := parseOpsOpenAITokenStatsFilter(c)
	require.ErrorContains(t, err, "invalid page")
	_, err = parseOpsUserUsageStatsFilter(c)
	require.ErrorContains(t, err, "invalid page")
	c, _ = gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?page=2&page_size=100", nil)
	filter, err := parseOpsOpenAITokenStatsFilter(c)
	require.NoError(t, err)
	require.Equal(t, 2, filter.Page)
}
