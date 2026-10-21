//go:build unit

package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingHandlerRequestTimezoneRoundtripAndPartialSave(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{}}
	svc := service.NewSettingService(repo, &config.Config{Gateway: config.GatewayConfig{OpenAIRequestTimezone: "Asia/Singapore"}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
	request := func(method, body string, wantStatus int) map[string]any {
		t.Helper()
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(method, "/api/v1/admin/settings", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		if method == http.MethodPut {
			handler.UpdateSettings(c)
		} else {
			handler.GetSettings(c)
		}
		require.Equal(t, wantStatus, recorder.Code, recorder.Body.String())
		var payload struct {
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
		return payload.Data
	}
	defaults := request(http.MethodGet, "", http.StatusOK)
	require.Equal(t, true, defaults["openai_request_timezone_enabled"])
	require.Equal(t, "Asia/Singapore", defaults["openai_request_timezone"])
	saved := request(http.MethodPut, `{"openai_request_timezone_enabled":false,"openai_request_timezone":"Asia/Tokyo"}`, http.StatusOK)
	require.Equal(t, false, saved["openai_request_timezone_enabled"])
	require.Equal(t, "Asia/Tokyo", saved["openai_request_timezone"])
	request(http.MethodPut, `{"site_name":"Timezone fixture"}`, http.StatusOK)
	loaded := request(http.MethodGet, "", http.StatusOK)
	require.Equal(t, false, loaded["openai_request_timezone_enabled"])
	require.Equal(t, "Asia/Tokyo", loaded["openai_request_timezone"])
	request(http.MethodPut, `{"openai_request_timezone":"Invalid/Timezone"}`, http.StatusBadRequest)
	require.Equal(t, "Asia/Tokyo", repo.values[service.SettingKeyOpenAIRequestTimezone])
}
