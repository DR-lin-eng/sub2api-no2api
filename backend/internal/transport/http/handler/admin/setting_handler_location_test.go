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

func TestSettingHandlerLocationRoundtripAndPartialSave(t *testing.T) {
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
	require.Equal(t, false, defaults["openai_location_enabled"])
	saved := request(http.MethodPut, `{"openai_location_enabled":true,"openai_location_latitude":31.2304,"openai_location_longitude":121.4737}`, http.StatusOK)
	require.Equal(t, true, saved["openai_location_enabled"])
	require.Equal(t, 31.2304, saved["openai_location_latitude"])
	request(http.MethodPut, `{"site_name":"location fixture"}`, http.StatusOK)
	loaded := request(http.MethodGet, "", http.StatusOK)
	require.Equal(t, true, loaded["openai_location_enabled"])
	require.Equal(t, 121.4737, loaded["openai_location_longitude"])
	request(http.MethodPut, `{"openai_location_latitude":91}`, http.StatusBadRequest)
	require.Equal(t, "31.2304", repo.values[service.SettingKeyOpenAILocationLatitude])
	disabled := request(http.MethodPut, `{"openai_location_enabled":false}`, http.StatusOK)
	require.Equal(t, false, disabled["openai_location_enabled"])
	require.Equal(t, 31.2304, disabled["openai_location_latitude"])
}
