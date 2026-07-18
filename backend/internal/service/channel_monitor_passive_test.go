//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type passiveChannelMonitorRepoStub struct {
	ChannelMonitorRepository
	monitor         *ChannelMonitor
	samples         []*ChannelMonitorPassiveSample
	channelID       int64
	provider        string
	models          []string
	window          time.Duration
	history         []*ChannelMonitorHistoryRow
	markedCheckedAt time.Time
	created         *ChannelMonitor
}

func (r *passiveChannelMonitorRepoStub) Create(_ context.Context, monitor *ChannelMonitor) error {
	copy := *monitor
	copy.ChannelID = cloneInt64Pointer(monitor.ChannelID)
	r.created = &copy
	monitor.ID = 101
	return nil
}

func (r *passiveChannelMonitorRepoStub) GetByID(context.Context, int64) (*ChannelMonitor, error) {
	copy := *r.monitor
	copy.ExtraModels = append([]string(nil), r.monitor.ExtraModels...)
	copy.ChannelID = cloneInt64Pointer(r.monitor.ChannelID)
	return &copy, nil
}

func (r *passiveChannelMonitorRepoStub) ComputePassiveSamples(
	_ context.Context,
	channelID int64,
	provider string,
	models []string,
	startTime, endTime time.Time,
) ([]*ChannelMonitorPassiveSample, error) {
	r.channelID = channelID
	r.provider = provider
	r.models = append([]string(nil), models...)
	r.window = endTime.Sub(startTime)
	return r.samples, nil
}

func (r *passiveChannelMonitorRepoStub) InsertHistoryBatch(_ context.Context, rows []*ChannelMonitorHistoryRow) error {
	r.history = rows
	return nil
}

func (r *passiveChannelMonitorRepoStub) MarkChecked(_ context.Context, _ int64, checkedAt time.Time) error {
	r.markedCheckedAt = checkedAt
	return nil
}

func TestChannelMonitorRunCheckPassiveUsesRealRequestSamples(t *testing.T) {
	channelID := int64(7)
	slowLatency := 7001
	repo := &passiveChannelMonitorRepoStub{
		monitor: &ChannelMonitor{
			ID:              42,
			MonitorMode:     MonitorModePassive,
			ChannelID:       &channelID,
			Provider:        MonitorProviderOpenAI,
			PrimaryModel:    "gpt-5.4",
			ExtraModels:     []string{"gpt-5.4-mini", "gpt-5.3", "gpt-5.2", "gpt-5.1"},
			IntervalSeconds: 60,
		},
		samples: []*ChannelMonitorPassiveSample{
			{Model: "gpt-5.4", SuccessCount: 99, FailureCount: 1},
			{Model: "gpt-5.4-mini", SuccessCount: 9, FailureCount: 1},
			{Model: "gpt-5.3", SuccessCount: 8, FailureCount: 2},
			{Model: "gpt-5.2", SuccessCount: 10, AvgLatencyMs: &slowLatency},
			{Model: "gpt-5.1"},
		},
	}
	svc := NewChannelMonitorService(repo, nil)

	results, err := svc.RunCheck(context.Background(), 42)

	require.NoError(t, err)
	require.Equal(t, int64(7), repo.channelID)
	require.Equal(t, MonitorProviderOpenAI, repo.provider)
	require.Equal(t, []string{"gpt-5.4", "gpt-5.4-mini", "gpt-5.3", "gpt-5.2", "gpt-5.1"}, repo.models)
	require.Equal(t, time.Minute, repo.window)
	require.Len(t, results, 5)
	require.Equal(t, MonitorStatusOperational, results[0].Status)
	require.Equal(t, MonitorStatusDegraded, results[1].Status)
	require.Equal(t, MonitorStatusFailed, results[2].Status)
	require.Equal(t, MonitorStatusDegraded, results[3].Status)
	require.Equal(t, MonitorStatusUnknown, results[4].Status)
	require.Contains(t, results[0].Message, "requests=100")
	require.Contains(t, results[0].Message, "success_rate=99.00%")
	require.Equal(t, results[3].LatencyMs, &slowLatency)
	require.Len(t, repo.history, 5)
	require.Equal(t, MonitorStatusUnknown, repo.history[4].Status)
	require.False(t, repo.markedCheckedAt.IsZero())
}

func TestChannelMonitorPassiveCreateRequiresChannelButNotProbeCredentials(t *testing.T) {
	repo := &passiveChannelMonitorRepoStub{}
	svc := NewChannelMonitorService(repo, nil)
	params := ChannelMonitorCreateParams{
		Name:             "passive",
		Provider:         MonitorProviderOpenAI,
		MonitorMode:      MonitorModePassive,
		PrimaryModel:     "gpt-5.4",
		IntervalSeconds:  60,
		BodyOverrideMode: MonitorBodyOverrideModeOff,
	}

	_, err := svc.Create(context.Background(), params)

	require.ErrorIs(t, err, ErrChannelMonitorMissingChannel)

	channelID := int64(7)
	params.ChannelID = &channelID
	created, err := svc.Create(context.Background(), params)
	require.NoError(t, err)
	require.Equal(t, int64(101), created.ID)
	require.Equal(t, MonitorModePassive, repo.created.MonitorMode)
	require.Equal(t, int64(7), *repo.created.ChannelID)
	require.Empty(t, repo.created.Endpoint)
	require.Empty(t, repo.created.APIKey)
}
