//go:build unit

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

func TestUpdateRateLimit429CooldownSettingsPreservesOmittedQuotaCheckSwitch(t *testing.T) {
	handler, repo := newPanelSettingHandlerTest()
	repo.values = map[string]string{
		service.SettingKeyRateLimit429CooldownSettings: `{"enabled":true,"cooldown_seconds":5,"auto_disable_enabled":true,"auto_disable_threshold":3,"auto_disable_quota_check_enabled":true,"auto_enable_after_quota_reset_enabled":true,"auto_enable_when_quota_available_enabled":true}`,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/rate-limit-429-cooldown", bytes.NewBufferString(
		`{"enabled":true,"cooldown_seconds":8,"auto_disable_enabled":true,"auto_disable_threshold":4}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateRateLimit429CooldownSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, repo.values[service.SettingKeyRateLimit429CooldownSettings], `"auto_disable_quota_check_enabled":true`)
	require.Contains(t, repo.values[service.SettingKeyRateLimit429CooldownSettings], `"auto_enable_after_quota_reset_enabled":true`)
	require.Contains(t, repo.values[service.SettingKeyRateLimit429CooldownSettings], `"auto_enable_when_quota_available_enabled":true`)
	require.Contains(t, recorder.Body.String(), `"auto_disable_quota_check_enabled":true`)
	require.Contains(t, recorder.Body.String(), `"auto_enable_after_quota_reset_enabled":true`)
	require.Contains(t, recorder.Body.String(), `"auto_enable_when_quota_available_enabled":true`)
}

func TestUpdateRateLimit429CooldownSettingsUpdatesQuotaCheckSwitch(t *testing.T) {
	handler, repo := newPanelSettingHandlerTest()
	repo.values = map[string]string{
		service.SettingKeyRateLimit429CooldownSettings: `{"enabled":true,"cooldown_seconds":5,"auto_disable_enabled":true,"auto_disable_threshold":3,"auto_disable_quota_check_enabled":true,"auto_enable_after_quota_reset_enabled":false,"auto_enable_when_quota_available_enabled":false}`,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/rate-limit-429-cooldown", bytes.NewBufferString(
		`{"enabled":true,"cooldown_seconds":5,"auto_disable_enabled":true,"auto_disable_threshold":3,"auto_disable_quota_check_enabled":false,"auto_enable_after_quota_reset_enabled":true,"auto_enable_when_quota_available_enabled":true}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateRateLimit429CooldownSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, repo.values[service.SettingKeyRateLimit429CooldownSettings], `"auto_disable_quota_check_enabled":false`)
	require.Contains(t, repo.values[service.SettingKeyRateLimit429CooldownSettings], `"auto_enable_after_quota_reset_enabled":true`)
	require.Contains(t, repo.values[service.SettingKeyRateLimit429CooldownSettings], `"auto_enable_when_quota_available_enabled":true`)
}
