package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUpdateAdvancedSettingsLegacyClientPreservesBusinessLimited429Setting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newTestSettingRepo()
	ops := service.NewOpsService(nil, repo, &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)

	current, err := ops.GetOpsAdvancedSettings(context.Background())
	require.NoError(t, err)
	current.RecordBusinessLimited429 = false
	_, err = ops.UpdateOpsAdvancedSettings(context.Background(), current)
	require.NoError(t, err)

	raw, err := json.Marshal(current)
	require.NoError(t, err)
	var legacyPayload map[string]any
	require.NoError(t, json.Unmarshal(raw, &legacyPayload))
	delete(legacyPayload, "record_business_limited_429")
	raw, err = json.Marshal(legacyPayload)
	require.NoError(t, err)

	router := gin.New()
	router.PUT("/advanced-settings", NewOpsHandler(ops).UpdateAdvancedSettings)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/advanced-settings", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.False(t, ops.OpsAdvancedSettingsSnapshot().RecordBusinessLimited429)

	legacyPayload["record_business_limited_429"] = true
	raw, err = json.Marshal(legacyPayload)
	require.NoError(t, err)
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/advanced-settings", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.True(t, ops.OpsAdvancedSettingsSnapshot().RecordBusinessLimited429)
}
