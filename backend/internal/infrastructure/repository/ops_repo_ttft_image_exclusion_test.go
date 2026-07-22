package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestQueryUsageTTFTExcludesImageRows(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	mock.ExpectQuery(`(?s)FROM usage_logs ul.*first_token_ms IS NOT NULL.*COALESCE\(image_count, 0\) = 0`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"ttft_p50",
			"ttft_p90",
			"ttft_p95",
			"ttft_p99",
			"ttft_avg",
			"ttft_max",
		}).AddRow(100.0, 200.0, 250.0, 300.0, 150.0, int64(350)))

	result, err := repo.queryUsageTTFT(context.Background(), &service.OpsDashboardFilter{}, start, end)
	require.NoError(t, err)
	require.NotNil(t, result.P50)
	require.Equal(t, 100, *result.P50)
	require.NotNil(t, result.P99)
	require.Equal(t, 300, *result.P99)
	require.NotNil(t, result.Max)
	require.Equal(t, 350, *result.Max)
	require.NoError(t, mock.ExpectationsWereMet())
}
