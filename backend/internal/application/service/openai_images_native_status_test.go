package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestNativeCodexImageLateErrorKeepsStatus(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("X-Codex-Image-Turn-Id", "fixture-turn")
	stop := StartOpenAIImagesJSONKeepalive(c, time.Nanosecond)
	defer stop()
	// A native request must never install the heartbeat writer or commit a 200,
	// even when the configured interval would have elapsed before the error.
	require.Nil(t, openAIImagesJSONKeepaliveFromContext(c))
	require.False(t, c.Writer.Written())
	require.True(t, writeOpenAIImagesUpstreamErrorResponse(c, &OpenAIImagesUpstreamError{
		StatusCode: http.StatusForbidden, ErrorType: "permission_error", Code: "fixture_denied", Message: "request rejected",
	}))
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"code":"fixture_denied"`)
}
