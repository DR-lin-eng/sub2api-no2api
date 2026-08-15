//go:build unit

package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWriteOpenAICompactSSEFailureMessageCarriesCreatedAt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	writeOpenAICompactSSEFailureMessage(c, http.StatusBadGateway, "upstream_error", "boom")

	_, data, found := strings.Cut(recorder.Body.String(), "data: ")
	require.True(t, found)
	var event struct {
		Type     string `json:"type"`
		Response struct {
			CreatedAt int64 `json:"created_at"`
		} `json:"response"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(data)), &event))
	require.Equal(t, "response.failed", event.Type)
	require.Greater(t, event.Response.CreatedAt, int64(0))
}
