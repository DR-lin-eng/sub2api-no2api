package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newCodexSimulationSettingHandlerTest(cfg *config.Config) (*SettingHandler, *panelSettingHandlerRepo) {
	repo := &panelSettingHandlerRepo{}
	svc := service.NewSettingService(repo, cfg)
	return NewSettingHandler(svc, nil, nil, nil, nil, nil, nil), repo
}

func TestCodexSimulationSettingsHandlerHidesIdentitySecret(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.CodexSimulation = config.GatewayCodexSimulationConfig{
		FullSimulationEnabled: true,
		ContinuationMode:      "shadow",
		StateTTLSeconds:       600,
		IdentitySecret:        "handler-codex-simulation-secret-32-bytes",
	}
	h, _ := newCodexSimulationSettingHandlerTest(cfg)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/codex-simulation", nil)

	h.GetCodexSimulationSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"identity_secret_configured":true`)
	require.NotContains(t, recorder.Body.String(), cfg.Gateway.CodexSimulation.IdentitySecret)
	require.NotContains(t, recorder.Body.String(), `"identity_secret"`)
}

func TestCodexSimulationSettingsHandlerPersistsRuntimeOverrideAndGeneratesSecret(t *testing.T) {
	h, repo := newCodexSimulationSettingHandlerTest(&config.Config{})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/codex-simulation", bytes.NewBufferString(
		`{"full_simulation_enabled":true,"continuation_mode":"enforce","state_ttl_seconds":604800}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateCodexSimulationSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"identity_secret_configured":true`)
	var persisted service.CodexSimulationSettings
	require.NoError(t, json.Unmarshal([]byte(repo.values[service.SettingKeyCodexSimulationSettings]), &persisted))
	require.True(t, persisted.IdentitySecretConfigured())
	require.NotContains(t, recorder.Body.String(), persisted.IdentitySecret)
	require.NotContains(t, recorder.Body.String(), `"identity_secret"`)
}

func TestCodexSimulationSettingsHandlerPersistsCLevelSwitch(t *testing.T) {
	h, repo := newCodexSimulationSettingHandlerTest(&config.Config{})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/codex-simulation", bytes.NewBufferString(
		`{"full_simulation_enabled":false,"c_level_simulation_enabled":true,"continuation_mode":"off","state_ttl_seconds":604800}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateCodexSimulationSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var persisted service.CodexSimulationSettings
	require.NoError(t, json.Unmarshal([]byte(repo.values[service.SettingKeyCodexSimulationSettings]), &persisted))
	require.True(t, persisted.CLevelSimulationEnabled)
}

func TestRestoreOriginalCodexBehaviorOverwritesMalformedSettingsWithoutBody(t *testing.T) {
	h, repo := newCodexSimulationSettingHandlerTest(&config.Config{})
	repo.values = map[string]string{
		service.SettingKeyCodexSimulationSettings: `{malformed`,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/codex-simulation/restore-original", nil)

	h.RestoreOriginalCodexBehavior(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"full_simulation_enabled":false`)
	require.Contains(t, recorder.Body.String(), `"continuation_mode":"off"`)
	var persisted service.CodexSimulationSettings
	require.NoError(t, json.Unmarshal([]byte(repo.values[service.SettingKeyCodexSimulationSettings]), &persisted))
	require.False(t, persisted.FullSimulationEnabled)
	require.Equal(t, "off", persisted.ContinuationMode)
	require.Positive(t, persisted.StateTTLSeconds)
}

func TestCodexSimulationSettingsHandlerRejectsMalformedPayloads(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "null", body: `null`},
		{name: "partial", body: `{"full_simulation_enabled":false}`},
		{name: "invalid mode", body: `{"full_simulation_enabled":false,"continuation_mode":"on","state_ttl_seconds":60}`},
		{name: "invalid ttl", body: `{"full_simulation_enabled":false,"continuation_mode":"off","state_ttl_seconds":0}`},
		{name: "null field", body: `{"full_simulation_enabled":null,"continuation_mode":"off","state_ttl_seconds":60}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			h, repo := newCodexSimulationSettingHandlerTest(&config.Config{})
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/codex-simulation", strings.NewReader(test.body))
			c.Request.Header.Set("Content-Type", "application/json")

			h.UpdateCodexSimulationSettings(c)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.NotContains(t, repo.values, service.SettingKeyCodexSimulationSettings)
		})
	}
}
