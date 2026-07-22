package service

import (
	"context"
	"testing"
	"time"
)

type opsSwitchTrendRepoProbe struct {
	*opsRepoMock
	deadlineRemaining time.Duration
}

func (p *opsSwitchTrendRepoProbe) GetSwitchTrend(
	ctx context.Context,
	_ *OpsDashboardFilter,
	_ int,
) (*OpsThroughputTrendResponse, error) {
	if deadline, ok := ctx.Deadline(); ok {
		p.deadlineRemaining = time.Until(deadline)
	}
	return &OpsThroughputTrendResponse{}, nil
}

func TestGetSwitchTrendBoundsSlowQuery(t *testing.T) {
	repo := &opsSwitchTrendRepoProbe{opsRepoMock: &opsRepoMock{}}
	svc := &OpsService{opsRepo: repo}
	start := time.Now().UTC().Add(-5 * time.Hour)
	_, err := svc.GetSwitchTrend(context.Background(), &OpsDashboardFilter{
		StartTime: start,
		EndTime:   start.Add(5 * time.Hour),
		QueryMode: OpsQueryModeRaw,
	}, 300)
	if err != nil {
		t.Fatalf("GetSwitchTrend() error = %v", err)
	}
	if repo.deadlineRemaining <= 4*time.Second || repo.deadlineRemaining > 5*time.Second {
		t.Fatalf("query deadline remaining = %s, want bounded near 5s", repo.deadlineRemaining)
	}
}
