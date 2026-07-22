package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

func TestGetSwitchTrendUsesOnlyErrorEventSource(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	start := time.Date(2026, 7, 22, 8, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Hour)

	mock.ExpectQuery(`(?s)FROM ops_error_logs\s+CROSS JOIN LATERAL jsonb_array_elements.*upstream_errors IS NOT NULL`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"bucket", "switch_count"}).
			AddRow(start, int64(9)))

	result, err := repo.GetSwitchTrend(context.Background(), &service.OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
	}, 300)
	if err != nil {
		t.Fatalf("GetSwitchTrend() error = %v", err)
	}
	if result.Bucket != "5m" || len(result.Points) != 60 {
		t.Fatalf("unexpected switch trend shape: bucket=%q points=%d", result.Bucket, len(result.Points))
	}
	if result.Points[0].SwitchCount != 9 {
		t.Fatalf("first switch count = %d, want 9", result.Points[0].SwitchCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
