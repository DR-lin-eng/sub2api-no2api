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

func TestSettingHandlerImageIndependentSchedulingPartialSave(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{}}
	svc := service.NewSettingService(repo, &config.Config{})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
	request := func(method, body string) bool {
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
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var payload struct {
			Data struct {
				Enabled bool `json:"image_independent_scheduling_enabled"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
		return payload.Data.Enabled
	}
	require.False(t, request(http.MethodGet, ""))
	require.False(t, svc.ImageIndependentSchedulingEnabled(t.Context()))
	require.True(t, request(http.MethodPut, `{"image_independent_scheduling_enabled":true}`))
	require.True(t, svc.ImageIndependentSchedulingEnabled(t.Context()), "saving must immediately refresh this node's runtime policy")
	require.True(t, request(http.MethodPut, `{"site_name":"keep image setting"}`))
	require.Equal(t, "true", repo.values[service.SettingKeyImageIndependentSchedulingEnabled])
	require.False(t, request(http.MethodPut, `{"image_independent_scheduling_enabled":false}`))
	require.False(t, svc.ImageIndependentSchedulingEnabled(t.Context()))
}
