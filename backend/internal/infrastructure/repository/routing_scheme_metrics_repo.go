package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"

	"github.com/lib/pq"
)

type routingSchemeMetricsRepository struct {
	db *sql.DB
}

// NewRoutingSchemeMetricsRepository reads the rolling per-group signals used by
// routing scheme scoring. Successful traffic comes from usage_logs; provider
// failures come from ops_error_logs so user-side and billing errors never drag
// a healthy group down.
func NewRoutingSchemeMetricsRepository(db *sql.DB) service.RoutingSchemeMetricsRepository {
	return &routingSchemeMetricsRepository{db: db}
}

func (r *routingSchemeMetricsRepository) GetGroupMetrics(ctx context.Context, groupIDs []int64, since time.Time) (map[int64]service.RoutingSchemeGroupMetric, error) {
	out := make(map[int64]service.RoutingSchemeGroupMetric, len(groupIDs))
	if r == nil || r.db == nil || len(groupIDs) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(groupIDs))
	for _, id := range groupIDs {
		if id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}

	const q = `
		WITH success_agg AS (
		    SELECT ul.group_id,
		           COUNT(*) FILTER (WHERE ul.actual_cost > 0) AS success_count,
		           percentile_cont(0.95) WITHIN GROUP (ORDER BY ul.first_token_ms) FILTER (
		               WHERE ul.first_token_ms IS NOT NULL
		                 AND COALESCE(ul.image_count, 0) = 0
		                 AND COALESCE(ul.video_count, 0) = 0
		           ) AS p95_ttft_ms
		    FROM usage_logs ul
		    WHERE ul.group_id = ANY($1)
		      AND ul.created_at >= $2
		    GROUP BY ul.group_id
		),
		error_agg AS (
		    SELECT e.group_id,
		           COUNT(*) AS failure_count,
		           COUNT(*) FILTER (WHERE e.status_code = 429) AS http_429,
		           COUNT(*) FILTER (WHERE e.status_code >= 500) AS http_5xx,
		           COUNT(*) FILTER (WHERE e.status_code IN (401, 403)) AS auth_error,
		           COUNT(*) FILTER (WHERE e.status_code IN (408, 504)) AS timeout_error,
		           COUNT(*) FILTER (WHERE e.status_code = 404) AS model_unavailable,
		           MAX(e.created_at) AS last_error_at
		    FROM ops_error_logs e
		    WHERE e.group_id = ANY($1)
		      AND e.created_at >= $2
		      AND COALESCE(e.status_code, 0) >= 400
		      -- Only provider-owned failures say anything about group health.
		      -- A client sending an unknown model name, a bad payload or an
		      -- invalid key is recorded as client/platform and must never park a
		      -- healthy group. 400/422 are request-shape errors even when the
		      -- upstream echoes them back.
		      AND COALESCE(e.error_owner, '') = 'provider'
		      AND COALESCE(e.status_code, 0) NOT IN (400, 422)
		      AND e.is_business_limited = FALSE
		      AND e.is_count_tokens = FALSE
		    GROUP BY e.group_id
		)
		SELECT g.group_id,
		       COALESCE(s.success_count, 0),
		       COALESCE(e.failure_count, 0),
		       s.p95_ttft_ms,
		       COALESCE(e.http_429, 0),
		       COALESCE(e.http_5xx, 0),
		       COALESCE(e.auth_error, 0),
		       COALESCE(e.timeout_error, 0),
		       COALESCE(e.model_unavailable, 0),
		       e.last_error_at
		FROM UNNEST($1::bigint[]) AS g(group_id)
		LEFT JOIN success_agg s ON s.group_id = g.group_id
		LEFT JOIN error_agg e ON e.group_id = g.group_id
	`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(ids), since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var metric service.RoutingSchemeGroupMetric
		var p95 sql.NullFloat64
		var http429, http5xx, authError, timeoutError, modelUnavailable int64
		var lastErrorAt sql.NullTime
		if err := rows.Scan(
			&metric.GroupID,
			&metric.SuccessCount,
			&metric.FailureCount,
			&p95,
			&http429,
			&http5xx,
			&authError,
			&timeoutError,
			&modelUnavailable,
			&lastErrorAt,
		); err != nil {
			return nil, err
		}
		if p95.Valid {
			metric.P95TTFTMs = p95.Float64
		}
		if lastErrorAt.Valid {
			at := lastErrorAt.Time
			metric.LastErrorAt = &at
		}
		metric.ErrorClassCounts = map[string]int64{
			service.RoutingErrorClassHTTP429:          http429,
			service.RoutingErrorClassHTTP5xx:          http5xx,
			service.RoutingErrorClassAuth:             authError,
			service.RoutingErrorClassTimeout:          timeoutError,
			service.RoutingErrorClassModelUnavailable: modelUnavailable,
		}
		out[metric.GroupID] = metric
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// GetSchemeGroupUsage aggregates traffic attributed to a scheme by the keys
// that opted into it. Group ownership alone is not enough: a group can be used
// by manual keys and scheme keys at the same time.
func (r *routingSchemeMetricsRepository) GetSchemeGroupUsage(ctx context.Context, schemeID int64, since time.Time) (map[int64]service.RoutingSchemeGroupUsage, error) {
	out := make(map[int64]service.RoutingSchemeGroupUsage)
	if r == nil || r.db == nil || schemeID <= 0 {
		return out, nil
	}

	const q = `
		WITH scheme_keys AS (
		    SELECT id FROM api_keys WHERE routing_scheme_id = $1
		),
		usage_agg AS (
		    SELECT ul.group_id,
		           COUNT(*) AS requests,
		           COUNT(*) FILTER (WHERE ul.actual_cost > 0) AS successes,
		           percentile_cont(0.95) WITHIN GROUP (ORDER BY ul.first_token_ms) FILTER (
		               WHERE ul.first_token_ms IS NOT NULL
		                 AND COALESCE(ul.image_count, 0) = 0
		                 AND COALESCE(ul.video_count, 0) = 0
		           ) AS p95_ttft_ms,
		           COALESCE(SUM(ul.input_tokens), 0) AS input_tokens,
		           COALESCE(SUM(ul.output_tokens), 0) AS output_tokens,
		           COALESCE(SUM(ul.actual_cost), 0) AS cost
		    FROM usage_logs ul
		    WHERE ul.api_key_id IN (SELECT id FROM scheme_keys)
		      AND ul.created_at >= $2
		      AND ul.group_id IS NOT NULL
		    GROUP BY ul.group_id
		),
		error_agg AS (
		    SELECT e.group_id, COUNT(*) AS failures
		    FROM ops_error_logs e
		    WHERE e.api_key_id IN (SELECT id FROM scheme_keys)
		      AND e.created_at >= $2
		      AND COALESCE(e.status_code, 0) >= 400
		      AND COALESCE(e.error_owner, '') = 'provider'
		      AND COALESCE(e.status_code, 0) NOT IN (400, 422)
		      AND e.is_business_limited = FALSE
		      AND e.is_count_tokens = FALSE
		      AND e.group_id IS NOT NULL
		    GROUP BY e.group_id
		)
		SELECT COALESCE(u.group_id, e.group_id) AS group_id,
		       COALESCE(u.requests, 0),
		       COALESCE(u.successes, 0),
		       COALESCE(e.failures, 0),
		       COALESCE(u.p95_ttft_ms, 0),
		       COALESCE(u.input_tokens, 0),
		       COALESCE(u.output_tokens, 0),
		       COALESCE(u.cost, 0)
		FROM usage_agg u
		FULL OUTER JOIN error_agg e ON e.group_id = u.group_id
	`
	rows, err := r.db.QueryContext(ctx, q, schemeID, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var groupID int64
		var usage service.RoutingSchemeGroupUsage
		if err := rows.Scan(
			&groupID,
			&usage.Requests,
			&usage.Successes,
			&usage.Failures,
			&usage.P95TTFTMs,
			&usage.InputTokens,
			&usage.OutputTokens,
			&usage.Cost,
		); err != nil {
			return nil, err
		}
		out[groupID] = usage
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
