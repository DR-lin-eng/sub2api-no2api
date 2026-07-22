package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/usagestats"
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

type accountHourlyUsageBatchRepoStub struct {
	UsageLogRepository
	batchCalls atomic.Int64
}

func (s *accountHourlyUsageBatchRepoStub) GetAccountHourlyUsageStatsBatch(_ context.Context, accountIDs []int64, _, _ time.Time) (map[int64]*usagestats.AccountHourlyUsageStats, error) {
	s.batchCalls.Add(1)
	return map[int64]*usagestats.AccountHourlyUsageStats{
		accountIDs[0]: {TotalRequests: 5, SuccessfulRequests: 4, SuccessRate: 0.8},
	}, nil
}

func TestGetAccountHourlyUsageStatsBatchStaysBatchOnly(t *testing.T) {
	repo := &accountHourlyUsageBatchRepoStub{}
	service := &AccountUsageService{usageLogRepo: repo}
	endTime := time.Now().UTC()

	stats, err := service.GetAccountHourlyUsageStatsBatch(
		context.Background(), []int64{7, 8}, endTime.Add(-time.Hour), endTime,
	)

	require.NoError(t, err)
	require.Equal(t, int64(1), repo.batchCalls.Load())
	require.Equal(t, int64(5), stats[7].TotalRequests)
}

func TestGetAccountHourlyUsageStatsBatchRejectsNPlusOneFallback(t *testing.T) {
	service := &AccountUsageService{usageLogRepo: &accountUsageByStartBatchRepoStub{}}
	endTime := time.Now().UTC()

	_, err := service.GetAccountHourlyUsageStatsBatch(
		context.Background(), []int64{7, 8}, endTime.Add(-time.Hour), endTime,
	)

	require.ErrorContains(t, err, "batch query is not supported")
}
