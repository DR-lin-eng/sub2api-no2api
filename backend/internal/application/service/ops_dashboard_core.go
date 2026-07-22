package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"golang.org/x/sync/errgroup"
)

type opsDashboardCoreRepository interface {
	GetDashboardCoreSnapshot(
		ctx context.Context,
		filter *OpsDashboardFilter,
		bucketSeconds int,
		includeThroughputTrend bool,
		includeLatencyHistogram bool,
		includeErrorDistribution bool,
	) (*OpsDashboardCoreSnapshot, error)
}

// GetDashboardCoreSnapshot uses shared bucket sources for short raw/auto
// windows. Repositories without the optimized method retain the legacy path.
func (s *OpsService) GetDashboardCoreSnapshot(
	ctx context.Context,
	filter *OpsDashboardFilter,
	bucketSeconds int,
	includeThroughputTrend bool,
	includeLatencyHistogram bool,
	includeErrorTrend bool,
	includeErrorDistribution bool,
	includeSwitchCounts bool,
) (*OpsDashboardCoreSnapshot, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if filter == nil {
		return nil, infraerrors.BadRequest("OPS_FILTER_REQUIRED", "filter is required")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_REQUIRED", "start_time/end_time are required")
	}
	if filter.StartTime.After(filter.EndTime) {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_INVALID", "start_time must be <= end_time")
	}

	filter.QueryMode = s.resolveOpsQueryMode(ctx, filter.QueryMode)
	if repo, ok := s.opsRepo.(opsDashboardCoreRepository); ok &&
		filter.QueryMode != OpsQueryModePreagg &&
		!includeSwitchCounts &&
		filter.EndTime.Sub(filter.StartTime) <= 2*time.Hour {
		result, err := repo.GetDashboardCoreSnapshot(
			ctx,
			filter,
			bucketSeconds,
			includeThroughputTrend,
			includeLatencyHistogram,
			includeErrorDistribution,
		)
		if err != nil {
			return nil, err
		}
		if !includeThroughputTrend {
			result.ThroughputTrend = nil
		}
		if !includeErrorTrend {
			result.ErrorTrend = nil
		}
		s.enrichDashboardOverview(ctx, result.Overview)
		return result, nil
	}

	return s.getDashboardCoreSnapshotLegacy(
		ctx,
		filter,
		bucketSeconds,
		includeThroughputTrend,
		includeLatencyHistogram,
		includeErrorTrend,
		includeErrorDistribution,
		includeSwitchCounts,
	)
}

func (s *OpsService) getDashboardCoreSnapshotLegacy(
	ctx context.Context,
	filter *OpsDashboardFilter,
	bucketSeconds int,
	includeThroughputTrend bool,
	includeLatencyHistogram bool,
	includeErrorTrend bool,
	includeErrorDistribution bool,
	includeSwitchCounts bool,
) (*OpsDashboardCoreSnapshot, error) {
	result := &OpsDashboardCoreSnapshot{}
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		f := *filter
		overview, err := s.GetDashboardOverview(gctx, &f)
		result.Overview = overview
		return err
	})
	if includeThroughputTrend {
		g.Go(func() error {
			f := *filter
			f.ReuseErrorTrendCounts = includeErrorTrend
			f.ExcludeSwitchCounts = !includeSwitchCounts
			trend, err := s.GetThroughputTrend(gctx, &f, bucketSeconds)
			result.ThroughputTrend = trend
			return err
		})
	}
	if includeLatencyHistogram {
		g.Go(func() error {
			f := *filter
			histogram, err := s.GetLatencyHistogram(gctx, &f)
			result.LatencyHistogram = histogram
			return err
		})
	}
	if includeErrorTrend {
		g.Go(func() error {
			f := *filter
			trend, err := s.GetErrorTrend(gctx, &f, bucketSeconds)
			result.ErrorTrend = trend
			return err
		})
	}
	if includeErrorDistribution {
		g.Go(func() error {
			f := *filter
			distribution, err := s.GetErrorDistribution(gctx, &f)
			result.ErrorDistribution = distribution
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if includeThroughputTrend && includeErrorTrend {
		mergeOpsErrorTrendIntoThroughput(result.ThroughputTrend, result.ErrorTrend, bucketSeconds)
	}
	return result, nil
}

func mergeOpsErrorTrendIntoThroughput(
	throughput *OpsThroughputTrendResponse,
	errorTrend *OpsErrorTrendResponse,
	bucketSeconds int,
) {
	if throughput == nil || errorTrend == nil || bucketSeconds <= 0 {
		return
	}
	errorsByBucket := make(map[int64]int64, len(errorTrend.Points))
	for _, point := range errorTrend.Points {
		if point != nil {
			errorsByBucket[point.BucketStart.UTC().Unix()] = point.ErrorCountTotal
		}
	}
	for _, point := range throughput.Points {
		if point == nil {
			continue
		}
		point.RequestCount += errorsByBucket[point.BucketStart.UTC().Unix()]
		point.QPS = roundOpsRate(float64(point.RequestCount) / float64(bucketSeconds))
	}
}

func roundOpsRate(value float64) float64 {
	if value >= 0 {
		return float64(int64(value*10+0.5)) / 10
	}
	return float64(int64(value*10-0.5)) / 10
}
