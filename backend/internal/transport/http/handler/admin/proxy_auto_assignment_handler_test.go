package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProxyAutoAssignmentSettingsHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := &stubAdminService{
		proxyAutoAssignmentSettings: &service.ProxyAutoAssignmentSettings{
			Enabled:                    false,
			HealthCheckEnabled:         false,
			HealthCheckIntervalMinutes: 15,
			FailureThreshold:           3,
		},
		rebalancedProxyAccounts: 7,
	}
	handler := NewProxyHandler(adminSvc)
	router := gin.New()
	router.GET("/proxies/auto-assignment", handler.GetAutoAssignmentSettings)
	router.PUT("/proxies/auto-assignment", handler.UpdateAutoAssignmentSettings)
	router.POST("/proxies/auto-assignment/rebalance", handler.RebalanceAutoAssignments)

	getRecorder := httptest.NewRecorder()
	router.ServeHTTP(getRecorder, httptest.NewRequest(http.MethodGet, "/proxies/auto-assignment", nil))
	require.Equal(t, http.StatusOK, getRecorder.Code)
	require.Contains(t, getRecorder.Body.String(), `"health_check_interval_minutes":15`)

	updateRecorder := httptest.NewRecorder()
	updateRequest := httptest.NewRequest(http.MethodPut, "/proxies/auto-assignment", bytes.NewBufferString(
		`{"enabled":true,"health_check_enabled":true,"health_check_interval_minutes":5,"failure_threshold":2}`,
	))
	updateRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(updateRecorder, updateRequest)
	require.Equal(t, http.StatusOK, updateRecorder.Code)
	require.Contains(t, updateRecorder.Body.String(), `"enabled":true`)
	require.True(t, adminSvc.proxyAutoAssignmentSettings.Enabled)
	require.Equal(t, 5, adminSvc.proxyAutoAssignmentSettings.HealthCheckIntervalMinutes)

	rebalanceRecorder := httptest.NewRecorder()
	router.ServeHTTP(rebalanceRecorder, httptest.NewRequest(http.MethodPost, "/proxies/auto-assignment/rebalance", nil))
	require.Equal(t, http.StatusOK, rebalanceRecorder.Code)
	require.Contains(t, rebalanceRecorder.Body.String(), `"reassigned_accounts":7`)
}

func TestUpdateProxyAutoAssignmentSettingsRejectsPartialPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewProxyHandler(&stubAdminService{})
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/proxies/auto-assignment", bytes.NewBufferString(`{"enabled":true}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateAutoAssignmentSettings(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
}
