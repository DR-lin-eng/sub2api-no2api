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

func TestSettingHandlerImagesModeDefaultsAndPartialSave(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{}}
	svc := service.NewSettingService(repo, &config.Config{})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
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
		var payload struct {
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
		return payload.Data
	}
	require.Equal(t, false, request(http.MethodGet, "")["openai_images_legacy_enabled"])
	require.Equal(t, true, request(http.MethodPut, `{"openai_images_legacy_enabled":true}`)["openai_images_legacy_enabled"])
	require.True(t, svc.OpenAIImagesLegacyEnabled(t.Context()))
	request(http.MethodPut, `{"site_name":"unrelated partial save"}`)
	require.Equal(t, true, request(http.MethodGet, "")["openai_images_legacy_enabled"])
	require.Equal(t, "true", repo.values[service.SettingKeyOpenAIImagesLegacyEnabled])
	require.Equal(t, false, request(http.MethodPut, `{"openai_images_legacy_enabled":false}`)["openai_images_legacy_enabled"])
	require.False(t, svc.OpenAIImagesLegacyEnabled(t.Context()))
}
