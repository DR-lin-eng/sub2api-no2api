package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

func (r *opsRepository) GetNetworkBandwidthTrend(
	ctx context.Context,
	startTime time.Time,
	endTime time.Time,
	bucketSeconds int,
) (*service.OpsNetworkBandwidthTrendResponse, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if startTime.IsZero() || endTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}
	if bucketSeconds != 60 && bucketSeconds != 300 && bucketSeconds != 3600 {
		bucketSeconds = 60
	}

	start := startTime.UTC()
	end := endTime.UTC()
	bucketExpr := opsBucketExprForError(bucketSeconds)
	query := `
SELECT
  ` + bucketExpr + ` AS bucket,
  AVG(network_receive_bytes_per_second) AS receive_bytes_per_second,
  AVG(network_transmit_bytes_per_second) AS transmit_bytes_per_second
FROM ops_system_metrics
WHERE created_at >= $1 AND created_at < $2
  AND window_minutes = 1
  AND platform IS NULL
  AND group_id IS NULL
  AND (
    network_receive_bytes_per_second IS NOT NULL
    OR network_transmit_bytes_per_second IS NOT NULL
  )
GROUP BY 1
ORDER BY 1`

	rows, err := r.db.QueryContext(ctx, query, start, end)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	points := make([]*service.OpsNetworkBandwidthTrendPoint, 0, 128)
	for rows.Next() {
		var point service.OpsNetworkBandwidthTrendPoint
		var receive sql.NullFloat64
		var transmit sql.NullFloat64
		if err := rows.Scan(&point.BucketStart, &receive, &transmit); err != nil {
			return nil, err
		}
		point.BucketStart = point.BucketStart.UTC()
		if receive.Valid {
			value := receive.Float64
			point.ReceiveBytesPerSecond = &value
		}
		if transmit.Valid {
			value := transmit.Float64
			point.TransmitBytesPerSecond = &value
		}
		points = append(points, &point)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &service.OpsNetworkBandwidthTrendResponse{
		StartTime: start,
		EndTime:   end,
		Bucket:    opsBucketLabel(bucketSeconds),
		Points:    fillOpsNetworkBandwidthBuckets(start, end, bucketSeconds, points),
	}, nil
}

func fillOpsNetworkBandwidthBuckets(
	start time.Time,
	end time.Time,
	bucketSeconds int,
	points []*service.OpsNetworkBandwidthTrendPoint,
) []*service.OpsNetworkBandwidthTrendPoint {
	if bucketSeconds <= 0 {
		bucketSeconds = 60
	}
	if !start.Before(end) {
		return points
	}

	endMinus := end.Add(-time.Nanosecond)
	if endMinus.Before(start) {
		return points
	}
	first := opsFloorToBucketStart(start, bucketSeconds)
	last := opsFloorToBucketStart(endMinus, bucketSeconds)
	step := time.Duration(bucketSeconds) * time.Second

	existing := make(map[int64]*service.OpsNetworkBandwidthTrendPoint, len(points))
	for _, point := range points {
		if point != nil {
			existing[point.BucketStart.UTC().Unix()] = point
		}
	}
	result := make([]*service.OpsNetworkBandwidthTrendPoint, 0, int(last.Sub(first)/step)+1)
	for cursor := first; !cursor.After(last); cursor = cursor.Add(step) {
		if point, ok := existing[cursor.Unix()]; ok {
			result = append(result, point)
			continue
		}
		result = append(result, &service.OpsNetworkBandwidthTrendPoint{BucketStart: cursor})
	}
	return result
}
