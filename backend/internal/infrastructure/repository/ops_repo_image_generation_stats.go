package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

func (r *opsRepository) GetImageGenerationStats(ctx context.Context, filter *service.OpsDashboardFilter) (*service.OpsImageGenerationStatsResponse, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}

	start := filter.StartTime.UTC()
	end := filter.EndTime.UTC()
	join, where, args, _ := buildUsageWhere(filter, start, end, 1)
	where += " AND ul.image_count > 0 AND COALESCE(ul.video_count, 0) = 0"

	summarySQL := `
WITH image_usage AS (
  SELECT
    ul.created_at,
    ul.duration_ms,
    ul.image_count
  FROM usage_logs ul
  ` + join + `
  ` + where + `
),
events AS (
  SELECT GREATEST(created_at - duration_ms * INTERVAL '1 millisecond', $1) AS event_at, 1::bigint AS delta
  FROM image_usage
  WHERE duration_ms IS NOT NULL AND duration_ms > 0
  UNION ALL
  SELECT created_at AS event_at, -1::bigint AS delta
  FROM image_usage
  WHERE duration_ms IS NOT NULL AND duration_ms > 0
),
event_totals AS (
  SELECT event_at, SUM(delta) AS delta
  FROM events
  GROUP BY event_at
),
running AS (
  SELECT SUM(delta) OVER (ORDER BY event_at ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS active
  FROM event_totals
)
SELECT
  COUNT(*)::bigint AS request_count,
  COALESCE(SUM(image_count), 0)::bigint AS image_count,
  percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS p95_duration_ms,
  AVG(duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS avg_duration_ms,
  MAX(duration_ms) AS max_duration_ms,
  COALESCE(
    SUM(EXTRACT(EPOCH FROM (created_at - GREATEST(created_at - duration_ms * INTERVAL '1 millisecond', $1))))
      FILTER (WHERE duration_ms IS NOT NULL AND duration_ms > 0)
    / NULLIF(EXTRACT(EPOCH FROM ($2::timestamptz - $1::timestamptz)), 0),
    0
  ) AS average_concurrent,
  COALESCE((SELECT MAX(active) FROM running), 0)::bigint AS peak_concurrent
FROM image_usage`

	var requestCount, imageCount, peakConcurrent int64
	var p95, avg sql.NullFloat64
	var max sql.NullInt64
	var averageConcurrent float64
	if err := r.db.QueryRowContext(ctx, summarySQL, args...).Scan(
		&requestCount,
		&imageCount,
		&p95,
		&avg,
		&max,
		&averageConcurrent,
		&peakConcurrent,
	); err != nil {
		return nil, err
	}

	breakdownSQL := `
SELECT
  COALESCE(NULLIF(BTRIM(ul.image_output_size), ''), 'unknown') AS resolution,
  COALESCE(NULLIF(BTRIM(ul.image_size), ''), 'unknown') AS billing_tier,
  COUNT(*)::bigint AS request_count,
  COALESCE(SUM(ul.image_count), 0)::bigint AS image_count,
  AVG(ul.duration_ms) FILTER (WHERE ul.duration_ms IS NOT NULL) AS avg_duration_ms,
  percentile_cont(0.95) WITHIN GROUP (ORDER BY ul.duration_ms) FILTER (WHERE ul.duration_ms IS NOT NULL) AS p95_duration_ms,
  MAX(ul.duration_ms) AS max_duration_ms
FROM usage_logs ul
` + join + `
` + where + `
GROUP BY 1, 2
ORDER BY request_count DESC, image_count DESC, resolution ASC, billing_tier ASC`

	rows, err := r.db.QueryContext(ctx, breakdownSQL, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpsImageGenerationResolutionStats, 0, 8)
	for rows.Next() {
		item := &service.OpsImageGenerationResolutionStats{}
		var itemAvg, itemP95 sql.NullFloat64
		var itemMax sql.NullInt64
		if err := rows.Scan(
			&item.Resolution,
			&item.BillingTier,
			&item.RequestCount,
			&item.ImageCount,
			&itemAvg,
			&itemP95,
			&itemMax,
		); err != nil {
			return nil, err
		}
		item.AvgDurationMs = floatToIntPtr(itemAvg)
		item.P95DurationMs = floatToIntPtr(itemP95)
		if itemMax.Valid {
			v := int(itemMax.Int64)
			item.MaxDurationMs = &v
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	windowMinutes := end.Sub(start).Minutes()
	requestsPerMinute := 0.0
	if windowMinutes > 0 {
		requestsPerMinute = roundTo1DP(float64(requestCount) / windowMinutes)
	}

	result := &service.OpsImageGenerationStatsResponse{
		StartTime:         start,
		EndTime:           end,
		Platform:          strings.TrimSpace(strings.ToLower(filter.Platform)),
		GroupID:           filter.GroupID,
		RequestCount:      requestCount,
		ImageCount:        imageCount,
		RequestsPerMinute: requestsPerMinute,
		AvgDurationMs:     floatToIntPtr(avg),
		P95DurationMs:     floatToIntPtr(p95),
		AverageConcurrent: roundTo1DP(averageConcurrent),
		PeakConcurrent:    peakConcurrent,
		ByResolution:      items,
	}
	if max.Valid {
		v := int(max.Int64)
		result.MaxDurationMs = &v
	}
	return result, nil
}
