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
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingHandler_OpenAIOAuthForceRelayPersistsAcrossGetAndOtherSaves(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{
		service.SettingKeyPromoCodeEnabled: "true",
	}}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	request := func(method string, body []byte) map[string]any {
		t.Helper()
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(method, "/api/v1/admin/settings", bytes.NewReader(body))
		if method == http.MethodPut {
			c.Request.Header.Set("Content-Type", "application/json")
			handler.UpdateSettings(c)
		} else {
			handler.GetSettings(c)
		}
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var result response.Response
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
		data, ok := result.Data.(map[string]any)
		require.True(t, ok)
		return data
	}

	baseURL := "https://relay.example/backend-api/codex"
	saved := request(http.MethodPut, []byte(`{"openai_oauth_force_relay_enabled":true,"openai_oauth_force_relay_base_url":"`+baseURL+`"}`))
	require.Equal(t, true, saved[service.SettingKeyOpenAIOAuthForceRelayEnabled])
	require.Equal(t, baseURL, saved[service.SettingKeyOpenAIOAuthForceRelayBaseURL])
	require.Equal(t, "true", repo.values[service.SettingKeyOpenAIOAuthForceRelayEnabled])
	require.Equal(t, baseURL, repo.values[service.SettingKeyOpenAIOAuthForceRelayBaseURL])

	loaded := request(http.MethodGet, nil)
	require.Equal(t, true, loaded[service.SettingKeyOpenAIOAuthForceRelayEnabled])
	require.Equal(t, baseURL, loaded[service.SettingKeyOpenAIOAuthForceRelayBaseURL])

	request(http.MethodPut, []byte(`{"risk_control_enabled":true}`))
	loaded = request(http.MethodGet, nil)
	require.Equal(t, true, loaded[service.SettingKeyOpenAIOAuthForceRelayEnabled])
	require.Equal(t, baseURL, loaded[service.SettingKeyOpenAIOAuthForceRelayBaseURL])
}
