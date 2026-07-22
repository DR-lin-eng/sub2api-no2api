package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type opsImageGenerationStatsRepoStub struct {
	OpsRepository
	result *OpsImageGenerationStatsResponse
}

func (r *opsImageGenerationStatsRepoStub) GetImageGenerationStats(context.Context, *OpsDashboardFilter) (*OpsImageGenerationStatsResponse, error) {
	return r.result, nil
}

func TestOpsServiceGetImageGenerationStatsAddsRealtimeLimiterSnapshot(t *testing.T) {
	start := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	repo := &opsImageGenerationStatsRepoStub{result: &OpsImageGenerationStatsResponse{ByResolution: []*OpsImageGenerationResolutionStats{}}}
	svc := NewOpsService(repo, nil, &config.Config{
		Ops: config.OpsConfig{Enabled: true},
		Gateway: config.GatewayConfig{
			ImageConcurrency: config.ImageConcurrencyConfig{
				Enabled:               true,
				MaxConcurrentRequests: 8,
				MaxWaitingRequests:    20,
			},
		},
	}, nil, nil, nil, nil, nil, nil, nil, nil)
	svc.SetImageConcurrencySnapshotProvider(func() (int, int) { return 5, 2 })

	result, err := svc.GetImageGenerationStats(context.Background(), &OpsDashboardFilter{
		StartTime: start,
		EndTime:   start.Add(time.Hour),
	})
	require.NoError(t, err)
	require.True(t, result.Realtime.Available)
	require.True(t, result.Realtime.Enabled)
	require.Equal(t, "instance", result.Realtime.Scope)
	require.Equal(t, 5, result.Realtime.CurrentConcurrent)
	require.Equal(t, 2, result.Realtime.Waiting)
	require.Equal(t, 8, result.Realtime.Limit)
	require.Equal(t, 20, result.Realtime.MaxWaiting)
}
