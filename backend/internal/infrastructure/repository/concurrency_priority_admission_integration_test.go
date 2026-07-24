//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type priorityAdmissionCommandCountHook struct {
	commands atomic.Int64
}

func (h *priorityAdmissionCommandCountHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *priorityAdmissionCommandCountHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if name := strings.ToLower(cmd.Name()); name == "evalsha" || name == "eval" {
			h.commands.Add(1)
		}
		return next(ctx, cmd)
	}
}

func (h *priorityAdmissionCommandCountHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

type prioritySlotIntegrationDriver struct {
	acquire func(requestID string, tier service.RequestSchedulingTier, register bool, maxWaiting int, waitTimeout time.Duration) (service.PriorityAccountAdmissionStatus, error)
	release func(requestID string) error
	cancel  func(requestID string) error
	keys    priorityAccountKeys
	waitKey string
}

func (s *ConcurrencyCacheSuite) priorityAccountDriver(accountID int64) prioritySlotIntegrationDriver {
	return prioritySlotIntegrationDriver{
		acquire: func(requestID string, tier service.RequestSchedulingTier, register bool, maxWaiting int, waitTimeout time.Duration) (service.PriorityAccountAdmissionStatus, error) {
			return s.rawCache.AcquirePriorityAccountSlot(s.ctx, service.PriorityAccountAdmissionRequest{
				AccountID:      accountID,
				MaxConcurrency: 1,
				MaxWaiting:     maxWaiting,
				Tier:           tier,
				RequestID:      requestID,
				WaitTimeout:    waitTimeout,
				Register:       register,
			})
		},
		release: func(requestID string) error {
			return s.rawCache.ReleaseAccountSlot(s.ctx, accountID, requestID)
		},
		cancel: func(requestID string) error {
			return s.rawCache.CancelPriorityAccountWait(s.ctx, accountID, requestID)
		},
		keys:    priorityWaitKeys(priorityAccountWaitPrefix, accountID),
		waitKey: priorityWaitCountKey(priorityAccountWaitPrefix, accountID),
	}
}

func (s *ConcurrencyCacheSuite) priorityUserDriver(userID int64) prioritySlotIntegrationDriver {
	return prioritySlotIntegrationDriver{
		acquire: func(requestID string, tier service.RequestSchedulingTier, register bool, maxWaiting int, waitTimeout time.Duration) (service.PriorityAccountAdmissionStatus, error) {
			return s.rawCache.AcquirePriorityUserSlot(s.ctx, service.PriorityUserAdmissionRequest{
				UserID:         userID,
				MaxConcurrency: 1,
				MaxWaiting:     maxWaiting,
				Tier:           tier,
				RequestID:      requestID,
				WaitTimeout:    waitTimeout,
				Register:       register,
			})
		},
		release: func(requestID string) error {
			return s.rawCache.ReleaseUserSlot(s.ctx, userID, requestID)
		},
		cancel: func(requestID string) error {
			return s.rawCache.CancelPriorityUserWait(s.ctx, userID, requestID)
		},
		keys:    priorityWaitKeys(priorityUserWaitPrefix, userID),
		waitKey: priorityWaitCountKey(priorityUserWaitPrefix, userID),
	}
}

func (s *ConcurrencyCacheSuite) TestPriorityAdmission_AccountAndUserFourToOneFairness() {
	drivers := map[string]prioritySlotIntegrationDriver{
		"account": s.priorityAccountDriver(81001),
		"user":    s.priorityUserDriver(82001),
	}
	for name, driver := range drivers {
		s.Run(name, func() {
			status, err := driver.acquire("blocker", service.RequestSchedulingTierNormal, false, 0, 0)
			require.NoError(s.T(), err)
			require.Equal(s.T(), service.PriorityAccountAdmissionAcquired, status)

			priorityIDs := []string{"p1", "p2", "p3", "p4", "p5", "p6"}
			normalIDs := []string{"n1", "n2"}
			for _, requestID := range priorityIDs {
				status, err = driver.acquire(requestID, service.RequestSchedulingTierPriority, true, 12, time.Minute)
				require.NoError(s.T(), err)
				require.Equal(s.T(), service.PriorityAccountAdmissionWaiting, status)
			}
			for _, requestID := range normalIDs {
				status, err = driver.acquire(requestID, service.RequestSchedulingTierNormal, true, 12, time.Minute)
				require.NoError(s.T(), err)
				require.Equal(s.T(), service.PriorityAccountAdmissionWaiting, status)
			}
			require.NoError(s.T(), driver.release("blocker"))

			for _, expectedID := range []string{"p1", "p2", "p3", "p4", "n1", "p5", "p6", "n2"} {
				tier := service.RequestSchedulingTierPriority
				if expectedID[0] == 'n' {
					tier = service.RequestSchedulingTierNormal
				}
				status, err = driver.acquire(expectedID, tier, true, 12, time.Minute)
				require.NoError(s.T(), err)
				require.Equal(s.T(), service.PriorityAccountAdmissionAcquired, status, "grant %s for %s", expectedID, name)
				require.NoError(s.T(), driver.release(expectedID))
			}
		})
	}
}

func (s *ConcurrencyCacheSuite) TestPriorityAdmission_AccountQueueReservesQuarterPerTier() {
	driver := s.priorityAccountDriver(81002)
	status, err := driver.acquire("blocker", service.RequestSchedulingTierNormal, false, 0, 0)
	require.NoError(s.T(), err)
	require.Equal(s.T(), service.PriorityAccountAdmissionAcquired, status)

	for i := 1; i <= 3; i++ {
		status, err = driver.acquire(fmt.Sprintf("p%d", i), service.RequestSchedulingTierPriority, true, 4, time.Minute)
		require.NoError(s.T(), err)
		require.Equal(s.T(), service.PriorityAccountAdmissionWaiting, status)
	}
	status, err = driver.acquire("p4", service.RequestSchedulingTierPriority, true, 4, time.Minute)
	require.NoError(s.T(), err)
	require.Equal(s.T(), service.PriorityAccountAdmissionQueueFull, status)
	status, err = driver.acquire("n1", service.RequestSchedulingTierNormal, true, 4, time.Minute)
	require.NoError(s.T(), err)
	require.Equal(s.T(), service.PriorityAccountAdmissionWaiting, status)
	status, err = driver.acquire("n2", service.RequestSchedulingTierNormal, true, 4, time.Minute)
	require.NoError(s.T(), err)
	require.Equal(s.T(), service.PriorityAccountAdmissionQueueFull, status)
}

func (s *ConcurrencyCacheSuite) TestPriorityAdmission_AccountQueueTierBudgetUsesStrictFloor() {
	for _, tc := range []struct {
		maxWaiting    int
		prioritySlots int
	}{
		{maxWaiting: 1, prioritySlots: 0},
		{maxWaiting: 2, prioritySlots: 1},
		{maxWaiting: 3, prioritySlots: 2},
	} {
		s.Run(fmt.Sprintf("max_%d", tc.maxWaiting), func() {
			driver := s.priorityAccountDriver(int64(81100 + tc.maxWaiting))
			status, err := driver.acquire("blocker", service.RequestSchedulingTierNormal, false, 0, 0)
			require.NoError(s.T(), err)
			require.Equal(s.T(), service.PriorityAccountAdmissionAcquired, status)

			for i := 0; i < tc.prioritySlots; i++ {
				status, err = driver.acquire(fmt.Sprintf("p%d", i), service.RequestSchedulingTierPriority, true, tc.maxWaiting, time.Minute)
				require.NoError(s.T(), err)
				require.Equal(s.T(), service.PriorityAccountAdmissionWaiting, status)
			}
			status, err = driver.acquire("priority-overflow", service.RequestSchedulingTierPriority, true, tc.maxWaiting, time.Minute)
			require.NoError(s.T(), err)
			require.Equal(s.T(), service.PriorityAccountAdmissionQueueFull, status)
		})
	}
}

func (s *ConcurrencyCacheSuite) TestPriorityAdmission_LowTierNeverQueuesOrBypassesProtectedWaiter() {
	for name, driver := range map[string]prioritySlotIntegrationDriver{
		"account": s.priorityAccountDriver(81003),
		"user":    s.priorityUserDriver(82003),
	} {
		s.Run(name, func() {
			status, err := driver.acquire("blocker", service.RequestSchedulingTierNormal, false, 0, 0)
			require.NoError(s.T(), err)
			require.Equal(s.T(), service.PriorityAccountAdmissionAcquired, status)
			status, err = driver.acquire("normal", service.RequestSchedulingTierNormal, true, 4, time.Minute)
			require.NoError(s.T(), err)
			require.Equal(s.T(), service.PriorityAccountAdmissionWaiting, status)
			require.NoError(s.T(), driver.release("blocker"))

			status, err = driver.acquire("low", service.RequestSchedulingTierLow, true, 4, time.Minute)
			require.NoError(s.T(), err)
			require.Equal(s.T(), service.PriorityAccountAdmissionRejected, status)
			_, err = s.rdb.ZScore(s.ctx, driver.keys.priority, "low").Result()
			require.ErrorIs(s.T(), err, redis.Nil)
			_, err = s.rdb.ZScore(s.ctx, driver.keys.normal, "low").Result()
			require.ErrorIs(s.T(), err, redis.Nil)

			status, err = driver.acquire("normal", service.RequestSchedulingTierNormal, true, 4, time.Minute)
			require.NoError(s.T(), err)
			require.Equal(s.T(), service.PriorityAccountAdmissionAcquired, status)
			require.NoError(s.T(), driver.release("normal"))
		})
	}
}

func (s *ConcurrencyCacheSuite) TestPriorityAdmission_IdempotentCancelAndDeadlineCleanup() {
	driver := s.priorityAccountDriver(81004)
	status, err := driver.acquire("blocker", service.RequestSchedulingTierNormal, false, 0, 0)
	require.NoError(s.T(), err)
	require.Equal(s.T(), service.PriorityAccountAdmissionAcquired, status)

	for i := 0; i < 2; i++ {
		status, err = driver.acquire("same", service.RequestSchedulingTierPriority, true, 4, time.Minute)
		require.NoError(s.T(), err)
		require.Equal(s.T(), service.PriorityAccountAdmissionWaiting, status)
	}
	count, err := s.rdb.ZCard(s.ctx, driver.keys.priority).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), count)
	require.NoError(s.T(), driver.cancel("same"))
	count, err = s.rdb.ZCard(s.ctx, driver.keys.priority).Result()
	require.NoError(s.T(), err)
	require.Zero(s.T(), count)

	status, err = driver.acquire("expires", service.RequestSchedulingTierNormal, true, 4, 25*time.Millisecond)
	require.NoError(s.T(), err)
	require.Equal(s.T(), service.PriorityAccountAdmissionWaiting, status)
	time.Sleep(40 * time.Millisecond)
	status, err = driver.acquire("replacement", service.RequestSchedulingTierNormal, true, 4, time.Minute)
	require.NoError(s.T(), err)
	require.Equal(s.T(), service.PriorityAccountAdmissionWaiting, status)
	_, err = s.rdb.ZScore(s.ctx, driver.keys.normal, "expires").Result()
	require.ErrorIs(s.T(), err, redis.Nil)
	waiting, err := s.rdb.Get(s.ctx, driver.waitKey).Int()
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, waiting)
}

