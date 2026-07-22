package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestGetBatchAPIKeyUsageStatsIncludesTokensAndEmptyKeys(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)SELECT.*SUM\(input_tokens \+ output_tokens \+ cache_creation_tokens \+ cache_read_tokens\).*created_at < \$3`).
		WithArgs(sqlmock.AnyArg(), start, end, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"api_key_id", "total_cost", "today_cost", "total_tokens"}).
			AddRow(int64(11), 3.25, 0.5, int64(12345)))

	stats, err := newUsageLogRepositoryWithSQL(nil, db).GetBatchAPIKeyUsageStats(context.Background(), []int64{11, 12}, start, end)
	require.NoError(t, err)
	require.Equal(t, int64(12345), stats[11].TotalTokens)
	require.Equal(t, 3.25, stats[11].TotalActualCost)
	require.Zero(t, stats[12].TotalTokens)
	require.Zero(t, stats[12].TotalActualCost)
	require.NoError(t, mock.ExpectationsWereMet())
}
