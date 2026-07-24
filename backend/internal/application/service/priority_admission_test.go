//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type priorityAdmissionCacheForTest struct {
	stubConcurrencyCacheForTest

	mu                   sync.Mutex
	legacyAcquireCalls   int
	priorityAcquireCalls int
	priorityRequests     []PriorityAccountAdmissionRequest
	priorityStatuses     []PriorityAccountAdmissionStatus
	priorityErr          error
	cancelledRequestIDs  []string
	priorityUserRequests []PriorityUserAdmissionRequest
	priorityUserStatuses []PriorityAccountAdmissionStatus
	priorityUserErr      error
	cancelledUserIDs     []string
}

func (c *priorityAdmissionCacheForTest) AcquireAccountSlot(_ context.Context, _ int64, _ int, _ string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.legacyAcquireCalls++
	return c.acquireResult, c.acquireErr
}

func (c *priorityAdmissionCacheForTest) ReleaseAccountSlot(context.Context, int64, string) error {
	return c.releaseErr
}

func (c *priorityAdmissionCacheForTest) AcquirePriorityAccountSlot(_ context.Context, request PriorityAccountAdmissionRequest) (PriorityAccountAdmissionStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.priorityAcquireCalls++
	c.priorityRequests = append(c.priorityRequests, request)
	if c.priorityErr != nil {
		return PriorityAccountAdmissionRejected, c.priorityErr
	}
	if len(c.priorityStatuses) == 0 {
		return PriorityAccountAdmissionRejected, nil
	}
	status := c.priorityStatuses[0]
	c.priorityStatuses = c.priorityStatuses[1:]
	return status, nil
}

func (c *priorityAdmissionCacheForTest) CancelPriorityAccountWait(_ context.Context, _ int64, requestID string) error {
	c.mu.Lock()
	c.cancelledRequestIDs = append(c.cancelledRequestIDs, requestID)
	c.mu.Unlock()
	return nil
}

func (c *priorityAdmissionCacheForTest) AcquirePriorityUserSlot(_ context.Context, request PriorityUserAdmissionRequest) (PriorityAccountAdmissionStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.priorityUserRequests = append(c.priorityUserRequests, request)
	if c.priorityUserErr != nil {
		return PriorityAccountAdmissionRejected, c.priorityUserErr
	}
	if len(c.priorityUserStatuses) == 0 {
		return PriorityAccountAdmissionRejected, nil
	}
	status := c.priorityUserStatuses[0]
	c.priorityUserStatuses = c.priorityUserStatuses[1:]
	return status, nil
}

func (c *priorityAdmissionCacheForTest) CancelPriorityUserWait(_ context.Context, _ int64, requestID string) error {
	c.mu.Lock()
	c.cancelledUserIDs = append(c.cancelledUserIDs, requestID)
	c.mu.Unlock()
	return nil
}

var _ PriorityAdmissionCache = (*priorityAdmissionCacheForTest)(nil)

type priorityAdmissionBenchmarkCache struct {
	stubConcurrencyCacheForTest
}

func (c *priorityAdmissionBenchmarkCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}

func (c *priorityAdmissionBenchmarkCache) ReleaseAccountSlot(context.Context, int64, string) error {
	return nil
}

func (c *priorityAdmissionBenchmarkCache) AcquirePriorityAccountSlot(context.Context, PriorityAccountAdmissionRequest) (PriorityAccountAdmissionStatus, error) {
	return PriorityAccountAdmissionAcquired, nil
}

func (c *priorityAdmissionBenchmarkCache) CancelPriorityAccountWait(context.Context, int64, string) error {
	return nil
}

func (c *priorityAdmissionBenchmarkCache) AcquirePriorityUserSlot(context.Context, PriorityUserAdmissionRequest) (PriorityAccountAdmissionStatus, error) {
	return PriorityAccountAdmissionAcquired, nil
}

func (c *priorityAdmissionBenchmarkCache) CancelPriorityUserWait(context.Context, int64, string) error {
	return nil
}

func enablePriorityAdmissionForTest(svc *ConcurrencyService, count int, bytes int64) {
	svc.SetPriorityAdmissionRuntimeConfig(PriorityAdmissionRuntimeConfig{
		Enabled:                 true,
		PendingLimitPerInstance: count,
		PendingBytesPerInstance: bytes,
	})
}

