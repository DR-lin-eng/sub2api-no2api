//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestGroupUsageRollupCurrentInsertDoesNotWaitForStateRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	schema := createGroupUsageRollupTestSchema(t, ctx, false)
	seedGroupUsageRollupTestRows(t, ctx, schema)

	syncTx := beginGroupUsageRollupTestTx(t, ctx, schema)
	defer func() { _ = syncTx.Rollback() }()
	var stateID int16
	require.NoError(t, syncTx.QueryRowContext(ctx, `
		SELECT id FROM usage_group_rollup_state WHERE id = 1 FOR UPDATE
	`).Scan(&stateID))

	insertTx := beginGroupUsageRollupTestTx(t, ctx, schema)
	defer func() { _ = insertTx.Rollback() }()
	insertResult := make(chan error, 1)
	go func() {
		_, err := insertTx.ExecContext(ctx, `
			INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
			VALUES (1, 1, 10, 1.25, clock_timestamp())
		`)
		insertResult <- err
	}()

	select {
	case err := <-insertResult:
		require.NoError(t, err)
	case <-time.After(500 * time.Millisecond):
		_ = syncTx.Rollback()
		t.Fatal("current-day usage insert waited for the rollup state row")
	}
}

func TestGroupUsageRollupInsertBarrierWaitsForCurrentTransaction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	schema := createGroupUsageRollupTestSchema(t, ctx, false)
	seedGroupUsageRollupTestRows(t, ctx, schema)

	insertTx := beginGroupUsageRollupTestTx(t, ctx, schema)
	_, err := insertTx.ExecContext(ctx, `
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
		VALUES (1, 1, 10, 1.25, clock_timestamp())
	`)
	require.NoError(t, err)

	barrierResult := make(chan error, 1)
	go func() {
		barrierResult <- waitForGroupUsageInsertBarrier(ctx, integrationDB)
	}()

	select {
	case err := <-barrierResult:
		_ = insertTx.Rollback()
		require.NoError(t, err)
		t.Fatal("rollup insert barrier passed before the writer transaction completed")
	case <-time.After(150 * time.Millisecond):
	}

	require.NoError(t, insertTx.Commit())
	select {
	case err := <-barrierResult:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("rollup insert barrier did not resume after the writer committed")
	}
}

func TestGroupUsageRollupHistoricalCascadeInvalidatesOrdinaryAndPartitionedTables(t *testing.T) {
	for _, partitioned := range []bool{false, true} {
		name := "ordinary"
		if partitioned {
			name = "partitioned"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			schema := createGroupUsageRollupTestSchema(t, ctx, partitioned)
			tx := beginGroupUsageRollupTestTx(t, ctx, schema)
			defer func() { _ = tx.Rollback() }()

			_, err := tx.ExecContext(ctx, `
				INSERT INTO groups (id) VALUES (10);
				INSERT INTO users (id) VALUES (1);
				INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at)
				VALUES (1, 1, 10, 1.25, TIMESTAMPTZ '2020-01-02 08:00:00+08');
				UPDATE usage_group_rollup_state
				SET closed_before = (clock_timestamp() AT TIME ZONE current_setting('TimeZone'))::date
				WHERE id = 1;
				DELETE FROM users WHERE id = 1;
			`)
			require.NoError(t, err)

			var closedBefore string
			require.NoError(t, tx.QueryRowContext(ctx, `
				SELECT closed_before::text FROM usage_group_rollup_state WHERE id = 1
			`).Scan(&closedBefore))
			require.Equal(t, "2020-01-02", closedBefore)
		})
	}
}

