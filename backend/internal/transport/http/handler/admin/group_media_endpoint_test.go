package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCompositeMediaRouteRequestAcceptsVideos(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, preview := range []bool{false, true} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		payload := `{"public_model":"seedance","target_platform":"multimodal","endpoint":"videos"}`
		if preview {
			payload = `{"model":"seedance","endpoint":"videos"}`
		}
		c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
		c.Request.Header.Set("Content-Type", "application/json")
		if preview {
			var request CompositeRoutePreviewRequest
			require.NoError(t, c.ShouldBindJSON(&request))
			require.Equal(t, "videos", request.Endpoint)
		} else {
			var request CompositeRouteRequest
			require.NoError(t, c.ShouldBindJSON(&request))
			require.Equal(t, "videos", request.Endpoint)
		}
	}
}