func TestPriorityAdmissionFeatureOffUsesLegacyPathOnly(t *testing.T) {
	cache := &priorityAdmissionCacheForTest{}
	cache.acquireResult = true
	svc := NewConcurrencyService(cache)
	ctx := WithRequestSchedulingTier(context.Background(), RequestSchedulingTierLow)

	result, err := svc.AcquireAccountSlot(ctx, 41, 1)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	result.ReleaseFunc()
	require.Equal(t, 1, cache.legacyAcquireCalls)
	require.Zero(t, cache.priorityAcquireCalls)
}

func TestPriorityAdmissionEnabledUsesExplicitGatewayTier(t *testing.T) {
	cache := &priorityAdmissionCacheForTest{
		priorityStatuses: []PriorityAccountAdmissionStatus{PriorityAccountAdmissionRejected},
	}
	cache.acquireResult = true
	svc := NewConcurrencyService(cache)
	enablePriorityAdmissionForTest(svc, 256, 256<<20)

	// Untagged internal work stays on the legacy path.
	legacyResult, err := svc.AcquireAccountSlot(context.Background(), 42, 1)
	require.NoError(t, err)
	require.True(t, legacyResult.Acquired)
	legacyResult.ReleaseFunc()

	ctx := WithRequestSchedulingTier(context.Background(), RequestSchedulingTierLow)
	priorityResult, err := svc.AcquireAccountSlotForTier(ctx, 42, 1, RequestSchedulingTierLow)
	require.NoError(t, err)
	require.False(t, priorityResult.Acquired)
	require.Equal(t, 1, cache.legacyAcquireCalls)
	require.Equal(t, 1, cache.priorityAcquireCalls)
	require.Equal(t, RequestSchedulingTierLow, cache.priorityRequests[0].Tier)
	require.False(t, cache.priorityRequests[0].Register)
}

func TestPriorityAdmissionEnabledFailsClosedWhenCacheErrors(t *testing.T) {
	cache := &priorityAdmissionCacheForTest{priorityErr: errors.New("redis down")}
	svc := NewConcurrencyService(cache)
	enablePriorityAdmissionForTest(svc, 256, 256<<20)

	ctx := WithRequestSchedulingTier(context.Background(), RequestSchedulingTierNormal)
	result, err := svc.AcquireAccountSlotForTier(ctx, 43, 1, RequestSchedulingTierNormal)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrPriorityAdmissionUnavailable)
}

func TestPriorityAdmissionInstanceBudgetReservesQuarterForOtherTier(t *testing.T) {
	cache := &priorityAdmissionCacheForTest{}
	cache.waitAllowed = true
	svc := NewConcurrencyService(cache)
	enablePriorityAdmissionForTest(svc, 4, 100)

	var leases []*PriorityUserWaitLease
	for i := 0; i < 3; i++ {
		lease, allowed, err := svc.BeginPriorityUserWait(int64(i+1), 1, 20, RequestSchedulingTierNormal, 25, time.Second)
		require.NoError(t, err)
		require.True(t, allowed)
		leases = append(leases, lease)
	}
	lease, allowed, err := svc.BeginPriorityUserWait(10, 1, 20, RequestSchedulingTierNormal, 1, time.Second)
	require.NoError(t, err)
	require.False(t, allowed, "normal tier must stop at 75% of the instance budget")
	require.Nil(t, lease)

	priorityLease, allowed, err := svc.BeginPriorityUserWait(11, 1, 20, RequestSchedulingTierPriority, 25, time.Second)
	require.NoError(t, err)
	require.True(t, allowed, "the reserved quarter remains available to priority traffic")
	leases = append(leases, priorityLease)

	lease, allowed, err = svc.BeginPriorityUserWait(12, 1, 20, RequestSchedulingTierPriority, 1, time.Second)
	require.NoError(t, err)
	require.False(t, allowed, "the global count and byte limits are hard caps")
	require.Nil(t, lease)
	require.Equal(t, PriorityAdmissionPendingSnapshot{
		PriorityCount: 1,
		NormalCount:   3,
		TotalCount:    4,
		PriorityBytes: 25,
		NormalBytes:   75,
		TotalBytes:    100,
	}, svc.PriorityAdmissionPendingSnapshot())

	for _, activeLease := range leases {
		activeLease.Close()
	}
	require.Equal(t, PriorityAdmissionPendingSnapshot{}, svc.PriorityAdmissionPendingSnapshot())
}

