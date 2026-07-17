//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestGetAccountHourlyUsageStatsBatchUsesOneQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	rows := sqlmock.NewRows([]string{
		"account_id", "successful_requests", "avg_first_token_ms", "error_total", "error_4xx", "error_5xx",
	}).AddRow(int64(11), int64(8), 250.5, int64(2), int64(1), int64(1))
	mock.ExpectQuery(`WITH requested AS`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(rows)

	repo := newUsageLogRepositoryWithSQL(nil, db)
	endTime := time.Now().UTC()
	stats, err := repo.GetAccountHourlyUsageStatsBatch(
		context.Background(),
		[]int64{11, 22},
		endTime.Add(-time.Hour),
		endTime,
	)

	require.NoError(t, err)
	require.Equal(t, int64(10), stats[11].TotalRequests)
	require.Equal(t, int64(8), stats[11].SuccessfulRequests)
	require.InDelta(t, 0.8, stats[11].SuccessRate, 1e-9)
	require.NotNil(t, stats[11].AvgFirstTokenMs)
	require.InDelta(t, 250.5, *stats[11].AvgFirstTokenMs, 1e-9)
	require.Equal(t, int64(1), stats[11].Error4xx)
	require.Equal(t, int64(1), stats[11].Error5xx)
	require.NotNil(t, stats[22])
	require.Zero(t, stats[22].TotalRequests)
	require.NoError(t, mock.ExpectationsWereMet())
}
