package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type channelMonitorBatchRepoStub struct {
	ChannelMonitorRepository
	latestCalls       int
	availabilityCalls map[int]int
}

func (s *channelMonitorBatchRepoStub) ListEnabled(context.Context) ([]*ChannelMonitor, error) {
	return []*ChannelMonitor{{
		ID: 1, Name: "primary", Provider: MonitorProviderOpenAI,
		PrimaryModel: "gpt-5", ExtraModels: []string{"gpt-5-mini"}, Enabled: true,
	}}, nil
}

func (s *channelMonitorBatchRepoStub) ListLatestForMonitorIDs(_ context.Context, ids []int64) (map[int64][]*ChannelMonitorLatest, error) {
	s.latestCalls++
	latency := 125
	return map[int64][]*ChannelMonitorLatest{
		1: {{Model: "gpt-5", Status: MonitorStatusOperational, LatencyMs: &latency}},
	}, nil
}

func (s *channelMonitorBatchRepoStub) ComputeAvailabilityForMonitors(_ context.Context, ids []int64, windowDays int) (map[int64][]*ChannelMonitorAvailability, error) {
	if s.availabilityCalls == nil {
		s.availabilityCalls = make(map[int]int)
	}
	s.availabilityCalls[windowDays]++
	avgLatency := 100 + windowDays
	return map[int64][]*ChannelMonitorAvailability{
		1: {{Model: "gpt-5", WindowDays: windowDays, AvailabilityPct: float64(90 + windowDays), AvgLatencyMs: &avgLatency}},
	}, nil
}

func TestGetUserDetailsUsesFixedBatchQueries(t *testing.T) {
	repo := &channelMonitorBatchRepoStub{}
	svc := &ChannelMonitorService{repo: repo}

	details, err := svc.GetUserDetails(context.Background(), []int64{1})
	require.NoError(t, err)
	require.Len(t, details, 1)
	require.Equal(t, 1, repo.latestCalls)
	require.Equal(t, map[int]int{7: 1, 15: 1, 30: 1}, repo.availabilityCalls)
	require.Equal(t, 97.0, details[0].Models[0].Availability7d)
	require.Equal(t, 105.0, details[0].Models[0].Availability15d)
	require.Equal(t, 120.0, details[0].Models[0].Availability30d)
}
