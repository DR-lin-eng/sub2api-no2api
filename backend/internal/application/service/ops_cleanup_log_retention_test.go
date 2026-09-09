//go:build unit

package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOpsSystemLogsUseRuntimeRetentionIndependently(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	svc := makeOverlayService(repo, config.OpsCleanupConfig{ErrorLogRetentionDays: 30})
	ctx := context.Background()
	require.Equal(t, 30, svc.systemLogRetentionDays(ctx, 30))
	require.NoError(t, repo.Set(ctx, SettingKeyOpsRuntimeLogConfig, `{"retention_days":7,"redis_only":true}`))
	require.Equal(t, 7, svc.systemLogRetentionDays(ctx, 0), "error-log truncate must not discard retained system logs")
	require.NoError(t, repo.Set(ctx, SettingKeyOpsRuntimeLogConfig, `{"retention_days":-1}`))
	require.Equal(t, 30, svc.systemLogRetentionDays(ctx, 30))
}

func TestOpsCleanupPartialOverlayPreservesUnspecifiedRetention(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	base := config.OpsCleanupConfig{Enabled: true, Schedule: "0 2 * * *", ErrorLogRetentionDays: 30, MinuteMetricsRetentionDays: 14, HourlyMetricsRetentionDays: 90}
	svc := makeOverlayService(repo, base)
	require.NoError(t, repo.Set(context.Background(), SettingKeyOpsAdvancedSettings, `{"data_retention":{"cleanup_schedule":"0 3 * * *"}}`))
	svc.computeEffectiveLocked(context.Background())
	require.Equal(t, 30, svc.effective.ErrorLogRetentionDays)
	require.Equal(t, 14, svc.effective.MinuteMetricsRetentionDays)
	require.Equal(t, 90, svc.effective.HourlyMetricsRetentionDays)
	require.True(t, svc.effective.Enabled)
}
