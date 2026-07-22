package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryGetImageGenerationStats(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	groupID := int64(7)
	filter := &service.OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
		Platform:  "OpenAI",
		GroupID:   &groupID,
	}

	mock.ExpectQuery(`(?s)WITH image_usage AS .*ul.image_count > 0 AND COALESCE\(ul.video_count, 0\) = 0.*average_concurrent.*peak_concurrent`).
		WithArgs(start, end, groupID, "openai").
		WillReturnRows(sqlmock.NewRows([]string{
			"request_count",
			"image_count",
			"p95_duration_ms",
			"avg_duration_ms",
			"max_duration_ms",
			"average_concurrent",
			"peak_concurrent",
		}).AddRow(int64(12), int64(15), 24000.0, 18000.0, int64(32000), 3.25, int64(6)))

	mock.ExpectQuery(`(?s)SELECT.*image_output_size.*image_size.*GROUP BY 1, 2.*ORDER BY request_count DESC`).
		WithArgs(start, end, groupID, "openai").
		WillReturnRows(sqlmock.NewRows([]string{
			"resolution",
			"billing_tier",
			"request_count",
			"image_count",
			"avg_duration_ms",
			"p95_duration_ms",
			"max_duration_ms",
		}).
			AddRow("1024x1024", "1K", int64(8), int64(8), 12000.0, 15000.0, int64(18000)).
			AddRow("3840x2160", "4K", int64(4), int64(7), 30000.0, 31000.0, int64(32000)))

	result, err := repo.GetImageGenerationStats(context.Background(), filter)
	require.NoError(t, err)
	require.Equal(t, int64(12), result.RequestCount)
	require.Equal(t, int64(15), result.ImageCount)
	require.InDelta(t, 0.2, result.RequestsPerMinute, 0.001)
	require.NotNil(t, result.AvgDurationMs)
	require.Equal(t, 18000, *result.AvgDurationMs)
	require.NotNil(t, result.P95DurationMs)
	require.Equal(t, 24000, *result.P95DurationMs)
	require.InDelta(t, 3.3, result.AverageConcurrent, 0.001)
	require.Equal(t, int64(6), result.PeakConcurrent)
	require.Len(t, result.ByResolution, 2)
	require.Equal(t, "1024x1024", result.ByResolution[0].Resolution)
	require.Equal(t, "4K", result.ByResolution[1].BillingTier)
	require.NoError(t, mock.ExpectationsWereMet())
}