func (s *ConcurrencyCacheSuite) TestPriorityAdmission_ExpiredBurstDoesNotLeaveHeadOfLineBlockers() {
	driver := s.priorityAccountDriver(81007)
	status, err := driver.acquire("blocker", service.RequestSchedulingTierNormal, false, 0, 0)
	require.NoError(s.T(), err)
	require.Equal(s.T(), service.PriorityAccountAdmissionAcquired, status)

	for i := 0; i < 70; i++ {
		status, err = driver.acquire(fmt.Sprintf("expired-%d", i), service.RequestSchedulingTierNormal, true, 100, 10*time.Millisecond)
		require.NoError(s.T(), err)
		require.Equal(s.T(), service.PriorityAccountAdmissionWaiting, status)
	}
	time.Sleep(20 * time.Millisecond)
	status, err = driver.acquire("replacement", service.RequestSchedulingTierNormal, true, 100, time.Minute)
	require.NoError(s.T(), err)
	require.Equal(s.T(), service.PriorityAccountAdmissionWaiting, status)
	count, err := s.rdb.ZCard(s.ctx, driver.keys.normal).Result()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), count)
}

func (s *ConcurrencyCacheSuite) TestPriorityAdmission_WarmSuccessUsesOneRedisRoundTrip() {
	driver := s.priorityAccountDriver(81005)
	status, err := driver.acquire("warm", service.RequestSchedulingTierNormal, false, 0, 0)
	require.NoError(s.T(), err)
	require.Equal(s.T(), service.PriorityAccountAdmissionAcquired, status)
	require.NoError(s.T(), driver.release("warm"))

	hook := &priorityAdmissionCommandCountHook{}
	s.rdb.AddHook(hook)
	status, err = driver.acquire("measured", service.RequestSchedulingTierNormal, false, 0, 0)
	require.NoError(s.T(), err)
	require.Equal(s.T(), service.PriorityAccountAdmissionAcquired, status)
	require.Equal(s.T(), int64(1), hook.commands.Load(), "warm successful acquisition must be exactly one EVALSHA/EVAL")
}

func (s *ConcurrencyCacheSuite) TestPriorityAdmission_WaitCountsAreIndependentAndMergedOnlyWhenEnabled() {
	accountID := int64(81006)
	legacyKey := accountWaitKey(accountID)
	priorityKey := priorityWaitCountKey(priorityAccountWaitPrefix, accountID)
	require.NoError(s.T(), s.rdb.Set(s.ctx, legacyKey, 2, time.Minute).Err())
	require.NoError(s.T(), s.rdb.Set(s.ctx, priorityKey, 3, time.Minute).Err())

	svc := service.NewConcurrencyService(s.rawCache)
	count, err := svc.GetAccountWaitingCount(s.ctx, accountID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 2, count)

	svc.SetPriorityAdmissionRuntimeConfig(service.PriorityAdmissionRuntimeConfig{
		Enabled:                 true,
		PendingLimitPerInstance: 256,
		PendingBytesPerInstance: 256 << 20,
	})
	count, err = svc.GetAccountWaitingCount(s.ctx, accountID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 5, count)
}
