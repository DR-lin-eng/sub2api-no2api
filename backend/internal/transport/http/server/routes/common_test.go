package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestReadyRouteReportsReleaseReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	releases := service.NewClusterReleaseService(nil, nil, nil, &config.Config{
		Deployment: config.DeploymentConfig{Mode: config.DeploymentModeMultiInstance},
	})
	router := gin.New()
	RegisterCommonRoutes(router, releases)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"ready":false`)
	require.Contains(t, recorder.Body.String(), `"reason":"initializing_release_state"`)
}
