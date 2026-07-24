package service

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
)

const openAIContentSessionStressRequests = 50_000

type contentSessionBenchmarkGatewayCache struct {
	GatewayCache
	sticky           sync.Map
	defaultAccountID atomic.Int64
}

type contentSessionBenchmarkCacheKey struct {
	groupID     int64
	sessionHash string
}

func (c *contentSessionBenchmarkGatewayCache) GetSessionAccountID(_ context.Context, groupID int64, sessionHash string) (int64, error) {
	if value, ok := c.sticky.Load(contentSessionBenchmarkCacheKey{groupID: groupID, sessionHash: sessionHash}); ok {
		accountID, _ := value.(int64)
		return accountID, nil
	}
	return c.defaultAccountID.Load(), nil
}

func (c *contentSessionBenchmarkGatewayCache) SetSessionAccountID(_ context.Context, groupID int64, sessionHash string, accountID int64, _ time.Duration) error {
	c.sticky.Store(contentSessionBenchmarkCacheKey{groupID: groupID, sessionHash: sessionHash}, accountID)
	return nil
}

func (*contentSessionBenchmarkGatewayCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (c *contentSessionBenchmarkGatewayCache) DeleteSessionAccountID(_ context.Context, groupID int64, sessionHash string) error {
	c.sticky.Store(contentSessionBenchmarkCacheKey{groupID: groupID, sessionHash: sessionHash}, int64(0))
	return nil
}

type contentSessionBenchmarkAccountRepo struct {
	schedulerTestOpenAIAccountRepo
	listCalls       *atomic.Int64
	activeListCalls *atomic.Int64
	maxListCalls    *atomic.Int64
}

type contentSessionSaturatingConcurrencyCache struct {
	schedulerTestConcurrencyCache
	mu     sync.Mutex
	active map[int64]int
}

func (c *contentSessionSaturatingConcurrencyCache) AcquireAccountSlot(_ context.Context, accountID int64, maxConcurrency int, _ string) (bool, error) {
	if c.acquireResults != nil {
		if result, ok := c.acquireResults[accountID]; ok && !result {
			return false, nil
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active == nil {
		c.active = make(map[int64]int)
	}
	if c.active[accountID] >= maxConcurrency {
		return false, nil
	}
	c.active[accountID]++
	return true, nil
}

func (c *contentSessionSaturatingConcurrencyCache) ReleaseAccountSlot(_ context.Context, accountID int64, _ string) error {
	c.mu.Lock()
	if c.active[accountID] <= 1 {
		delete(c.active, accountID)
	} else {
		c.active[accountID]--
	}
	c.mu.Unlock()
	return nil
}

func (r contentSessionBenchmarkAccountRepo) recordList() func() {
	if r.listCalls != nil {
		r.listCalls.Add(1)
	}
	if r.activeListCalls == nil {
		return func() {}
	}
	active := r.activeListCalls.Add(1)
	for r.maxListCalls != nil {
		previous := r.maxListCalls.Load()
		if active <= previous || r.maxListCalls.CompareAndSwap(previous, active) {
			break
		}
	}
	return func() { r.activeListCalls.Add(-1) }
}

func (r contentSessionBenchmarkAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	done := r.recordList()
	defer done()
	return r.schedulerTestOpenAIAccountRepo.ListSchedulableByGroupIDAndPlatform(ctx, groupID, platform)
}

func (r contentSessionBenchmarkAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	done := r.recordList()
	defer done()
	return r.schedulerTestOpenAIAccountRepo.ListSchedulableByPlatform(ctx, platform)
}

func (r contentSessionBenchmarkAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	done := r.recordList()
	defer done()
	return r.schedulerTestOpenAIAccountRepo.ListSchedulableUngroupedByPlatform(ctx, platform)
}

func newContentSessionSchedulerBenchmark(accountCount int) OpenAIAccountScheduler {
	scheduler, _, _, _, _ := newContentSessionSchedulerStress(accountCount, true, true, false, false)
	return scheduler
}

func newContentSessionSchedulerStress(accountCount int, preboundSticky bool, stickyAvailable bool, staleSticky bool, enforceSlots bool) (OpenAIAccountScheduler, *OpenAIGatewayService, *atomic.Int64, *atomic.Int64, int64) {
	accounts := make([]Account, accountCount)
	loads := make(map[int64]*AccountLoadInfo, accountCount)
	for i := range accounts {
		accountID := int64(20_000 + i)
		accounts[i] = Account{
			ID:          accountID,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 64,
			Priority:    i % 7,
			GroupIDs:    []int64{42},
			Credentials: map[string]any{"base_url": "https://relay.example.com/v1"},
		}
		loads[accountID] = &AccountLoadInfo{
			AccountID:          accountID,
			CurrentConcurrency: i % 32,
			WaitingCount:       i % 5,
			LoadRate:           (i * 17) % 100,
		}
	}

	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.Scheduling.StickySessionMaxWaiting = 3
	cfg.Gateway.Scheduling.StickySessionWaitTimeout = 120 * time.Second
	cfg.Gateway.Scheduling.FallbackMaxWaiting = 100
	cfg.Gateway.OpenAIWS.LBTopK = 7
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = 0.7
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.ErrorRate = 0.8
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT = 0.5

	fullScans := &atomic.Int64{}
	activeFullScans := &atomic.Int64{}
	maxFullScans := &atomic.Int64{}
	cache := &contentSessionBenchmarkGatewayCache{}
	initialStickyID := int64(0)
	if preboundSticky {
		initialStickyID = accounts[0].ID
		if staleSticky {
			initialStickyID = 999_999
		}
		cache.defaultAccountID.Store(initialStickyID)
	}
	concurrencyCache := schedulerTestConcurrencyCache{loadMap: loads}
	if !stickyAvailable {
		concurrencyCache.acquireResults = map[int64]bool{accounts[0].ID: false}
	}
	var effectiveConcurrencyCache ConcurrencyCache = concurrencyCache
	if enforceSlots {
		effectiveConcurrencyCache = &contentSessionSaturatingConcurrencyCache{
			schedulerTestConcurrencyCache: concurrencyCache,
		}
	}
	svc := &OpenAIGatewayService{
		accountRepo: contentSessionBenchmarkAccountRepo{
			schedulerTestOpenAIAccountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
			listCalls:                      fullScans,
			activeListCalls:                activeFullScans,
			maxListCalls:                   maxFullScans,
		},
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(effectiveConcurrencyCache),
	}
	return newDefaultOpenAIAccountScheduler(svc, nil), svc, fullScans, maxFullScans, initialStickyID
}

func BenchmarkOpenAIContentSessionBurstBalance(b *testing.B) {
	const sessionHash = "same-content-session"
	groupID := int64(42)

	b.Run("tracker_disabled", func(b *testing.B) {
		resetOpenAIAdvancedSchedulerSettingCacheForTest()
		svc := &OpenAIGatewayService{rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("", "", "", "false")}
		_, _, _ = svc.beginOpenAIContentSessionRequest(context.Background(), &groupID, sessionHash)
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				tracked, concurrent, overflow := svc.beginOpenAIContentSessionRequest(context.Background(), &groupID, sessionHash)
				if tracked || concurrent || overflow {
					b.Fatal("disabled tracker unexpectedly tracked a request")
				}
			}
		})
	})

	b.Run("tracker_enabled_same_session", func(b *testing.B) {
		resetOpenAIAdvancedSchedulerSettingCacheForTest()
		svc := &OpenAIGatewayService{rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("", "", "", "true")}
		_, _, _ = svc.beginOpenAIContentSessionRequest(context.Background(), &groupID, sessionHash)
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				tracked, _, _ := svc.beginOpenAIContentSessionRequest(context.Background(), &groupID, sessionHash)
				if !tracked {
					b.Fatal("enabled tracker did not track a request")
				}
				svc.openaiContentSessions.release(groupID, sessionHash)
			}
		})
	})

	scheduler := newContentSessionSchedulerBenchmark(256)
	for _, tc := range []struct {
		name       string
		concurrent bool
	}{
		{name: "sticky_fast_path", concurrent: false},
		{name: "burst_balance_full_scan", concurrent: true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					selection, _, err := scheduler.Select(context.Background(), OpenAIAccountScheduleRequest{
						GroupID:                  &groupID,
						Platform:                 PlatformOpenAI,
						SessionHash:              sessionHash,
						RequestedModel:           "gpt-5.1",
						ContentSessionConcurrent: tc.concurrent,
					})
					if err != nil || selection == nil || selection.Account == nil {
						b.Fatalf("scheduler selection failed: selection=%v err=%v", selection, err)
					}
					if selection.ReleaseFunc != nil {
						selection.ReleaseFunc()
					}
				}
			})
		})
	}
}

