package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryGetNetworkBandwidthTrendFillsMissingBuckets(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	end := start.Add(3 * time.Minute)

	mock.ExpectQuery(`(?s)AVG\(network_receive_bytes_per_second\).*FROM ops_system_metrics.*platform IS NULL.*group_id IS NULL`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"bucket",
			"receive_bytes_per_second",
			"transmit_bytes_per_second",
		}).
			AddRow(start, 1024.0, 512.0).
			AddRow(start.Add(2*time.Minute), 4096.0, 2048.0))

	result, err := repo.GetNetworkBandwidthTrend(context.Background(), start, end, 60)
	require.NoError(t, err)
	require.Equal(t, "1m", result.Bucket)
	require.Len(t, result.Points, 3)
	require.Equal(t, 1024.0, *result.Points[0].ReceiveBytesPerSecond)
	require.Nil(t, result.Points[1].ReceiveBytesPerSecond)
	require.Nil(t, result.Points[1].TransmitBytesPerSecond)
	require.Equal(t, 2048.0, *result.Points[2].TransmitBytesPerSecond)
	require.NoError(t, mock.ExpectationsWereMet())
}
