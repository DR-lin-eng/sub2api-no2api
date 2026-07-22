package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryListRequestDetailsTTFTDrilldown(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	filter := &service.OpsRequestDetailFilter{
		StartTime: &start,
		EndTime:   &end,
		Kind:      "success",
		TTFTOnly:  true,
		Sort:      "ttft_desc",
		Page:      1,
		PageSize:  10,
	}

	mock.ExpectQuery(`(?s)SELECT COUNT\(1\) FROM combined WHERE kind = \$3 AND first_token_ms IS NOT NULL`).
		WithArgs(start, end, "success").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	createdAt := start.Add(30 * time.Minute)
	rows := sqlmock.NewRows([]string{
		"kind",
		"created_at",
		"request_id",
		"platform",
		"model",
		"duration_ms",
		"first_token_ms",
		"status_code",
		"error_id",
		"phase",
		"severity",
		"message",
		"user_id",
		"api_key_id",
		"account_id",
		"group_id",
		"stream",
	}).AddRow(
		"success",
		createdAt,
		"req-ttft",
		"openai",
		"gpt-5",
		1200,
		450,
		nil,
		nil,
		nil,
		nil,
		nil,
		int64(1),
		int64(2),
		int64(3),
		int64(4),
		true,
	)

	mock.ExpectQuery(`(?s)FROM combined\s+WHERE kind = \$3 AND first_token_ms IS NOT NULL\s+ORDER BY first_token_ms DESC NULLS LAST, created_at DESC\s+LIMIT \$4 OFFSET \$5`).
		WithArgs(start, end, "success", 10, 0).
		WillReturnRows(rows)

	items, total, err := repo.ListRequestDetails(context.Background(), filter)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, "req-ttft", items[0].RequestID)
	require.NotNil(t, items[0].FirstTokenMs)
	require.Equal(t, 450, *items[0].FirstTokenMs)
	require.NotNil(t, items[0].DurationMs)
	require.Equal(t, 1200, *items[0].DurationMs)
	require.NoError(t, mock.ExpectationsWereMet())
}
