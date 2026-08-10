package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClusterReadinessRejectsBusinessTrafficWhileNodeIsNotReady(t *testing.T) {
	gin.SetMode(gin.TestMode)
	releases := service.NewClusterReleaseService(nil, nil, nil, &config.Config{
		Deployment: config.DeploymentConfig{Mode: config.DeploymentModeMultiInstance},
	})
	router := gin.New()
	router.Use(ClusterReadiness(releases))
	router.GET("/api/v1/test", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/test", nil))

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"reason":"NODE_NOT_READY"`)
}

func TestClusterReadinessAlwaysAllowsHealthProbes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	releases := service.NewClusterReleaseService(nil, nil, nil, &config.Config{
		Deployment: config.DeploymentConfig{Mode: config.DeploymentModeMultiInstance},
	})
	router := gin.New()
	router.Use(ClusterReadiness(releases))
	router.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestClusterReadinessAllowsClusterRecoveryTrafficWhileNodeIsNotReady(t *testing.T) {
	gin.SetMode(gin.TestMode)
	releases := service.NewClusterReleaseService(nil, nil, nil, &config.Config{
		Deployment: config.DeploymentConfig{Mode: config.DeploymentModeMultiInstance},
	})
	router := gin.New()
	router.Use(ClusterReadiness(releases))
	router.GET("/api/v1/admin/cluster/status", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/cluster/status", nil))

	require.Equal(t, http.StatusNoContent, recorder.Code)
}
