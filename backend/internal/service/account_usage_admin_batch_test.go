package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type accountUsageByStartBatchRepoStub struct {
	UsageLogRepository

	batchResult map[int64]*usagestats.AccountStats
	batchErr    error
	batchCalls  atomic.Int64
	singleCalls atomic.Int64
}

func (s *accountUsageByStartBatchRepoStub) GetAccountWindowStatsByStartBatch(_ context.Context, _ map[int64]time.Time) (map[int64]*usagestats.AccountStats, error) {
	s.batchCalls.Add(1)
	return s.batchResult, s.batchErr
}

func (s *accountUsageByStartBatchRepoStub) GetAccountWindowStats(_ context.Context, accountID int64, _ time.Time) (*usagestats.AccountStats, error) {
	s.singleCalls.Add(1)
	return &usagestats.AccountStats{StandardCost: float64(accountID)}, nil
}

func TestGetAccountWindowStatsByStartBatchUsesSingleBatchQuery(t *testing.T) {
	repo := &accountUsageByStartBatchRepoStub{batchResult: map[int64]*usagestats.AccountStats{
		1: {StandardCost: 12.5},
	}}
	service := &AccountUsageService{usageLogRepo: repo}

	stats, err := service.GetAccountWindowStatsByStartBatch(context.Background(), map[int64]time.Time{
		1: time.Now().Add(-time.Hour),
		2: time.Now().Add(-2 * time.Hour),
	})

	require.NoError(t, err)
	require.Equal(t, 12.5, stats[1].StandardCost)
	require.Zero(t, stats[2].StandardCost)
	require.Equal(t, int64(1), repo.batchCalls.Load())
	require.Zero(t, repo.singleCalls.Load())
}

func TestGetAccountWindowStatsByStartBatchFallsBackOnBatchError(t *testing.T) {
	repo := &accountUsageByStartBatchRepoStub{batchErr: context.DeadlineExceeded}
	service := &AccountUsageService{usageLogRepo: repo}

	stats, err := service.GetAccountWindowStatsByStartBatch(context.Background(), map[int64]time.Time{
		3: time.Now().Add(-time.Hour),
		4: time.Now().Add(-2 * time.Hour),
	})

	require.NoError(t, err)
	require.Equal(t, 3.0, stats[3].StandardCost)
	require.Equal(t, 4.0, stats[4].StandardCost)
	require.Equal(t, int64(1), repo.batchCalls.Load())
	require.Equal(t, int64(2), repo.singleCalls.Load())
}
