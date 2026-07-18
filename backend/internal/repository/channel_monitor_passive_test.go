package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorRepositoryComputePassiveSamples(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &channelMonitorRepository{db: db}
	start := time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)

	mock.ExpectQuery(`(?s)WITH requested_models AS .*FROM usage_logs ul.*ul.channel_id = \$1.*ul.actual_cost > 0.*FROM ops_error_logs e.*JOIN channel_groups cg.*COALESCE\(e.status_code, 0\) >= 400.*e.is_business_limited = FALSE.*e.is_count_tokens = FALSE`).
		WithArgs(int64(7), "openai", sqlmock.AnyArg(), start, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"model", "success_count", "failure_count", "avg_latency_ms",
		}).
			AddRow("gpt-5.4", int64(12), int64(1), 1250.5).
			AddRow("gpt-5.4-mini", int64(0), int64(0), nil))

	samples, err := repo.ComputePassiveSamples(
		context.Background(),
		7,
		"openai",
		[]string{"gpt-5.4", "gpt-5.4-mini"},
		start,
		end,
	)

	require.NoError(t, err)
	require.Len(t, samples, 2)
	require.Equal(t, int64(12), samples[0].SuccessCount)
	require.Equal(t, int64(1), samples[0].FailureCount)
	require.NotNil(t, samples[0].AvgLatencyMs)
	require.Equal(t, 1250, *samples[0].AvgLatencyMs)
	require.Nil(t, samples[1].AvgLatencyMs)
	require.NoError(t, mock.ExpectationsWereMet())
}