func TestPriorityAdmissionInstanceTierBudgetUsesStrictFloor(t *testing.T) {
	for _, tc := range []struct {
		limit        int
		allowedFirst int
	}{
		{limit: 1, allowedFirst: 0},
		{limit: 2, allowedFirst: 1},
		{limit: 3, allowedFirst: 2},
	} {
		t.Run(fmt.Sprintf("limit_%d", tc.limit), func(t *testing.T) {
			cache := &priorityAdmissionCacheForTest{}
			svc := NewConcurrencyService(cache)
			enablePriorityAdmissionForTest(svc, tc.limit, 1024)

			var leases []*PriorityUserWaitLease
			for i := 0; i < tc.allowedFirst; i++ {
				lease, allowed, err := svc.BeginPriorityUserWait(int64(i+1), 1, 20, RequestSchedulingTierPriority, 0, time.Second)
				require.NoError(t, err)
				require.True(t, allowed)
				leases = append(leases, lease)
			}
			lease, allowed, err := svc.BeginPriorityUserWait(100, 1, 20, RequestSchedulingTierPriority, 0, time.Second)
			require.NoError(t, err)
			require.False(t, allowed)
			require.Nil(t, lease)

			for _, active := range leases {
				active.Close()
			}
		})
	}
}

func TestPriorityAdmissionRequestSnapshotSurvivesGlobalDisable(t *testing.T) {
	cache := &priorityAdmissionCacheForTest{
		priorityUserStatuses: []PriorityAccountAdmissionStatus{PriorityAccountAdmissionRejected},
	}
	svc := NewConcurrencyService(cache)
	enablePriorityAdmissionForTest(svc, 4, 100)
	ctx, enabled := svc.WithPriorityAdmissionRequestSnapshot(context.Background(), RequestSchedulingTierNormal)
	require.True(t, enabled)
	svc.SetPriorityAdmissionRuntimeConfig(DefaultPriorityAdmissionRuntimeConfig())

	result, err := svc.AcquireUserSlotForTier(ctx, 1, 1, RequestSchedulingTierNormal)
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.Len(t, cache.priorityUserRequests, 1)

	lease, allowed, err := svc.BeginPriorityUserWaitForContext(ctx, 1, 1, 4, RequestSchedulingTierNormal, 10, time.Second)
	require.NoError(t, err)
	require.True(t, allowed)
	lease.Close()
}

func TestSchedulerAccountAdmissionUsesCapturedSnapshotAfterGlobalDisable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		acquire func(*ConcurrencyService, context.Context) (*AcquireResult, error)
	}{
		{
			name: "anthropic gateway",
			acquire: func(concurrency *ConcurrencyService, ctx context.Context) (*AcquireResult, error) {
				gateway := &GatewayService{concurrencyService: concurrency}
				return gateway.tryAcquireAccountSlot(ctx, 1, 1)
			},
		},
		{
			name: "openai gateway",
			acquire: func(concurrency *ConcurrencyService, ctx context.Context) (*AcquireResult, error) {
				gateway := &OpenAIGatewayService{concurrencyService: concurrency}
				return gateway.tryAcquireAccountSlot(ctx, 1, 1)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := &priorityAdmissionCacheForTest{
				priorityStatuses: []PriorityAccountAdmissionStatus{PriorityAccountAdmissionRejected},
			}
			cache.acquireResult = true
			concurrency := NewConcurrencyService(cache)
			enablePriorityAdmissionForTest(concurrency, 4, 100)
			ctx, enabled := concurrency.WithPriorityAdmissionRequestSnapshot(context.Background(), RequestSchedulingTierNormal)
			require.True(t, enabled)
			concurrency.SetPriorityAdmissionRuntimeConfig(DefaultPriorityAdmissionRuntimeConfig())

			result, err := tc.acquire(concurrency, ctx)
			require.NoError(t, err)
			require.False(t, result.Acquired)
			require.Equal(t, 1, cache.priorityAcquireCalls)
			require.Zero(t, cache.legacyAcquireCalls)

			result, err = concurrency.AcquireAccountSlot(context.Background(), 2, 1)
			require.NoError(t, err)
			require.True(t, result.Acquired)
			require.Equal(t, 1, cache.priorityAcquireCalls)
			require.Equal(t, 1, cache.legacyAcquireCalls)
			result.ReleaseFunc()
		})
	}
}

