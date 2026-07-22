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

	mock.ExpectQuery(`(?s)WITH requested_models AS .*AVG\(ul.first_token_ms\).*COALESCE\(ul.image_count, 0\) = 0.*COALESCE\(ul.video_count, 0\) = 0.*FROM usage_logs ul.*ul.channel_id = \$1.*ul.group_id = \$2.*ul.actual_cost > 0.*FROM ops_error_logs e.*FROM channel_groups cg.*e.group_id = \$2.*COALESCE\(e.status_code, 0\) >= 400.*e.is_business_limited = FALSE.*e.is_count_tokens = FALSE`).
		WithArgs(int64(7), nil, "openai", sqlmock.AnyArg(), start, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"model", "success_count", "failure_count", "avg_ttft_ms",
		}).
			AddRow("gpt-5.4", int64(12), int64(1), 1250.5).
			AddRow("gpt-5.4-mini", int64(0), int64(0), nil))

	samples, err := repo.ComputePassiveSamples(
		context.Background(),
		int64Pointer(7),
		nil,
		"openai",
		[]string{"gpt-5.4", "gpt-5.4-mini"},
		start,
		end,
	)

	require.NoError(t, err)
	require.Len(t, samples, 2)
	require.Equal(t, int64(12), samples[0].SuccessCount)
	require.Equal(t, int64(1), samples[0].FailureCount)
	require.NotNil(t, samples[0].AvgTTFTMs)
	require.Equal(t, 1250, *samples[0].AvgTTFTMs)
	require.Nil(t, samples[1].AvgTTFTMs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelMonitorRepositoryComputePassiveSamplesByGroup(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &channelMonitorRepository{db: db}
	start := time.Date(2026, 7, 18, 2, 0, 0, 0, time.UTC)
	end := start.Add(90 * time.Second)

	mock.ExpectQuery(`(?s)FROM usage_logs ul.*ul.group_id = \$2.*FROM ops_error_logs e.*e.group_id = \$2`).
		WithArgs(nil, int64(12), "anthropic", sqlmock.AnyArg(), start, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"model", "success_count", "failure_count", "avg_ttft_ms",
		}).AddRow("claude-sonnet-4-6", int64(8), int64(2), 900.0))

	samples, err := repo.ComputePassiveSamples(
		context.Background(),
		nil,
		int64Pointer(12),
		"anthropic",
		[]string{"claude-sonnet-4-6"},
		start,
		end,
	)

	require.NoError(t, err)
	require.Len(t, samples, 1)
	require.Equal(t, int64(8), samples[0].SuccessCount)
	require.Equal(t, int64(2), samples[0].FailureCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func int64Pointer(value int64) *int64 {
	return &value
}
