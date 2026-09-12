package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type qualityProbeDeadlineStub struct{ deadline time.Time }

func (p *qualityProbeDeadlineStub) RunQualityTestBackground(ctx context.Context, _ int64, _, _, _ string) (*ScheduledTestResult, error) {
	if deadline, ok := ctx.Deadline(); ok {
		p.deadline = deadline
	}
	return &ScheduledTestResult{Status: "failed", ErrorMessage: "Stream read error: context deadline exceeded"}, nil
}

func TestAccountQualitySettingsUseIndependentSettingKeys(t *testing.T) {
	repo := &inspectionSettingRepoStub{values: map[string]string{}}
	svc := NewAccountQualityMonitoringService(nil, repo, nil, nil, nil, nil)
	settings := DefaultAccountQualitySettings()
	settings.Enabled = true
	settings.TimeoutSeconds = 180
	settings.SourceGroupID = nil
	saved, err := svc.UpdateSettings(context.Background(), &settings)
	require.NoError(t, err)
	require.True(t, saved.Enabled)
	require.Contains(t, repo.values, SettingKeyAccountQualitySettings)
	require.NotContains(t, repo.values, SettingKeyAccountInspectionSettings)

	var encoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(repo.values[SettingKeyAccountQualitySettings]), &encoded))
	require.Equal(t, true, encoded["enabled"])
	require.Equal(t, float64(180), encoded["timeout_seconds"])
	require.NotContains(t, encoded, "quality_monitoring_enabled")
}

func TestAccountInspectionSettingsDoNotSerializeQualityPolicy(t *testing.T) {
	payload, err := json.Marshal(DefaultAccountInspectionSettings())
	require.NoError(t, err)
	require.NotContains(t, string(payload), "quality_monitoring_enabled")
	require.NotContains(t, string(payload), "quality_source_group_id")
}

func TestQualityProbeTimeoutIsLongerThanLegacyThirtySecondsAndReportsError(t *testing.T) {
	probe := &qualityProbeDeadlineStub{}
	repo := &qualityRepoStub{extra: map[int64]map[string]any{}}
	svc := &AccountQualityMonitoringService{accountRepo: repo, accountTestSvc: probe}
	account := Account{ID: 478, Name: "quality", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	results := []AccountInspectionAccountResult{neutralAccountInspectionResult(&account, time.Now().UTC())}
	settings := DefaultAccountQualitySettings()
	settings.TimeoutSeconds = 180

	err := svc.runQualityMonitoring(context.Background(), []Account{account}, results, nil, settings, time.Now().UTC())
	require.NoError(t, err)
	require.Greater(t, time.Until(probe.deadline), 170*time.Second)
	require.Equal(t, "error", results[0].QualityStatus)
	require.Zero(t, results[0].QualityConsecutiveFailures)
	require.Contains(t, results[0].QualityError, "context deadline exceeded")
}

func TestAccountQualitySettingsValidateTimeoutRange(t *testing.T) {
	settings := DefaultAccountQualitySettings()
	settings.TimeoutSeconds = 29
	require.Error(t, settings.validate())
	settings.TimeoutSeconds = 301
	require.Error(t, settings.validate())
	settings.TimeoutSeconds = 180
	require.NoError(t, settings.validate())
}

func TestAccountQualitySettingsMigrateLegacyPromptAndStages(t *testing.T) {
	repo := &inspectionSettingRepoStub{values: map[string]string{
		SettingKeyAccountQualitySettings: `{"enabled":true,"prompt":"legacy drawing prompt","timeout_seconds":120}`,
	}}
	svc := NewAccountQualityMonitoringService(nil, repo, nil, nil, nil, nil)
	settings, err := svc.GetSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.Stage1Enabled)
	require.True(t, settings.Stage2Enabled)
	require.Equal(t, "legacy drawing prompt", settings.Stage2Prompt)
	require.Equal(t, settings.Stage2Prompt, settings.Prompt)
}

func TestQualityDrawingTimeoutOnlyAppliesBeforeFirstOutput(t *testing.T) {
	ctx, cleanup := qualityStageContext(context.Background(), "stage2", 1)
	markQualityProbeOutput(ctx)
	time.Sleep(1100 * time.Millisecond)
	require.NoError(t, ctx.Err())
	cleanup()

	ctx, cleanup = qualityStageContext(context.Background(), "stage2", 1)
	defer cleanup()
	select {
	case <-ctx.Done():
		require.ErrorIs(t, ctx.Err(), context.Canceled)
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("drawing context did not time out before first output")
	}
}
