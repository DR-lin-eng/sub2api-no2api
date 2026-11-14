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

func TestSettingHandlerOAuthAuxiliaryPersistsPartialSavesAndRestart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{}}
	settings := service.NewSettingService(repo, &config.Config{})
	handler := NewSettingHandler(settings, nil, nil, nil, nil, nil, nil)
	request := func(method, body string) map[string]any {
		t.Helper()
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(method, "/api/v1/admin/settings", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		if method == http.MethodPut {
			handler.UpdateSettings(c)
		} else {
			handler.GetSettings(c)
		}
		require.Equal(t, 200, rec.Code, rec.Body.String())
		var result struct {
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
		return result.Data
	}
	for _, key := range []string{service.SettingKeyOpenAIOAuthAttestationEnabled, service.SettingKeyOpenAILenovoAttributionEnabled} {
		require.Equal(t, false, request(http.MethodGet, "")[key])
	}
	saved := request(http.MethodPut, `{"openai_oauth_attestation_enabled":true,"openai_lenovo_attribution_enabled":true}`)
	require.Equal(t, true, saved[service.SettingKeyOpenAIOAuthAttestationEnabled], "PUT response must retain the saved switch")
	require.Equal(t, true, saved[service.SettingKeyOpenAILenovoAttributionEnabled])
	require.True(t, settings.OpenAIOAuthAttestationEnabled(t.Context()))
	require.True(t, settings.OpenAILenovoAttributionEnabled(t.Context()))
	request(http.MethodPut, `{"site_name":"unrelated old client save"}`)
	settings = service.NewSettingService(repo, &config.Config{})
	handler = NewSettingHandler(settings, nil, nil, nil, nil, nil, nil)
	require.True(t, settings.OpenAIOAuthAttestationEnabled(t.Context()), "fresh instance must load the DB override without opening the admin page")
	require.True(t, settings.OpenAILenovoAttributionEnabled(t.Context()))
	saved = request(http.MethodPut, `{"openai_lenovo_attribution_enabled":false}`)
	require.Equal(t, false, saved[service.SettingKeyOpenAILenovoAttributionEnabled])
	require.Equal(t, true, saved[service.SettingKeyOpenAIOAuthAttestationEnabled])
	require.False(t, settings.OpenAILenovoAttributionEnabled(t.Context()))
	require.Equal(t, "false", repo.values[service.SettingKeyOpenAILenovoAttributionEnabled])
}
