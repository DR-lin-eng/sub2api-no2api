//go:build unit

package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type channelMonitorRouteSettingRepoStub struct {
	service.SettingRepository
}

func (s *channelMonitorRouteSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{
		service.SettingKeyChannelMonitorEnabled:            "true",
		service.SettingKeyChannelMonitorPublicShareEnabled: "true",
	}, nil
}

func TestChannelMonitorPublicBatchRouteLimitsRequestBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settingService := service.NewSettingService(&channelMonitorRouteSettingRepoStub{}, &config.Config{})
	router := gin.New()
	RegisterChannelMonitorPublicRoutes(
		router.Group("/api/v1"),
		&handler.Handlers{ChannelMonitor: handler.NewChannelMonitorUserHandler(nil, settingService)},
		middleware.OptionalJWTAuthMiddleware(func(c *gin.Context) { c.Next() }),
		nil,
		nil,
	)

	body := `{"ids":[1],"padding":"` + strings.Repeat("x", int(channelMonitorPublicBatchBodyLimit)) + `"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/channel-status-share/status/batch", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
}
