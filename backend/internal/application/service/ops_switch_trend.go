package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

type opsSwitchTrendRepository interface {
	GetSwitchTrend(ctx context.Context, filter *OpsDashboardFilter, bucketSeconds int) (*OpsThroughputTrendResponse, error)
}

func (s *OpsService) GetSwitchTrend(ctx context.Context, filter *OpsDashboardFilter, bucketSeconds int) (*OpsThroughputTrendResponse, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if filter == nil || filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_REQUIRED", "start_time/end_time are required")
	}
	if filter.StartTime.After(filter.EndTime) {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_INVALID", "start_time must be <= end_time")
	}
	filter.QueryMode = s.resolveOpsQueryMode(ctx, filter.QueryMode)
	if repo, ok := s.opsRepo.(opsSwitchTrendRepository); ok {
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return repo.GetSwitchTrend(queryCtx, filter, bucketSeconds)
	}
	return s.GetThroughputTrend(ctx, filter, bucketSeconds)
}
