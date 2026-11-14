package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type routingSchemeAnalyticsRepoStub struct {
	scheme *RoutingScheme
}

func (s *routingSchemeAnalyticsRepoStub) List(context.Context, string, bool) ([]RoutingScheme, error) {
	return nil, nil
}

func (s *routingSchemeAnalyticsRepoStub) GetByID(context.Context, int64) (*RoutingScheme, error) {
	return s.scheme, nil
}

func (s *routingSchemeAnalyticsRepoStub) Create(context.Context, *RoutingScheme) error { return nil }
func (s *routingSchemeAnalyticsRepoStub) Update(context.Context, *RoutingScheme) error { return nil }
func (s *routingSchemeAnalyticsRepoStub) Delete(context.Context, int64) error          { return nil }
func (s *routingSchemeAnalyticsRepoStub) ListSchemeIDsByGroupID(context.Context, int64) ([]int64, error) {
	return nil, nil
}

type routingSchemeAnalyticsMetricsStub struct {
	usage map[int64]RoutingSchemeGroupUsage
}

func (s *routingSchemeAnalyticsMetricsStub) GetGroupMetrics(context.Context, []int64, time.Time) (map[int64]RoutingSchemeGroupMetric, error) {
	return nil, nil
}

func (s *routingSchemeAnalyticsMetricsStub) GetSchemeGroupUsage(context.Context, int64, time.Time) (map[int64]RoutingSchemeGroupUsage, error) {
	return s.usage, nil
}

func TestAnalyticsSuccessRateUsesRequestsAsDenominator(t *testing.T) {
	repo := &routingSchemeAnalyticsRepoStub{scheme: &RoutingScheme{
		ID: 1,
		Items: []RoutingSchemeItem{
			{GroupID: 10},
			{GroupID: 20},
		},
	}}
	metrics := &routingSchemeAnalyticsMetricsStub{usage: map[int64]RoutingSchemeGroupUsage{
		// Three provider failures happened across eight successfully billed
		// requests; the success rate must stay 8/10, not 8/(8+3).
		10: {Requests: 10, Successes: 8, Failures: 3, Cost: 1},
		20: {Requests: 5, Successes: 5, Cost: 3},
	}}
	svc := NewRoutingSchemeService(repo, nil, metrics)

	got, err := svc.Analytics(context.Background(), 1, "24h")
	require.NoError(t, err)
	require.Len(t, got.Groups, 2)

	byGroup := make(map[int64]RoutingSchemeAnalyticsGroup, len(got.Groups))
	for _, group := range got.Groups {
		byGroup[group.GroupID] = group
	}

	require.InDelta(t, 80, byGroup[10].SuccessRate, 0.001)
	require.InDelta(t, 100, byGroup[20].SuccessRate, 0.001)
	require.InDelta(t, 66.666, byGroup[10].RequestShare, 0.01)
	require.InDelta(t, 25, byGroup[10].CostShare, 0.01)
	require.Equal(t, int64(3), byGroup[10].Failures)
}
