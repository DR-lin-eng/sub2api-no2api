package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountQualitySettingsUseIndependentSettingKeys(t *testing.T) {
	repo := &inspectionSettingRepoStub{values: map[string]string{}}
	svc := NewAccountQualityMonitoringService(nil, repo, nil, nil, nil, nil)
	settings := DefaultAccountQualitySettings()
	settings.Enabled = true
	settings.SourceGroupID = nil
	saved, err := svc.UpdateSettings(context.Background(), &settings)
	require.NoError(t, err)
	require.True(t, saved.Enabled)
	require.Contains(t, repo.values, SettingKeyAccountQualitySettings)
	require.NotContains(t, repo.values, SettingKeyAccountInspectionSettings)

	var encoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(repo.values[SettingKeyAccountQualitySettings]), &encoded))
	require.Equal(t, true, encoded["enabled"])
	require.NotContains(t, encoded, "quality_monitoring_enabled")
}

func TestAccountInspectionSettingsDoNotSerializeQualityPolicy(t *testing.T) {
	payload, err := json.Marshal(DefaultAccountInspectionSettings())
	require.NoError(t, err)
	require.NotContains(t, string(payload), "quality_monitoring_enabled")
	require.NotContains(t, string(payload), "quality_source_group_id")
}
