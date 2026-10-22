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

func TestSettingHandlerChannelMonitorTimelineRoundtripAndPartialSave(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{}}
	svc := service.NewSettingService(repo, &config.Config{})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
	request := func(method, body string) map[string]any {
		t.Helper()
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(method, "/api/v1/admin/settings", bytes.NewBufferString(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		if method == http.MethodPut {
			handler.UpdateSettings(ctx)
		} else {
			handler.GetSettings(ctx)
		}
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var payload struct {
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
		return payload.Data
	}
	require.Equal(t, float64(20), request(http.MethodGet, "")["channel_monitor_timeline_points"])
	require.Equal(t, float64(35), request(http.MethodPut, `{"channel_monitor_timeline_points":35}`)["channel_monitor_timeline_points"])
	request(http.MethodPut, `{"site_name":"Monitor fixture"}`)
	require.Equal(t, float64(35), request(http.MethodGet, "")["channel_monitor_timeline_points"])
	public, err := svc.GetPublicSettings(t.Context())
	require.NoError(t, err)
	require.Equal(t, 35, public.ChannelMonitorTimelinePoints)
	require.Equal(t, "35", repo.values[service.SettingKeyChannelMonitorTimelinePoints])
	require.Equal(t, float64(1), request(http.MethodPut, `{"channel_monitor_timeline_points":0}`)["channel_monitor_timeline_points"])
	require.Equal(t, float64(60), request(http.MethodPut, `{"channel_monitor_timeline_points":999}`)["channel_monitor_timeline_points"])
}
