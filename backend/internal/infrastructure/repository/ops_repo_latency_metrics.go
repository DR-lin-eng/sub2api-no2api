package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

// Output rates share the existing raw latency scan. No extra query, sampling,
// hourly-percentile averaging, goroutine or unbounded cache is introduced.
var opsOutputTPSRate = fmt.Sprintf(`CASE WHEN ul.output_tokens > 0 AND ul.duration_ms > 0
  AND COALESCE(ul.image_count, 0) = 0 AND COALESCE(ul.image_output_tokens, 0) = 0
  AND COALESCE(ul.billing_mode, '') NOT IN ('image', 'live')
  AND ul.request_type IN (%d, %d, %d, %d)
  THEN ul.output_tokens * 1000.0 / NULLIF(ul.duration_ms, 0) END`,
	service.RequestTypeSync, service.RequestTypeStream, service.RequestTypeWSV2, service.RequestTypeCyberBlocked)

var opsOutputTPSColumns = fmt.Sprintf(`
  percentile_cont(0.05) WITHIN GROUP (ORDER BY %[1]s),
  percentile_cont(0.10) WITHIN GROUP (ORDER BY %[1]s),
  percentile_cont(0.50) WITHIN GROUP (ORDER BY %[1]s),
  AVG(%[1]s), COUNT(%[1]s)`, opsOutputTPSRate)

func (r *opsRepository) queryUsageLatencyMetrics(ctx context.Context, filter *service.OpsDashboardFilter, start, end time.Time, includeOutputTPS bool) (duration service.OpsPercentiles, ttft service.OpsPercentiles, ttftSampleCount int64, outputTPS *service.OpsOutputTPS, err error) {
	join, where, args, _ := buildUsageWhere(filter, start, end, 1)
	q := `
SELECT
  percentile_cont(0.50) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS duration_p50,
  percentile_cont(0.90) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS duration_p90,
  percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS duration_p95,
  percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS duration_p99,
  AVG(duration_ms) FILTER (WHERE duration_ms IS NOT NULL) AS duration_avg,
  MAX(duration_ms) AS duration_max,
  percentile_cont(0.50) WITHIN GROUP (ORDER BY first_token_ms) FILTER (WHERE first_token_ms IS NOT NULL AND COALESCE(image_count, 0) = 0) AS ttft_p50,
  percentile_cont(0.90) WITHIN GROUP (ORDER BY first_token_ms) FILTER (WHERE first_token_ms IS NOT NULL AND COALESCE(image_count, 0) = 0) AS ttft_p90,
  percentile_cont(0.95) WITHIN GROUP (ORDER BY first_token_ms) FILTER (WHERE first_token_ms IS NOT NULL AND COALESCE(image_count, 0) = 0) AS ttft_p95,
  percentile_cont(0.99) WITHIN GROUP (ORDER BY first_token_ms) FILTER (WHERE first_token_ms IS NOT NULL AND COALESCE(image_count, 0) = 0) AS ttft_p99,
  AVG(first_token_ms) FILTER (WHERE first_token_ms IS NOT NULL AND COALESCE(image_count, 0) = 0) AS ttft_avg,
  MAX(first_token_ms) FILTER (WHERE COALESCE(image_count, 0) = 0) AS ttft_max,
  COUNT(first_token_ms) FILTER (WHERE COALESCE(image_count, 0) = 0) AS ttft_sample_count
FROM usage_logs ul
` + join + `
` + where

	if includeOutputTPS {
		q = strings.Replace(q, "\nFROM usage_logs ul\n", ",\n"+opsOutputTPSColumns+"\nFROM usage_logs ul\n", 1)
	}
	var dP50, dP90, dP95, dP99 sql.NullFloat64
	var dAvg sql.NullFloat64
	var dMax sql.NullInt64
	var tP50, tP90, tP95, tP99 sql.NullFloat64
	var tAvg sql.NullFloat64
	var tMax sql.NullInt64
	var tCount int64
	var stats service.OpsOutputTPS
	destinations := []any{&dP50, &dP90, &dP95, &dP99, &dAvg, &dMax, &tP50, &tP90, &tP95, &tP99, &tAvg, &tMax, &tCount}
	if includeOutputTPS {
		destinations = append(destinations, &stats.P5, &stats.P10, &stats.P50, &stats.Avg, &stats.SampleCount)
	}
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(destinations...); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		return service.OpsPercentiles{}, service.OpsPercentiles{}, 0, nil, err
	}
	if includeOutputTPS {
		outputTPS = &stats
	}

	duration.P50 = floatToIntPtr(dP50)
	duration.P90 = floatToIntPtr(dP90)
	duration.P95 = floatToIntPtr(dP95)
	duration.P99 = floatToIntPtr(dP99)
	duration.Avg = floatToIntPtr(dAvg)
	if dMax.Valid {
		v := int(dMax.Int64)
		duration.Max = &v
	}

	ttft.P50 = floatToIntPtr(tP50)
	ttft.P90 = floatToIntPtr(tP90)
	ttft.P95 = floatToIntPtr(tP95)
	ttft.P99 = floatToIntPtr(tP99)
	ttft.Avg = floatToIntPtr(tAvg)
	if tMax.Valid {
		v := int(tMax.Int64)
		ttft.Max = &v
	}

	return duration, ttft, tCount, outputTPS, nil
}

func (r *opsRepository) queryUsageTTFTAndOutputTPS(ctx context.Context, filter *service.OpsDashboardFilter, start, end time.Time) (service.OpsPercentiles, *service.OpsOutputTPS, error) {
	join, where, args, _ := buildUsageWhere(filter, start, end, 1)
	q := `SELECT
percentile_cont(0.50) WITHIN GROUP (ORDER BY CASE WHEN first_token_ms IS NOT NULL AND COALESCE(image_count, 0) = 0 THEN first_token_ms END),
percentile_cont(0.90) WITHIN GROUP (ORDER BY CASE WHEN first_token_ms IS NOT NULL AND COALESCE(image_count, 0) = 0 THEN first_token_ms END),
percentile_cont(0.95) WITHIN GROUP (ORDER BY CASE WHEN first_token_ms IS NOT NULL AND COALESCE(image_count, 0) = 0 THEN first_token_ms END),
percentile_cont(0.99) WITHIN GROUP (ORDER BY CASE WHEN first_token_ms IS NOT NULL AND COALESCE(image_count, 0) = 0 THEN first_token_ms END),
AVG(CASE WHEN first_token_ms IS NOT NULL AND COALESCE(image_count, 0) = 0 THEN first_token_ms END),
MAX(CASE WHEN first_token_ms IS NOT NULL AND COALESCE(image_count, 0) = 0 THEN first_token_ms END),
` + opsOutputTPSColumns + `
FROM usage_logs ul
` + join + `
` + where
	var p50, p90, p95, p99, avg sql.NullFloat64
	var max sql.NullInt64
	var stats service.OpsOutputTPS
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(&p50, &p90, &p95, &p99, &avg, &max, &stats.P5, &stats.P10, &stats.P50, &stats.Avg, &stats.SampleCount); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		return service.OpsPercentiles{}, nil, err
	}
	out := service.OpsPercentiles{P50: floatToIntPtr(p50), P90: floatToIntPtr(p90), P95: floatToIntPtr(p95), P99: floatToIntPtr(p99), Avg: floatToIntPtr(avg)}
	if max.Valid {
		v := int(max.Int64)
		out.Max = &v
	}
	return out, &stats, nil
}
