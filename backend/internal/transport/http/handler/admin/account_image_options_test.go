//go:build unit

package admin

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountImageTestRejectsMalformedFrameCount(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/api/v1/admin/accounts/1/test", strings.NewReader(`{"model_id":"gpt-image-2.6-flare","output_format":"gif","frame_count":"invalid"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	(&AccountHandler{}).Test(c)
	require.Equal(t, 400, rec.Code)
}
