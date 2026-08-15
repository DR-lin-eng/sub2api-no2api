package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestDashboardAggregationRepositorySyncGroupUsageRollupsUsesShortInsertBarrier(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := service.GroupUsageTodayStart(time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC))
	retainedFrom := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	timezoneName := service.GroupUsageTimezoneName()
	retainedDate := service.GroupUsageDate(retainedFrom)
	rebuildStart, err := service.ParseGroupUsageDate(retainedDate)
	require.NoError(t, err)
	todayDate := service.GroupUsageDate(todayStart)

	mock.ExpectQuery(`SELECT closed_before::text, timezone_name`).
		WillReturnRows(sqlmock.NewRows([]string{"closed_before", "timezone_name"}).
			AddRow("1970-01-01", timezoneName))
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).
		WithArgs(groupUsageInsertBarrierKeyHigh, groupUsageInsertBarrierKeyLow).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT closed_before::text, retained_from, timezone_name.*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"closed_before", "retained_from", "timezone_name"}).
			AddRow("1970-01-01", time.Unix(0, 0).UTC(), timezoneName))
	mock.ExpectQuery(`SELECT MIN\(created_at\) FROM usage_logs`).
		WillReturnRows(sqlmock.NewRows([]string{"min"}).AddRow(retainedFrom))
	mock.ExpectExec(`DELETE FROM usage_group_daily_rollups`).
		WithArgs(retainedDate, retainedDate).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO usage_group_daily_rollups`).
		WithArgs(rebuildStart.UTC(), todayStart, timezoneName).
		WillReturnResult(sqlmock.NewResult(0, 12))
	mock.ExpectExec(`UPDATE usage_group_rollup_state`).
		WithArgs(todayDate, retainedFrom, timezoneName).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, repo.SyncGroupUsageRollups(context.Background(), todayStart))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardAggregationRepositorySyncGroupUsageRollupsSkipsBarrierWhenCurrent(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := newDashboardAggregationRepositoryWithSQL(db)
	todayStart := service.GroupUsageTodayStart(time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC))
	timezoneName := service.GroupUsageTimezoneName()
	todayDate := service.GroupUsageDate(todayStart)

	mock.ExpectQuery(`SELECT closed_before::text, timezone_name`).
		WillReturnRows(sqlmock.NewRows([]string{"closed_before", "timezone_name"}).
			AddRow(todayDate, timezoneName))

	require.NoError(t, repo.SyncGroupUsageRollups(context.Background(), todayStart))
	require.NoError(t, mock.ExpectationsWereMet())
}