func TestPriorityAdmissionLowTierAccountSnapshotAttemptsRedisOnce(t *testing.T) {
	cache := &priorityAdmissionCacheForTest{
		priorityStatuses: []PriorityAccountAdmissionStatus{PriorityAccountAdmissionRejected},
	}
	svc := NewConcurrencyService(cache)
	enablePriorityAdmissionForTest(svc, 4, 100)
	ctx, enabled := svc.WithPriorityAdmissionRequestSnapshot(context.Background(), RequestSchedulingTierLow)
	require.True(t, enabled)

	for i := 0; i < 2; i++ {
		result, err := svc.AcquireAccountSlotForTier(ctx, int64(i+1), 1, RequestSchedulingTierLow)
		require.NoError(t, err)
		require.False(t, result.Acquired)
		require.True(t, result.PriorityAdmissionTerminal)
	}
	require.Equal(t, 1, cache.priorityAcquireCalls)
}

func TestPriorityAdmissionRefreshStartsNewWebSocketTurn(t *testing.T) {
	cache := &priorityAdmissionCacheForTest{
		priorityStatuses: []PriorityAccountAdmissionStatus{
			PriorityAccountAdmissionRejected,
			PriorityAccountAdmissionRejected,
		},
	}
	svc := NewConcurrencyService(cache)
	enablePriorityAdmissionForTest(svc, 4, 100)
	ctx, enabled := svc.RefreshPriorityAdmissionRequestSnapshot(context.Background(), RequestSchedulingTierLow)
	require.True(t, enabled)

	result, err := svc.AcquireAccountSlotForTier(ctx, 1, 1, RequestSchedulingTierLow)
	require.NoError(t, err)
	require.True(t, result.PriorityAdmissionTerminal)
	ctx, enabled = svc.RefreshPriorityAdmissionRequestSnapshot(ctx, RequestSchedulingTierLow)
	require.True(t, enabled)
	refreshedSnapshot := ctx.Value(priorityAdmissionRequestSnapshotContextKey{}).(*priorityAdmissionRequestSnapshot)
	require.Nil(t, refreshedSnapshot.baseContext.Value(priorityAdmissionRequestSnapshotContextKey{}), "per-turn refresh must not retain an ever-growing snapshot context chain")
	result, err = svc.AcquireAccountSlotForTier(ctx, 2, 1, RequestSchedulingTierLow)
	require.NoError(t, err)
	require.True(t, result.PriorityAdmissionTerminal)
	require.Equal(t, 2, cache.priorityAcquireCalls)

	svc.SetPriorityAdmissionRuntimeConfig(DefaultPriorityAdmissionRuntimeConfig())
	ctx, enabled = svc.RefreshPriorityAdmissionRequestSnapshot(ctx, RequestSchedulingTierLow)
	require.False(t, enabled)
	require.False(t, svc.PriorityAdmissionEnabledForRequest(ctx))
}

func TestPriorityAdmissionLowTierNeverCreatesWaitLease(t *testing.T) {
	cache := &priorityAdmissionCacheForTest{}
	cache.waitAllowed = true
	svc := NewConcurrencyService(cache)
	enablePriorityAdmissionForTest(svc, 256, 256<<20)

	userLease, allowed, err := svc.BeginPriorityUserWait(1, 1, 20, RequestSchedulingTierLow, 10, time.Second)
	require.NoError(t, err)
	require.False(t, allowed)
	require.Nil(t, userLease)
	accountWaiter, allowed, err := svc.BeginPriorityAccountWait(1, 1, 20, RequestSchedulingTierLow, 10, time.Second)
	require.NoError(t, err)
	require.False(t, allowed)
	require.Nil(t, accountWaiter)
	require.Equal(t, PriorityAdmissionPendingSnapshot{}, svc.PriorityAdmissionPendingSnapshot())
}

