package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

type OpsNetworkBandwidthTrendPoint struct {
	BucketStart            time.Time `json:"bucket_start"`
	ReceiveBytesPerSecond  *float64  `json:"receive_bytes_per_second"`
	TransmitBytesPerSecond *float64  `json:"transmit_bytes_per_second"`
}

type OpsNetworkBandwidthTrendResponse struct {
	StartTime time.Time                        `json:"start_time"`
	EndTime   time.Time                        `json:"end_time"`
	Bucket    string                           `json:"bucket"`
	Points    []*OpsNetworkBandwidthTrendPoint `json:"points"`
}

type opsNetworkBandwidthRepository interface {
	GetNetworkBandwidthTrend(
		ctx context.Context,
		startTime time.Time,
		endTime time.Time,
		bucketSeconds int,
	) (*OpsNetworkBandwidthTrendResponse, error)
}

func (s *OpsService) GetNetworkBandwidthTrend(
	ctx context.Context,
	startTime time.Time,
	endTime time.Time,
	bucketSeconds int,
) (*OpsNetworkBandwidthTrendResponse, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if startTime.IsZero() || endTime.IsZero() {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_REQUIRED", "start_time/end_time are required")
	}
	if startTime.After(endTime) {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_INVALID", "start_time must be <= end_time")
	}
	repo, ok := s.opsRepo.(opsNetworkBandwidthRepository)
	if !ok {
		return nil, infraerrors.ServiceUnavailable("OPS_NETWORK_BANDWIDTH_UNAVAILABLE", "Network bandwidth metrics are not available")
	}
	return repo.GetNetworkBandwidthTrend(ctx, startTime.UTC(), endTime.UTC(), bucketSeconds)
}