func TestOpenAIContentSessionBurstOverflowUsesBoundedCandidates(t *testing.T) {
	groupID := int64(42)
	const sessionHash = "same-content-session"

	for _, tc := range []struct {
		name            string
		advancedEnabled string
	}{
		{name: "advanced", advancedEnabled: "true"},
		{name: "legacy", advancedEnabled: "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			_, gatewayService, fullScans, _, stickyAccountID := newContentSessionSchedulerStress(8, true, true, false, false)
			gatewayService.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService(tc.advancedEnabled, "false", "", "true")
			tracker := gatewayService.openAIContentSessionTracker()
			tracker.begin(groupID, sessionHash)
			tracker.recordCandidate(groupID, sessionHash, 20_001)
			tracker.recordCandidate(groupID, sessionHash, 20_002)

			ctx := withOpenAISessionHashMetadata(context.Background(), openAISessionHashMetadata{
				contentDerived:         true,
				contentRequestTracked:  true,
				contentRequestOverflow: true,
			})
			selectedIDs := make([]int64, 0, 4)
			for range 4 {
				selection, decision, err := gatewayService.SelectAccountWithScheduler(
					ctx, &groupID, "", sessionHash, "gpt-5.1", nil, OpenAIUpstreamTransportAny, false,
				)
				if err != nil || selection == nil || selection.Account == nil {
					t.Fatalf("candidate selection failed: selection=%v err=%v", selection, err)
				}
				if decision.Layer != openAIAccountScheduleLayerContentBurst {
					t.Fatalf("selection layer = %q, want %q", decision.Layer, openAIAccountScheduleLayerContentBurst)
				}
				selectedIDs = append(selectedIDs, selection.Account.ID)
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
			}
			if want := []int64{20_001, 20_002, 20_001, 20_002}; !slices.Equal(selectedIDs, want) {
				t.Fatalf("candidate distribution = %v, want %v", selectedIDs, want)
			}
			if scans := fullScans.Load(); scans != 0 {
				t.Fatalf("overflow candidate path performed %d full account scans", scans)
			}
			if cacheID, err := gatewayService.getStickySessionAccountID(context.Background(), &groupID, sessionHash); err != nil || cacheID != stickyAccountID {
				t.Fatalf("candidate path changed canonical sticky binding: id=%d err=%v want=%d", cacheID, err, stickyAccountID)
			}
			tracker.release(groupID, sessionHash)
		})
	}
}