func TestPriorityAccountWaiterKeepsStableIDAndReleasesBudgetOnAcquire(t *testing.T) {
	cache := &priorityAdmissionCacheForTest{
		priorityStatuses: []PriorityAccountAdmissionStatus{
			PriorityAccountAdmissionWaiting,
			PriorityAccountAdmissionAcquired,
		},
	}
	svc := NewConcurrencyService(cache)
	enablePriorityAdmissionForTest(svc, 4, 100)

	waiter, allowed, err := svc.BeginPriorityAccountWait(9, 1, 4, RequestSchedulingTierPriority, 30, time.Second)
	require.NoError(t, err)
	require.True(t, allowed)
	require.Equal(t, int64(1), svc.PriorityAdmissionPendingSnapshot().TotalCount)

	result, status, err := waiter.TryAcquire(context.Background())
	require.NoError(t, err)
	require.Nil(t, result)
	require.Equal(t, PriorityAccountAdmissionWaiting, status)
	result, status, err = waiter.TryAcquire(context.Background())
	require.NoError(t, err)
	require.Equal(t, PriorityAccountAdmissionAcquired, status)
	require.True(t, result.Acquired)
	require.Equal(t, cache.priorityRequests[0].RequestID, cache.priorityRequests[1].RequestID)
	require.Zero(t, svc.PriorityAdmissionPendingSnapshot().TotalCount)
	result.ReleaseFunc()
	waiter.Close()
	require.Empty(t, cache.cancelledRequestIDs, "known successful acquisition must not add a cleanup RTT")
}

func TestPriorityUserWaitFailsClosedOnRedisError(t *testing.T) {
	cache := &priorityAdmissionCacheForTest{priorityUserErr: errors.New("redis timeout")}
	svc := NewConcurrencyService(cache)
	enablePriorityAdmissionForTest(svc, 4, 100)

	lease, allowed, err := svc.BeginPriorityUserWait(1, 1, 20, RequestSchedulingTierNormal, 10, time.Second)
	require.NoError(t, err)
	require.True(t, allowed)
	result, status, err := lease.TryAcquire(context.Background())
	require.Nil(t, result)
	require.Equal(t, PriorityAccountAdmissionRejected, status)
	require.ErrorIs(t, err, ErrPriorityAdmissionUnavailable)
	require.Equal(t, PriorityAdmissionPendingSnapshot{}, svc.PriorityAdmissionPendingSnapshot())
	require.Len(t, cache.cancelledUserIDs, 1, "Redis failure still requires bounded explicit cleanup in addition to TTL expiry")
}

func BenchmarkPriorityAdmissionAccountAcquire(b *testing.B) {
	legacyCtx := context.Background()
	priorityCtx := WithRequestSchedulingTier(context.Background(), RequestSchedulingTierPriority)
	b.Run("legacy_direct", func(b *testing.B) {
		svc := NewConcurrencyService(&priorityAdmissionBenchmarkCache{})
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			result, err := svc.acquireAccountSlotLegacy(legacyCtx, 1, 1)
			if err != nil || !result.Acquired {
				b.Fatalf("legacy acquire failed: %v", err)
			}
			result.ReleaseFunc()
		}
	})
	b.Run("feature_off_public", func(b *testing.B) {
		svc := NewConcurrencyService(&priorityAdmissionBenchmarkCache{})
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			result, err := svc.AcquireAccountSlot(legacyCtx, 1, 1)
			if err != nil || !result.Acquired {
				b.Fatalf("feature-off acquire failed: %v", err)
			}
			result.ReleaseFunc()
		}
	})
	b.Run("enabled_unsaturated", func(b *testing.B) {
		svc := NewConcurrencyService(&priorityAdmissionBenchmarkCache{})
		enablePriorityAdmissionForTest(svc, 256, 256<<20)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			result, err := svc.AcquireAccountSlotForTier(priorityCtx, 1, 1, RequestSchedulingTierPriority)
			if err != nil || !result.Acquired {
				b.Fatalf("enabled acquire failed: %v", err)
			}
			result.ReleaseFunc()
		}
	})
}
