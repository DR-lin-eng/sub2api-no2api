//go:build unit

package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
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

func TestUpdateOAuth401CleanupSettings(t *testing.T) {
	handler, repo := newPanelSettingHandlerTest()
	repo.values = map[string]string{
		service.SettingKeyOAuth401CleanupSettings: `{"enabled":false}`,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/oauth-401-cleanup", bytes.NewBufferString(
		`{"enabled":true}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateOAuth401CleanupSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"enabled":true,"revoke_same_workspace":false}`, repo.values[service.SettingKeyOAuth401CleanupSettings])
	require.Contains(t, recorder.Body.String(), `"enabled":true`)
}

func TestUpdateOAuth401CleanupSettingsPreservesIndependentSwitches(t *testing.T) {
	for _, tt := range []struct{ name, before, request, want string }{
		{"workspace_only", `{"enabled":true}`, `{"revoke_same_workspace":true}`, `{"enabled":true,"revoke_same_workspace":true}`},
		{"legacy_client", `{"enabled":false,"revoke_same_workspace":true}`, `{"enabled":true}`, `{"enabled":true,"revoke_same_workspace":true}`},
		{"disable_workspace", `{"enabled":true,"revoke_same_workspace":true}`, `{"revoke_same_workspace":false}`, `{"enabled":true,"revoke_same_workspace":false}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handler, repo := newPanelSettingHandlerTest()
			repo.values = map[string]string{service.SettingKeyOAuth401CleanupSettings: tt.before}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/oauth-401-cleanup", bytes.NewBufferString(tt.request))
			c.Request.Header.Set("Content-Type", "application/json")
			handler.UpdateOAuth401CleanupSettings(c)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			require.JSONEq(t, tt.want, repo.values[service.SettingKeyOAuth401CleanupSettings])
			require.Contains(t, recorder.Body.String(), `"revoke_same_workspace":`)
		})
	}
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
