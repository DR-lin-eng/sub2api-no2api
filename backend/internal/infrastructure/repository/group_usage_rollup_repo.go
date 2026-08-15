package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

const (
	groupUsageInsertBarrierKeyHigh int32 = 1398096434 // SUB2
	groupUsageInsertBarrierKeyLow  int32 = 1196576853 // GRPU
)

// SyncGroupUsageRollups publishes every completed local calendar day. A short
// advisory barrier first drains transactions that mutated the former current
// day across midnight. The expensive aggregation runs after that barrier is
// released, so current-day gateway writes are not blocked by a backfill.
func (r *dashboardAggregationRepository) SyncGroupUsageRollups(ctx context.Context, todayStart time.Time) error {
	if r == nil || r.sql == nil {
		return nil
	}
	todayStart = service.GroupUsageTodayStart(todayStart)

	needsSync, err := r.groupUsageRollupsNeedSync(ctx, todayStart)
	if err != nil || !needsSync {
		return err
	}

	if db, ok := r.sql.(*sql.DB); ok {
		if err := waitForGroupUsageInsertBarrier(ctx, db); err != nil {
			return fmt.Errorf("wait for group usage insert barrier: %w", err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		txRepo := newDashboardAggregationRepositoryWithSQL(tx)
		txRepo.clock = r.clock
		if err := txRepo.syncGroupUsageRollupsInTx(ctx, todayStart); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	return r.syncGroupUsageRollupsInTx(ctx, todayStart)
}

func (r *dashboardAggregationRepository) groupUsageRollupsNeedSync(ctx context.Context, todayStart time.Time) (bool, error) {
	var closedBefore string
	var timezoneName string
	if err := scanSingleRow(ctx, r.sql, `
		SELECT closed_before::text, timezone_name
		FROM usage_group_rollup_state
		WHERE id = 1
	`, nil, &closedBefore, &timezoneName); err != nil {
		return false, fmt.Errorf("read group usage rollup state: %w", err)
	}
	todayDate := service.GroupUsageDate(todayStart)
	if timezoneName != service.GroupUsageTimezoneName() {
		return true, nil
	}
	closedTime, err := service.ParseGroupUsageDate(closedBefore)
	if err != nil {
		return false, fmt.Errorf("parse group usage rollup watermark %q: %w", closedBefore, err)
	}
	todayTime, err := service.ParseGroupUsageDate(todayDate)
	if err != nil {
		return false, err
	}
	if closedTime.After(todayTime) {
		return false, fmt.Errorf("group usage rollup watermark is in the future: %s", closedBefore)
	}
	return closedBefore != todayDate, nil
}

func waitForGroupUsageInsertBarrier(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		"SELECT pg_advisory_xact_lock($1, $2)",
		groupUsageInsertBarrierKeyHigh,
		groupUsageInsertBarrierKeyLow,
	); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (r *dashboardAggregationRepository) syncGroupUsageRollupsInTx(ctx context.Context, todayStart time.Time) error {
	var closedBefore string
	var previousRetainedFrom time.Time
	var stateTimezoneName string
	if err := scanSingleRow(ctx, r.sql, `
		SELECT closed_before::text, retained_from, timezone_name
		FROM usage_group_rollup_state
		WHERE id = 1
		FOR UPDATE
	`, nil, &closedBefore, &previousRetainedFrom, &stateTimezoneName); err != nil {
		return fmt.Errorf("read group usage rollup watermark: %w", err)
	}

	todayDate := service.GroupUsageDate(todayStart)
	timezoneName := service.GroupUsageTimezoneName()
	timezoneChanged := stateTimezoneName != timezoneName
	var closedTime time.Time
	if !timezoneChanged {
		var err error
		closedTime, err = service.ParseGroupUsageDate(closedBefore)
		if err != nil {
			return fmt.Errorf("parse group usage rollup watermark %q: %w", closedBefore, err)
		}
		todayDateTime, err := service.ParseGroupUsageDate(todayDate)
		if err != nil {
			return err
		}
		if closedTime.After(todayDateTime) {
			return fmt.Errorf("group usage rollup watermark is in the future: %s", closedBefore)
		}
		if closedBefore == todayDate {
			return nil
		}
	}

	var earliest sql.NullTime
	if err := scanSingleRow(ctx, r.sql, "SELECT MIN(created_at) FROM usage_logs", nil, &earliest); err != nil {
		return fmt.Errorf("read earliest usage log: %w", err)
	}
	retainedFrom := todayStart
	if earliest.Valid {
		retainedFrom = earliest.Time.UTC()
	}
	retainedDate := service.GroupUsageDate(retainedFrom)
	retainedDateTime, err := service.ParseGroupUsageDate(retainedDate)
	if err != nil {
		return err
	}
	rebuildStartDate := retainedDate
	if !timezoneChanged && closedTime.After(retainedDateTime) {
		rebuildStartDate = closedBefore
	}
	rebuildStart, err := service.ParseGroupUsageDate(rebuildStartDate)
	if err != nil {
		return err
	}

	if _, err := r.sql.ExecContext(ctx, `
		DELETE FROM usage_group_daily_rollups
		WHERE bucket_date < $1::date
			OR bucket_date >= $2::date
	`, retainedDate, rebuildStartDate); err != nil {
		return fmt.Errorf("clean group usage daily rollups: %w", err)
	}

	if _, err := r.sql.ExecContext(ctx, `
		INSERT INTO usage_group_daily_rollups (bucket_date, group_id, actual_cost, computed_at)
		SELECT
			(created_at AT TIME ZONE $3::text)::date AS bucket_date,
			group_id,
			COALESCE(SUM(actual_cost), 0) AS actual_cost,
			NOW()
		FROM usage_logs
		WHERE group_id IS NOT NULL
			AND created_at >= $1
			AND created_at < $2
		GROUP BY 1, 2
		ON CONFLICT (bucket_date, group_id)
		DO UPDATE SET
			actual_cost = EXCLUDED.actual_cost,
			computed_at = EXCLUDED.computed_at
	`, rebuildStart.UTC(), todayStart.UTC(), timezoneName); err != nil {
		return fmt.Errorf("rebuild group usage daily rollups: %w", err)
	}

	if _, err := r.sql.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET closed_before = $1::date,
			retained_from = $2,
			timezone_name = $3,
			updated_at = NOW()
		WHERE id = 1
	`, todayDate, retainedFrom, timezoneName); err != nil {
		return fmt.Errorf("update group usage rollup watermark: %w", err)
	}
	return nil
}

func lockGroupUsageRollupState(ctx context.Context, tx *sql.Tx) error {
	var id int16
	if err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM usage_group_rollup_state
		WHERE id = 1
		FOR UPDATE
	`).Scan(&id); err != nil {
		return fmt.Errorf("lock group usage rollup watermark: %w", err)
	}
	return nil
}

func invalidateGroupUsageRollupsAt(ctx context.Context, tx *sql.Tx, affectedAt time.Time) error {
	timezoneName := service.GroupUsageTimezoneName()
	_, err := tx.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET closed_before = LEAST(
			closed_before,
			($1::timestamptz AT TIME ZONE $2::text)::date
		),
			updated_at = NOW()
		WHERE id = 1
	`, affectedAt.UTC(), timezoneName)
	return err
}
