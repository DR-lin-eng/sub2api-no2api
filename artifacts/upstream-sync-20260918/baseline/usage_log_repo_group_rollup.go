package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/usagestats"
)

func (r *usageLogRepository) getAllGroupUsageSummaryFromRollups(ctx context.Context, todayStart time.Time) (results []usagestats.GroupUsageSummary, err error) {
	todayStart = service.GroupUsageTodayStart(todayStart)
	yesterdayStart := service.GroupUsageYesterdayStart(todayStart)
	timezoneName := service.GroupUsageTimezoneName()
	todayDate := service.GroupUsageDate(todayStart)
	yesterdayDate := service.GroupUsageDate(yesterdayStart)

	const query = `
		WITH state_values AS (
			SELECT
				COUNT(*) = 1
					AND MAX(timezone_name) = $3
					AND MAX(closed_before) <= $4::date AS valid,
				MAX(closed_before) AS closed_before,
				MAX(retained_from) AS retained_from
			FROM usage_group_rollup_state
			WHERE id = 1
		),
		state AS (
			SELECT
				CASE WHEN valid THEN closed_before ELSE DATE '1970-01-01' END AS closed_before,
				CASE WHEN valid THEN retained_from ELSE TIMESTAMPTZ '1970-01-01 00:00:00+00' END AS retained_from,
				CASE
					WHEN valid THEN closed_before::timestamp AT TIME ZONE $3::text
					ELSE TIMESTAMPTZ '1970-01-01 00:00:00+00'
				END AS tail_start,
				valid
			FROM state_values
		),
		historical AS (
			SELECT
				rollup.group_id,
				COALESCE(SUM(rollup.actual_cost), 0) AS actual_cost,
				COALESCE(SUM(rollup.actual_cost) FILTER (
					WHERE rollup.bucket_date = $5::date
				), 0) AS yesterday_cost
			FROM usage_group_daily_rollups rollup
			CROSS JOIN state
			WHERE state.valid
				AND rollup.bucket_date >= (state.retained_from AT TIME ZONE $3::text)::date
				AND rollup.bucket_date < state.closed_before
			GROUP BY rollup.group_id
		),
		tail AS (
			SELECT
				ul.group_id,
				COALESCE(SUM(ul.actual_cost), 0) AS actual_cost,
				COALESCE(SUM(ul.actual_cost) FILTER (WHERE ul.created_at >= $1), 0) AS today_cost,
				COALESCE(SUM(ul.actual_cost) FILTER (
					WHERE ul.created_at >= $2
						AND ul.created_at < $1
				), 0) AS yesterday_cost
			FROM usage_logs ul
			CROSS JOIN state
			WHERE ul.created_at >= state.tail_start
				AND ul.group_id IS NOT NULL
			GROUP BY ul.group_id
		)
		SELECT
			g.id AS group_id,
			COALESCE(historical.actual_cost, 0) + COALESCE(tail.actual_cost, 0) AS total_cost,
			COALESCE(tail.today_cost, 0) AS today_cost,
			COALESCE(historical.yesterday_cost, 0) + COALESCE(tail.yesterday_cost, 0) AS yesterday_cost
		FROM groups g
		LEFT JOIN historical ON historical.group_id = g.id
		LEFT JOIN tail ON tail.group_id = g.id
		ORDER BY g.id
	`

	rows, err := r.sql.QueryContext(
		ctx,
		query,
		todayStart,
		yesterdayStart,
		timezoneName,
		todayDate,
		yesterdayDate,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results = make([]usagestats.GroupUsageSummary, 0)
	for rows.Next() {
		var row usagestats.GroupUsageSummary
		if err := rows.Scan(&row.GroupID, &row.TotalCost, &row.TodayCost, &row.YesterdayCost); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}
