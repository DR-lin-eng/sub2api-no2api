package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRepositoryGetAllGroupUsageSummaryUsesRollupTail(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := newUsageLogRepositoryWithSQL(nil, db)
	todayStart := service.GroupUsageTodayStart(time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC))
	yesterdayStart := service.GroupUsageYesterdayStart(todayStart)
	timezoneName := service.GroupUsageTimezoneName()
	yesterdayDate := service.GroupUsageDate(yesterdayStart)

	mock.ExpectQuery(`(?s)COUNT\(\*\).*usage_group_rollup_state.*WHERE id = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"count", "closed_before", "retained_from", "timezone_name"}).
			AddRow(1, "2026-03-07", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), timezoneName))
	tailStart, err := service.ParseGroupUsageDate("2026-03-07")
	require.NoError(t, err)
	mock.ExpectQuery(`(?s)usage_group_daily_rollups.*FROM usage_logs ul\s+WHERE ul\.created_at >= \$7`).
		WithArgs(todayStart, yesterdayStart, yesterdayDate, true, service.GroupUsageDate(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)), "2026-03-07", tailStart.UTC()).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "total_cost", "today_cost", "yesterday_cost"}).
			AddRow(int64(7), 12.5, 1.25, 2.5))

	result, err := repo.GetAllGroupUsageSummary(context.Background(), todayStart)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, int64(7), result[0].GroupID)
	require.InDelta(t, 12.5, result[0].TotalCost, 0.0000001)
	require.InDelta(t, 1.25, result[0].TodayCost, 0.0000001)
	require.InDelta(t, 2.5, result[0].YesterdayCost, 0.0000001)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageLogRepositoryGetAllGroupUsageSummaryFallsBackWhenWatermarkInvalid(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := newUsageLogRepositoryWithSQL(nil, db)
	todayStart := service.GroupUsageTodayStart(time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC))
	yesterdayStart := service.GroupUsageYesterdayStart(todayStart)
	yesterdayDate := service.GroupUsageDate(yesterdayStart)
	mock.ExpectQuery(`(?s)COUNT\(\*\).*usage_group_rollup_state.*WHERE id = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"count", "closed_before", "retained_from", "timezone_name"}).AddRow(0, nil, nil, nil))
	mock.ExpectQuery(`(?s)usage_group_daily_rollups.*FROM usage_logs ul\s+WHERE ul\.created_at >= \$7`).
		WithArgs(todayStart, yesterdayStart, yesterdayDate, false, "1970-01-01", "1970-01-01", time.Unix(0, 0).UTC()).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "total_cost", "today_cost", "yesterday_cost"}))

	result, err := repo.GetAllGroupUsageSummary(context.Background(), todayStart)
	require.NoError(t, err)
	require.Empty(t, result)
	require.NoError(t, mock.ExpectationsWereMet())
}
