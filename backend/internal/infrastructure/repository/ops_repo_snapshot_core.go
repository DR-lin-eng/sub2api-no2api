package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"golang.org/x/sync/errgroup"
)

type opsUsageCoreResult struct {
	points         []*service.OpsThroughputTrendPoint
	histogram      *service.OpsLatencyHistogramResponse
	successCount   int64
	tokenConsumed  int64
	currentSuccess int64
	currentTokens  int64
}

type opsErrorCoreResult struct {
	trend                *service.OpsErrorTrendResponse
	distribution         *service.OpsErrorDistributionResponse
	errorCountTotal      int64
	businessLimitedCount int64
	errorCountSLA        int64
	upstreamExcl429529   int64
	upstream429          int64
	upstream529          int64
	currentErrors        int64
}

// GetDashboardCoreSnapshot scans each comparable source once: one usage bucket
// query and one error bucket query provide totals, current/peak rates and trends.
func (r *opsRepository) GetDashboardCoreSnapshot(
	ctx context.Context,
	filter *service.OpsDashboardFilter,
	bucketSeconds int,
	includeThroughputTrend bool,
	includeLatencyHistogram bool,
	includeErrorDistribution bool,
) (*service.OpsDashboardCoreSnapshot, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil || filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}
	if bucketSeconds != 60 && bucketSeconds != 300 && bucketSeconds != 3600 {
		bucketSeconds = 60
	}

	start := filter.StartTime.UTC()
	end := filter.EndTime.UTC()
	var usage *opsUsageCoreResult
	var errorsResult *opsErrorCoreResult
	var duration, ttft service.OpsPercentiles
	var byPlatform []*service.OpsThroughputPlatformBreakdownItem
	var topGroups []*service.OpsThroughputGroupBreakdownItem

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		result, err := r.queryOpsUsageCore(gctx, filter, start, end, bucketSeconds, includeLatencyHistogram)
		usage = result
		return err
	})
	g.Go(func() error {
		result, err := r.queryOpsErrorCore(gctx, filter, start, end, bucketSeconds, includeErrorDistribution)
		errorsResult = result
		return err
	})
	g.Go(func() error {
		latencyCtx, cancel := context.WithTimeout(gctx, opsRawLatencyQueryTimeout)
		defer cancel()
		var err error
		duration, ttft, _, err = r.queryUsageLatency(latencyCtx, filter, start, end)
		if isQueryTimeoutErr(err) {
			duration = service.OpsPercentiles{}
			ttft = service.OpsPercentiles{}
			return nil
		}
		return err
	})

	platform := strings.TrimSpace(strings.ToLower(filter.Platform))
	groupID := filter.GroupID
	if includeThroughputTrend && platform == "" && (groupID == nil || *groupID <= 0) {
		g.Go(func() error {
			items, err := r.getThroughputBreakdownByPlatform(gctx, start, end)
			byPlatform = items
			return err
		})
	} else if includeThroughputTrend && platform != "" && (groupID == nil || *groupID <= 0) {
		g.Go(func() error {
			items, err := r.getThroughputTopGroupsByPlatform(gctx, start, end, platform, 10)
			topGroups = items
			return err
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	if usage == nil || errorsResult == nil {
		return nil, fmt.Errorf("ops core aggregation returned no data")
	}

	mergeCoreErrorPoints(usage.points, errorsResult.trend.Points, bucketSeconds)
	var qpsPeak, tpsPeak float64
	for _, point := range usage.points {
		if point == nil {
			continue
		}
		if point.QPS > qpsPeak {
			qpsPeak = point.QPS
		}
		if point.TPS > tpsPeak {
			tpsPeak = point.TPS
		}
	}

	windowSeconds := end.Sub(start).Seconds()
	if windowSeconds <= 0 {
		windowSeconds = 1
	}
	requestCountTotal := usage.successCount + errorsResult.errorCountTotal
	requestCountSLA := usage.successCount + errorsResult.errorCountSLA
	overview := &service.OpsDashboardOverview{
		StartTime:                    start,
		EndTime:                      end,
		Platform:                     strings.TrimSpace(filter.Platform),
		GroupID:                      filter.GroupID,
		SuccessCount:                 usage.successCount,
		ErrorCountTotal:              errorsResult.errorCountTotal,
		BusinessLimitedCount:         errorsResult.businessLimitedCount,
		ErrorCountSLA:                errorsResult.errorCountSLA,
		RequestCountTotal:            requestCountTotal,
		RequestCountSLA:              requestCountSLA,
		TokenConsumed:                usage.tokenConsumed,
		SLA:                          roundTo4DP(safeDivideFloat64(float64(usage.successCount), float64(requestCountSLA))),
		ErrorRate:                    roundTo4DP(safeDivideFloat64(float64(errorsResult.errorCountSLA), float64(requestCountSLA))),
		UpstreamErrorRate:            roundTo4DP(safeDivideFloat64(float64(errorsResult.upstreamExcl429529), float64(requestCountSLA))),
		UpstreamErrorCountExcl429529: errorsResult.upstreamExcl429529,
		Upstream429Count:             errorsResult.upstream429,
		Upstream529Count:             errorsResult.upstream529,
		QPS: service.OpsRateSummary{
			Current: roundTo1DP(float64(usage.currentSuccess+errorsResult.currentErrors) / 60),
			Peak:    qpsPeak,
			Avg:     roundTo1DP(float64(requestCountTotal) / windowSeconds),
		},
		TPS: service.OpsRateSummary{
			Current: roundTo1DP(float64(usage.currentTokens) / 60),
			Peak:    tpsPeak,
			Avg:     roundTo1DP(float64(usage.tokenConsumed) / windowSeconds),
		},
		Duration: duration,
		TTFT:     ttft,
	}

	return &service.OpsDashboardCoreSnapshot{
		Overview: overview,
		ThroughputTrend: &service.OpsThroughputTrendResponse{
			Bucket:     opsBucketLabel(bucketSeconds),
			Points:     usage.points,
			ByPlatform: byPlatform,
			TopGroups:  topGroups,
		},
		LatencyHistogram:  usage.histogram,
		ErrorTrend:        errorsResult.trend,
		ErrorDistribution: errorsResult.distribution,
	}, nil
}

func (r *opsRepository) queryOpsUsageCore(
	ctx context.Context,
	filter *service.OpsDashboardFilter,
	start, end time.Time,
	bucketSeconds int,
	includeHistogram bool,
) (*opsUsageCoreResult, error) {
	join, where, args, _ := buildUsageWhere(filter, start, end, 1)
	bucketExpr := opsBucketExprForUsage(bucketSeconds)
	selectHistogram := ""
	if includeHistogram {
		selectHistogram = `,
	COUNT(*) FILTER (WHERE ul.duration_ms < 100),
	COUNT(*) FILTER (WHERE ul.duration_ms >= 100 AND ul.duration_ms < 200),
	COUNT(*) FILTER (WHERE ul.duration_ms >= 200 AND ul.duration_ms < 500),
	COUNT(*) FILTER (WHERE ul.duration_ms >= 500 AND ul.duration_ms < 1000),
	COUNT(*) FILTER (WHERE ul.duration_ms >= 1000 AND ul.duration_ms < 2000),
	COUNT(*) FILTER (WHERE ul.duration_ms >= 2000)`
	}
	q := `
SELECT
  ` + bucketExpr + ` AS bucket,
  COUNT(*) AS success_count,
  COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0) AS token_consumed,
  COUNT(*) FILTER (WHERE ul.created_at >= $2::timestamptz - INTERVAL '1 minute') AS current_success,
  COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens)
    FILTER (WHERE ul.created_at >= $2::timestamptz - INTERVAL '1 minute'), 0) AS current_tokens
  ` + selectHistogram + `
FROM usage_logs ul
` + join + `
` + where + `
GROUP BY 1
ORDER BY 1`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := &opsUsageCoreResult{points: make([]*service.OpsThroughputTrendPoint, 0, 128)}
	histogramCounts := make([]int64, len(latencyHistogramOrderedRanges))
	for rows.Next() {
		var bucket time.Time
		var success, tokens, currentSuccess, currentTokens int64
		if includeHistogram {
			rowHistogramCounts := make([]int64, len(latencyHistogramOrderedRanges))
			if err := rows.Scan(&bucket, &success, &tokens, &currentSuccess, &currentTokens,
				&rowHistogramCounts[0], &rowHistogramCounts[1], &rowHistogramCounts[2],
				&rowHistogramCounts[3], &rowHistogramCounts[4], &rowHistogramCounts[5]); err != nil {
				return nil, err
			}
			for index, count := range rowHistogramCounts {
				histogramCounts[index] += count
			}
		} else if err := rows.Scan(&bucket, &success, &tokens, &currentSuccess, &currentTokens); err != nil {
			return nil, err
		}
		result.successCount += success
		result.tokenConsumed += tokens
		result.currentSuccess += currentSuccess
		result.currentTokens += currentTokens
		result.points = append(result.points, &service.OpsThroughputTrendPoint{
			BucketStart:   bucket.UTC(),
			RequestCount:  success,
			TokenConsumed: tokens,
			QPS:           roundTo1DP(float64(success) / float64(bucketSeconds)),
			TPS:           roundTo1DP(float64(tokens) / float64(bucketSeconds)),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result.points = fillOpsThroughputBuckets(start, end, bucketSeconds, result.points)

	if includeHistogram {
		buckets := make([]*service.OpsLatencyHistogramBucket, 0, len(latencyHistogramOrderedRanges))
		var total int64
		for index, label := range latencyHistogramOrderedRanges {
			count := histogramCounts[index]
			total += count
			buckets = append(buckets, &service.OpsLatencyHistogramBucket{Range: label, Count: count})
		}
		result.histogram = &service.OpsLatencyHistogramResponse{
			StartTime:     start,
			EndTime:       end,
			Platform:      strings.TrimSpace(filter.Platform),
			GroupID:       filter.GroupID,
			TotalRequests: total,
			Buckets:       buckets,
		}
	}
	return result, nil
}

func (r *opsRepository) queryOpsErrorCore(
	ctx context.Context,
	filter *service.OpsDashboardFilter,
	start, end time.Time,
	bucketSeconds int,
	includeDistribution bool,
) (*opsErrorCoreResult, error) {
	where, args, _ := buildErrorWhere(filter, start, end, 1)
	bucketExpr := opsBucketExprForError(bucketSeconds)
	effectiveStatus := "COALESCE(upstream_status_code, status_code, 0)"
	groupingColumns := bucketExpr
	rowKind := "0"
	statusSelect := "NULL::integer"
	if includeDistribution {
		groupingColumns = "GROUPING SETS ((" + bucketExpr + "), (" + effectiveStatus + "))"
		rowKind = "GROUPING(" + bucketExpr + ")"
		statusSelect = "CASE WHEN GROUPING(" + effectiveStatus + ") = 0 THEN " + effectiveStatus + " END"
	}
	q := `
SELECT
  ` + rowKind + ` AS row_kind,
  ` + bucketExpr + ` AS bucket,
  ` + statusSelect + ` AS grouped_status,
  COUNT(*) FILTER (WHERE COALESCE(status_code, 0) >= 400) AS error_total,
  COUNT(*) FILTER (WHERE COALESCE(status_code, 0) >= 400 AND is_business_limited) AS business_limited,
  COUNT(*) FILTER (WHERE COALESCE(status_code, 0) >= 400 AND NOT is_business_limited) AS error_sla,
  COUNT(*) FILTER (WHERE error_owner = 'provider' AND NOT is_business_limited AND ` + effectiveStatus + ` NOT IN (429, 529)) AS upstream_excl,
  COUNT(*) FILTER (WHERE error_owner = 'provider' AND NOT is_business_limited AND ` + effectiveStatus + ` = 429) AS upstream_429,
  COUNT(*) FILTER (WHERE error_owner = 'provider' AND NOT is_business_limited AND ` + effectiveStatus + ` = 529) AS upstream_529,
  COUNT(*) FILTER (WHERE created_at >= $2::timestamptz - INTERVAL '1 minute' AND COALESCE(status_code, 0) >= 400) AS current_errors
FROM ops_error_logs
` + where + `
GROUP BY ` + groupingColumns + `
ORDER BY 1, 2, 3`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := &opsErrorCoreResult{
		trend: &service.OpsErrorTrendResponse{Bucket: opsBucketLabel(bucketSeconds), Points: make([]*service.OpsErrorTrendPoint, 0, 128)},
	}
	if includeDistribution {
		result.distribution = &service.OpsErrorDistributionResponse{Items: make([]*service.OpsErrorDistributionItem, 0, 16)}
	}
	for rows.Next() {
		var rowKind int
		var bucket sql.NullTime
		var status sql.NullInt64
		var total, business, sla, upstreamExcl, upstream429, upstream529, current int64
		if err := rows.Scan(&rowKind, &bucket, &status, &total, &business, &sla, &upstreamExcl, &upstream429, &upstream529, &current); err != nil {
			return nil, err
		}
		if rowKind == 0 {
			if !bucket.Valid {
				continue
			}
			result.errorCountTotal += total
			result.businessLimitedCount += business
			result.errorCountSLA += sla
			result.upstreamExcl429529 += upstreamExcl
			result.upstream429 += upstream429
			result.upstream529 += upstream529
			result.currentErrors += current
			result.trend.Points = append(result.trend.Points, &service.OpsErrorTrendPoint{
				BucketStart:                  bucket.Time.UTC(),
				ErrorCountTotal:              total,
				BusinessLimitedCount:         business,
				ErrorCountSLA:                sla,
				UpstreamErrorCountExcl429529: upstreamExcl,
				Upstream429Count:             upstream429,
				Upstream529Count:             upstream529,
			})
			continue
		}
		if result.distribution != nil && status.Valid && total > 0 {
			result.distribution.Items = append(result.distribution.Items, &service.OpsErrorDistributionItem{
				StatusCode:      int(status.Int64),
				Total:           total,
				SLA:             sla,
				BusinessLimited: business,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result.trend.Points = fillOpsErrorTrendBuckets(start, end, bucketSeconds, result.trend.Points)
	finalizeOpsErrorDistribution(result.distribution)
	return result, nil
}

func finalizeOpsErrorDistribution(distribution *service.OpsErrorDistributionResponse) {
	if distribution == nil {
		return
	}
	sort.Slice(distribution.Items, func(i, j int) bool {
		return distribution.Items[i].Total > distribution.Items[j].Total
	})
	if len(distribution.Items) > 20 {
		distribution.Items = distribution.Items[:20]
	}
	distribution.Total = 0
	for _, item := range distribution.Items {
		distribution.Total += item.Total
	}
}

func mergeCoreErrorPoints(
	throughput []*service.OpsThroughputTrendPoint,
	errorPoints []*service.OpsErrorTrendPoint,
	bucketSeconds int,
) {
	errorsByBucket := make(map[int64]int64, len(errorPoints))
	for _, point := range errorPoints {
		if point != nil {
			errorsByBucket[point.BucketStart.UTC().Unix()] = point.ErrorCountTotal
		}
	}
	for _, point := range throughput {
		if point == nil {
			continue
		}
		point.RequestCount += errorsByBucket[point.BucketStart.UTC().Unix()]
		point.QPS = roundTo1DP(float64(point.RequestCount) / float64(bucketSeconds))
	}
}
