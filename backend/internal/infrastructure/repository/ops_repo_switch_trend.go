package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

func (r *opsRepository) GetSwitchTrend(ctx context.Context, filter *service.OpsDashboardFilter, bucketSeconds int) (*service.OpsThroughputTrendResponse, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil || filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}
	if bucketSeconds != 60 && bucketSeconds != 300 && bucketSeconds != 3600 {
		bucketSeconds = 300
	}

	start := filter.StartTime.UTC()
	end := filter.EndTime.UTC()
	where, args, _ := buildErrorWhere(filter, start, end, 1)
	bucketExpr := opsBucketExprForError(bucketSeconds)
	q := `
SELECT
  ` + bucketExpr + ` AS bucket,
  COALESCE(SUM(CASE
    WHEN split_part(ev->>'kind', ':', 1) IN ('failover', 'retry_exhausted_failover', 'failover_on_400') THEN 1
    ELSE 0
  END), 0) AS switch_count
FROM ops_error_logs
CROSS JOIN LATERAL jsonb_array_elements(
  COALESCE(NULLIF(upstream_errors, 'null'::jsonb), '[]'::jsonb)
) AS ev
` + where + `
  AND upstream_errors IS NOT NULL
GROUP BY 1
ORDER BY 1`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	points := make([]*service.OpsThroughputTrendPoint, 0, 128)
	for rows.Next() {
		var bucket time.Time
		var switches int64
		if err := rows.Scan(&bucket, &switches); err != nil {
			return nil, err
		}
		points = append(points, &service.OpsThroughputTrendPoint{
			BucketStart: bucket.UTC(),
			SwitchCount: switches,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &service.OpsThroughputTrendResponse{
		Bucket: opsBucketLabel(bucketSeconds),
		Points: fillOpsThroughputBuckets(start, end, bucketSeconds, points),
	}, nil
}
