package service

import (
	"context"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

type OpsImageGenerationResolutionStats struct {
	Resolution    string `json:"resolution"`
	BillingTier   string `json:"billing_tier"`
	RequestCount  int64  `json:"request_count"`
	ImageCount    int64  `json:"image_count"`
	AvgDurationMs *int   `json:"avg_duration_ms"`
	P95DurationMs *int   `json:"p95_duration_ms"`
	MaxDurationMs *int   `json:"max_duration_ms"`
}

type OpsImageGenerationRealtime struct {
	Available         bool   `json:"available"`
	Scope             string `json:"scope"`
	Enabled           bool   `json:"enabled"`
	CurrentConcurrent int    `json:"current_concurrent"`
	Waiting           int    `json:"waiting"`
	Limit             int    `json:"limit"`
	MaxWaiting        int    `json:"max_waiting"`
}

type OpsImageGenerationStatsResponse struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Platform  string    `json:"platform"`
	GroupID   *int64    `json:"group_id"`

	RequestCount      int64   `json:"request_count"`
	ImageCount        int64   `json:"image_count"`
	RequestsPerMinute float64 `json:"requests_per_minute"`
	AvgDurationMs     *int    `json:"avg_duration_ms"`
	P95DurationMs     *int    `json:"p95_duration_ms"`
	MaxDurationMs     *int    `json:"max_duration_ms"`
	AverageConcurrent float64 `json:"average_concurrent"`
	PeakConcurrent    int64   `json:"peak_concurrent"`

	Realtime     OpsImageGenerationRealtime           `json:"realtime"`
	ByResolution []*OpsImageGenerationResolutionStats `json:"by_resolution"`
}

type opsImageGenerationStatsRepository interface {
	GetImageGenerationStats(ctx context.Context, filter *OpsDashboardFilter) (*OpsImageGenerationStatsResponse, error)
}

func (s *OpsService) GetImageGenerationStats(ctx context.Context, filter *OpsDashboardFilter) (*OpsImageGenerationStatsResponse, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s == nil || s.opsRepo == nil {
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
	if filter.GroupID != nil && *filter.GroupID <= 0 {
		return nil, infraerrors.BadRequest("OPS_GROUP_ID_INVALID", "group_id must be > 0")
	}

	repo, ok := s.opsRepo.(opsImageGenerationStatsRepository)
	if !ok {
		return nil, infraerrors.ServiceUnavailable("OPS_IMAGE_STATS_UNAVAILABLE", "Image generation statistics are not available")
	}
	filter.Platform = strings.TrimSpace(strings.ToLower(filter.Platform))
	result, err := repo.GetImageGenerationStats(ctx, filter)
	if err != nil {
		return nil, err
	}

	realtime := OpsImageGenerationRealtime{Scope: "instance"}
	if s.cfg != nil {
		cfg := s.cfg.Gateway.ImageConcurrency
		realtime.Enabled = cfg.Enabled && cfg.MaxConcurrentRequests > 0
		realtime.Limit = cfg.MaxConcurrentRequests
		realtime.MaxWaiting = cfg.MaxWaitingRequests
	}
	if s.imageConcurrencySnapshot != nil {
		realtime.Available = true
		realtime.CurrentConcurrent, realtime.Waiting = s.imageConcurrencySnapshot()
	}
	result.Realtime = realtime
	return result, nil
}