func TestOpenAIContentSessionBurstBalanceConcurrent50000(t *testing.T) {
	if os.Getenv("SUB2API_RUN_50K_CONCURRENCY_TEST") != "1" {
		t.Skip("set SUB2API_RUN_50K_CONCURRENCY_TEST=1 to run the 50k concurrency stress test")
	}

	const (
		accountCount = 256
		sessionHash  = "same-content-session"
	)
	groupID := int64(42)

	for _, tc := range []struct {
		name             string
		burstBalanced    bool
		preboundSticky   bool
		stickyAvailable  bool
		staleSticky      bool
		sessionCount     int
		legacy           bool
		disableLoadBatch bool
		baselineCold     bool
		distinctSessions bool
		stickyWeighted   bool
		enforceSlots     bool
		holdSlots        bool
		expectWaitPlans  bool
	}{
		{name: "advanced_sticky_fast_path", preboundSticky: true, stickyAvailable: true, sessionCount: 1},
		{name: "advanced_feature_off_cold_binding", stickyAvailable: true, sessionCount: 1, baselineCold: true},
		{name: "advanced_bounded_prebound", burstBalanced: true, preboundSticky: true, stickyAvailable: true, sessionCount: 1},
		{name: "advanced_bounded_cold_binding", burstBalanced: true, stickyAvailable: true, sessionCount: 1},
		{name: "advanced_bounded_stale_binding", burstBalanced: true, preboundSticky: true, stickyAvailable: true, staleSticky: true, sessionCount: 1},
		{name: "advanced_bounded_sticky_full", burstBalanced: true, preboundSticky: true, sessionCount: 1},
		{name: "advanced_bounded_multi_hot_sessions", burstBalanced: true, preboundSticky: true, stickyAvailable: true, sessionCount: 16},
		{name: "advanced_sticky_weighted_bounded", burstBalanced: true, preboundSticky: true, stickyAvailable: true, sessionCount: 1, stickyWeighted: true},
		{name: "advanced_bounded_saturated_slots", burstBalanced: true, preboundSticky: true, stickyAvailable: true, sessionCount: 1, enforceSlots: true, holdSlots: true, expectWaitPlans: true},
		{name: "legacy_sticky_fast_path", preboundSticky: true, stickyAvailable: true, sessionCount: 1, legacy: true},
		{name: "legacy_feature_off_cold_binding", stickyAvailable: true, sessionCount: 1, legacy: true, baselineCold: true},
		{name: "legacy_bounded_prebound", burstBalanced: true, preboundSticky: true, stickyAvailable: true, sessionCount: 1, legacy: true},
		{name: "legacy_bounded_cold_binding", burstBalanced: true, stickyAvailable: true, sessionCount: 1, legacy: true},
		{name: "legacy_bounded_stale_binding", burstBalanced: true, preboundSticky: true, stickyAvailable: true, staleSticky: true, sessionCount: 1, legacy: true},
		{name: "legacy_bounded_sticky_full", burstBalanced: true, preboundSticky: true, sessionCount: 1, legacy: true},
		{name: "legacy_bounded_multi_hot_sessions", burstBalanced: true, preboundSticky: true, stickyAvailable: true, sessionCount: 16, legacy: true},
		{name: "legacy_bounded_saturated_slots", burstBalanced: true, preboundSticky: true, stickyAvailable: true, sessionCount: 1, legacy: true, enforceSlots: true, holdSlots: true, expectWaitPlans: true},
		{name: "legacy_nonbatch_feature_off_cold_binding", stickyAvailable: true, sessionCount: 1, legacy: true, disableLoadBatch: true, baselineCold: true},
		{name: "legacy_nonbatch_cold_binding", burstBalanced: true, stickyAvailable: true, sessionCount: 1, legacy: true, disableLoadBatch: true},
		{name: "advanced_distinct_sessions", burstBalanced: true, stickyAvailable: true, sessionCount: openAIContentSessionStressRequests, distinctSessions: true},
		{name: "legacy_distinct_sessions", burstBalanced: true, stickyAvailable: true, sessionCount: openAIContentSessionStressRequests, legacy: true, distinctSessions: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, gatewayService, actualFullScans, maxParallelFullScans, _ := newContentSessionSchedulerStress(accountCount, tc.preboundSticky, tc.stickyAvailable, tc.staleSticky, tc.enforceSlots)
			if tc.disableLoadBatch {
				gatewayService.cfg.Gateway.Scheduling.LoadBatchEnabled = false
			}
			advancedEnabled := "true"
			if tc.legacy {
				advancedEnabled = "false"
			}
			stickyWeighted := "false"
			if tc.stickyWeighted {
				stickyWeighted = "true"
			}
			burstBalance := "false"
			if tc.burstBalanced {
				burstBalance = "true"
			}
			gatewayService.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService(advancedEnabled, stickyWeighted, "", burstBalance)
			var ready sync.WaitGroup
			var selectionDone sync.WaitGroup
			var done sync.WaitGroup
			var failures atomic.Int64
			var acquiredSelections atomic.Int64
			var waitPlans atomic.Int64
			var balanceSelections atomic.Int64
			var selectedAccounts sync.Map
			start := make(chan struct{})
			releaseHeldSlots := make(chan struct{})
			sessionHashes := make([]string, tc.sessionCount)
			for i := range sessionHashes {
				sessionHashes[i] = sessionHash
				if tc.sessionCount > 1 {
					sessionHashes[i] = fmt.Sprintf("%s-%d", sessionHash, i)
				}
				if tc.burstBalanced && !tc.distinctSessions {
					_, _, _ = gatewayService.beginOpenAIContentSessionRequest(context.Background(), &groupID, sessionHashes[i])
					defer gatewayService.openaiContentSessions.release(groupID, sessionHashes[i])
				}
			}
			ready.Add(openAIContentSessionStressRequests)
			selectionDone.Add(openAIContentSessionStressRequests)
			done.Add(openAIContentSessionStressRequests)

			for i := range openAIContentSessionStressRequests {
				go func(requestIndex int) {
					defer done.Done()
					ready.Done()
					<-start
					requestSessionHash := sessionHashes[requestIndex%len(sessionHashes)]
					tracked, contentSessionConcurrent, contentSessionOverflow := gatewayService.beginOpenAIContentSessionRequest(context.Background(), &groupID, requestSessionHash)
					if tracked {
						defer gatewayService.openaiContentSessions.release(groupID, requestSessionHash)
					}
					if contentSessionConcurrent {
						balanceSelections.Add(1)
					}
					requestCtx := withOpenAISessionHashMetadata(context.Background(), openAISessionHashMetadata{
						contentDerived:           true,
						contentRequestTracked:    tracked,
						contentRequestConcurrent: contentSessionConcurrent,
						contentRequestOverflow:   contentSessionOverflow,
					})
					selection, _, err := gatewayService.SelectAccountWithScheduler(
						requestCtx,
						&groupID,
						"",
						requestSessionHash,
						"gpt-5.1",
						nil,
						OpenAIUpstreamTransportAny,
						false,
					)
					if err != nil || selection == nil || selection.Account == nil {
						failures.Add(1)
						selectionDone.Done()
						return
					}
					if selection.Acquired {
						acquiredSelections.Add(1)
					}
					selectedAccounts.Store(selection.Account.ID, struct{}{})
					if selection.WaitPlan != nil {
						waitPlans.Add(1)
					}
					selectionDone.Done()
					if selection.ReleaseFunc != nil {
						if tc.holdSlots {
							<-releaseHeldSlots
						}
						selection.ReleaseFunc()
					}
				}(i)
			}

			ready.Wait()
			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)
			startedAt := time.Now()
			close(start)
			if tc.holdSlots {
				selectionDone.Wait()
				close(releaseHeldSlots)
			}
			done.Wait()
			elapsed := time.Since(startedAt)
			var after runtime.MemStats
			runtime.ReadMemStats(&after)

			if failures.Load() != 0 {
				t.Fatalf("%d of %d selections failed", failures.Load(), openAIContentSessionStressRequests)
			}
			if tc.expectWaitPlans && waitPlans.Load() == 0 {
				t.Fatal("saturated account slots did not produce any wait plans")
			}
			selectedAccountCount := 0
			selectedAccounts.Range(func(_, _ any) bool {
				selectedAccountCount++
				return true
			})
			if tc.expectWaitPlans && selectedAccountCount < 2 {
				t.Fatalf("saturated burst did not spread across accounts: selected_account_count=%d", selectedAccountCount)
			}
			expectedBalanceSelections := int64(0)
			maxFullScans := int64(0)
			if tc.distinctSessions {
				maxFullScans = openAIContentSessionStressRequests
			} else if tc.burstBalanced {
				expectedBalanceSelections = int64(tc.sessionCount * openAIContentSessionBurstBalanceMaxSelections)
				maxFullScans = expectedBalanceSelections + openAIContentSessionLoadBalanceMaxParallel
			} else if tc.baselineCold {
				maxFullScans = openAIContentSessionStressRequests
			}
			if got := balanceSelections.Load(); got != expectedBalanceSelections {
				t.Fatalf("unexpected balanced selections: got %d want %d", got, expectedBalanceSelections)
			}
			if scans := actualFullScans.Load(); scans > maxFullScans {
				t.Fatalf("actual full account scans exceeded bound: got %d limit %d", scans, maxFullScans)
			}
			if tc.distinctSessions && actualFullScans.Load() != openAIContentSessionStressRequests {
				t.Fatalf("distinct first requests unexpectedly entered burst admission: scans=%d want=%d", actualFullScans.Load(), openAIContentSessionStressRequests)
			}
			if tc.burstBalanced && maxParallelFullScans.Load() > openAIContentSessionLoadBalanceMaxParallel {
				t.Fatalf("parallel full account scans exceeded admission bound: got %d limit %d", maxParallelFullScans.Load(), openAIContentSessionLoadBalanceMaxParallel)
			}
			t.Logf(
				"requests=%d sessions=%d accounts=%d elapsed=%s throughput=%.0f selections/s total_alloc=%d bytes mallocs=%d balance_selections=%d acquired_selections=%d wait_plans=%d selected_account_count=%d actual_full_scans=%d max_parallel_full_scans=%d selection_errors=%d selection_error_rate=%.6f",
				openAIContentSessionStressRequests,
				tc.sessionCount,
				accountCount,
				elapsed,
				float64(openAIContentSessionStressRequests)/elapsed.Seconds(),
				after.TotalAlloc-before.TotalAlloc,
				after.Mallocs-before.Mallocs,
				balanceSelections.Load(),
				acquiredSelections.Load(),
				waitPlans.Load(),
				selectedAccountCount,
				actualFullScans.Load(),
				maxParallelFullScans.Load(),
				failures.Load(),
				float64(failures.Load())/float64(openAIContentSessionStressRequests),
			)
		})
	}
}