func TestGroupUsageRollupSyncAndSummaryUseClosedBucketsPlusRawTail(t *testing.T) {
	ctx := context.Background()
	schema := createGroupUsageRollupTestSchema(t, ctx, false)
	tx := beginGroupUsageRollupTestTx(t, ctx, schema)
	defer func() { _ = tx.Rollback() }()

	_, err := tx.ExecContext(ctx, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		INSERT INTO usage_logs (id, user_id, group_id, actual_cost, created_at) VALUES
			(1, 1, 10, 2, TIMESTAMPTZ '2026-08-12 12:00:00+00'),
			(2, 1, 10, 3, TIMESTAMPTZ '2026-08-13 12:00:00+00'),
			(3, 1, 10, 4, TIMESTAMPTZ '2026-08-14 12:00:00+00');
	`)
	require.NoError(t, err)

	todayStart := service.GroupUsageTodayStart(time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC))
	aggregationRepo := newDashboardAggregationRepositoryWithSQL(tx)
	require.NoError(t, aggregationRepo.SyncGroupUsageRollups(ctx, todayStart))

	usageRepo := newUsageLogRepositoryWithSQL(nil, tx)
	result, err := usageRepo.GetAllGroupUsageSummary(ctx, todayStart)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, int64(10), result[0].GroupID)
	require.InDelta(t, 9, result[0].TotalCost, 0.0000001)
	require.InDelta(t, 4, result[0].TodayCost, 0.0000001)
	require.InDelta(t, 3, result[0].YesterdayCost, 0.0000001)
}

func createGroupUsageRollupTestSchema(t *testing.T, ctx context.Context, partitioned bool) string {
	t.Helper()
	schema := fmt.Sprintf("group_usage_rollup_%d", time.Now().UnixNano())
	quotedSchema := pq.QuoteIdentifier(schema)
	_, err := integrationDB.ExecContext(ctx, "CREATE SCHEMA "+quotedSchema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+quotedSchema+" CASCADE")
	})

	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, setGroupUsageRollupSearchPath(ctx, tx, quotedSchema))

	usageLogsDDL := `
		CREATE TABLE usage_logs (
			id BIGINT PRIMARY KEY,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL,
			actual_cost NUMERIC(20, 10) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL
		);
	`
	if partitioned {
		usageLogsDDL = `
			CREATE TABLE usage_logs (
				id BIGINT NOT NULL,
				user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL,
				actual_cost NUMERIC(20, 10) NOT NULL,
				created_at TIMESTAMPTZ NOT NULL
			) PARTITION BY RANGE (created_at);
			CREATE TABLE usage_logs_default PARTITION OF usage_logs DEFAULT;
		`
	}
	_, err = tx.ExecContext(ctx, `
		CREATE TABLE users (id BIGINT PRIMARY KEY);
		CREATE TABLE groups (id BIGINT PRIMARY KEY);
	`+usageLogsDDL)
	require.NoError(t, err)

	for _, migrationName := range []string{
		"222_group_usage_daily_rollups.sql",
		"223_group_usage_rollup_timezone.sql",
	} {
		migrationSQL, readErr := dbmigrations.FS.ReadFile(migrationName)
		require.NoError(t, readErr)
		_, err = tx.ExecContext(ctx, string(migrationSQL))
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())
	return schema
}

func seedGroupUsageRollupTestRows(t *testing.T, ctx context.Context, schema string) {
	t.Helper()
	tx := beginGroupUsageRollupTestTx(t, ctx, schema)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO groups (id) VALUES (10);
		INSERT INTO users (id) VALUES (1);
		UPDATE usage_group_rollup_state
		SET closed_before = (clock_timestamp() AT TIME ZONE current_setting('TimeZone'))::date
		WHERE id = 1;
	`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

func beginGroupUsageRollupTestTx(t *testing.T, ctx context.Context, schema string) *sql.Tx {
	t.Helper()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, setGroupUsageRollupSearchPath(ctx, tx, pq.QuoteIdentifier(schema)))
	return tx
}

func setGroupUsageRollupSearchPath(ctx context.Context, tx *sql.Tx, quotedSchema string) error {
	_, err := tx.ExecContext(ctx, "SET LOCAL search_path TO "+quotedSchema)
	return err
}
