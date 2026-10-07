package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type panelSettingHandlerRepo struct {
	values map[string]string
}

func (r *panelSettingHandlerRepo) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}

func (r *panelSettingHandlerRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}

func (r *panelSettingHandlerRepo) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *panelSettingHandlerRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, nil
}

func (r *panelSettingHandlerRepo) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		if err := r.Set(context.Background(), key, value); err != nil {
			return err
		}
	}
	return nil
}

func (r *panelSettingHandlerRepo) GetAll(context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *panelSettingHandlerRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func newPanelSettingHandlerTest() (*SettingHandler, *panelSettingHandlerRepo) {
	repo := &panelSettingHandlerRepo{}
	svc := service.NewSettingService(repo, &config.Config{})
	return NewSettingHandler(svc, nil, nil, nil, nil, nil, nil), repo
}

func TestGetPanelRateLimitSettingsMissingDefaultsEnabled(t *testing.T) {
	h, _ := newPanelSettingHandlerTest()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/panel-rate-limit", nil)

	h.GetPanelRateLimitSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"enabled":true`)
}

func TestUpdatePanelRateLimitSettingsPersistsAndRefreshes(t *testing.T) {
	h, repo := newPanelSettingHandlerTest()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/panel-rate-limit", bytes.NewBufferString(
		`{"enabled":true,"user_rpm":120,"heavy_rpm":30,"exempt_admin":true,"public_ip_rpm":60}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdatePanelRateLimitSettings(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, repo.values[service.SettingKeyPanelRateLimitSettings], `"enabled":true`)
	require.Contains(t, recorder.Body.String(), `"heavy_rpm":30`)
}

func TestUpdatePanelRateLimitSettingsRejectsNegativeLimit(t *testing.T) {
	h, _ := newPanelSettingHandlerTest()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/panel-rate-limit", bytes.NewBufferString(
		`{"enabled":true,"user_rpm":-1,"heavy_rpm":30,"exempt_admin":true,"public_ip_rpm":60}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdatePanelRateLimitSettings(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestUpdatePanelRateLimitSettingsRejectsNullAndPartialPayloads(t *testing.T) {
	for _, testCase := range []struct {
		name string
		body string
	}{
		{name: "null", body: `null`},
		{name: "missing limits", body: `{"enabled":true}`},
		{name: "null field", body: `{"enabled":true,"user_rpm":null,"heavy_rpm":30,"exempt_admin":false,"public_ip_rpm":60}`},
		{name: "invalid field type", body: `{"enabled":true,"user_rpm":"120","heavy_rpm":30,"exempt_admin":false,"public_ip_rpm":60}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			h, repo := newPanelSettingHandlerTest()
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/panel-rate-limit", bytes.NewBufferString(testCase.body))
			c.Request.Header.Set("Content-Type", "application/json")

			h.UpdatePanelRateLimitSettings(c)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.NotContains(t, repo.values, service.SettingKeyPanelRateLimitSettings)
		})
	}
}
