//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestGetAccountWindowStatsByStartBatchUsesOneQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	rows := sqlmock.NewRows([]string{"account_id", "requests", "tokens", "cost", "standard_cost", "user_cost"}).
		AddRow(int64(1), int64(2), int64(30), 0.7, 0.8, 0.6).
		AddRow(int64(2), int64(1), int64(10), 0.2, 0.3, 0.1)
	mock.ExpectQuery(`WITH requested AS`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(rows)

	repo := newUsageLogRepositoryWithSQL(nil, db)
	stats, err := repo.GetAccountWindowStatsByStartBatch(context.Background(), map[int64]time.Time{
		2: time.Now().Add(-2 * time.Hour),
		1: time.Now().Add(-time.Hour),
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), stats[1].Requests)
	require.Equal(t, 0.8, stats[1].StandardCost)
	require.Equal(t, int64(1), stats[2].Requests)
	require.NoError(t, mock.ExpectationsWereMet())
}
