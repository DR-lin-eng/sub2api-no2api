//go:build loadtest

package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

const (
	priorityAdmissionLoadRPM               = 50_000
	priorityAdmissionLoadBatchSize         = 10
	priorityAdmissionLoadAccountSlots      = 64
	priorityAdmissionLoadUserSlots         = 512
	priorityAdmissionLoadMaxWaiting        = 256
	priorityAdmissionLoadPendingBytes      = 1024
	priorityAdmissionLoadDefaultWarmup     = 2 * time.Minute
	priorityAdmissionLoadDefaultSteady     = 10 * time.Minute
	priorityAdmissionLoadDefaultSlotHold   = time.Second
	priorityAdmissionLoadRequestTimeout    = 30 * time.Second
	priorityAdmissionLoadSampleInterval    = time.Second
	priorityAdmissionLoadResourceMaxGrowth = 0.05
	priorityAdmissionLoadCPUMaxGrowth      = 0.10
	priorityAdmissionLoadFairnessMaxError  = 0.10
)

type priorityAdmissionLoadCounters struct {
	started              atomic.Int64
	prioritySucceeded    atomic.Int64
	normalSucceeded      atomic.Int64
	lowSucceeded         atomic.Int64
	priorityRejected     atomic.Int64
	normalRejected       atomic.Int64
	lowRejected          atomic.Int64
	lowWaitAttempts      atomic.Int64
	fairPriorityGranted  atomic.Int64
	fairNormalGranted    atomic.Int64
	redisErrors          atomic.Int64
	activeUpstream       atomic.Int64
	maxActiveUpstream    atomic.Int64
	maxPendingCount      atomic.Int64
	maxPendingBytes      atomic.Int64
	maxRedisPendingCount atomic.Int64
}

type priorityAdmissionResourceSample struct {
	at         time.Duration
	rssBytes   int64
	goroutines int
	processCPU float64
	redisCPU   float64
}

// TestPriorityAdmissionSustainedLoad is intentionally opt-in. The production
// acceptance invocation runs it in Docker with a dedicated Redis container,
// four CPUs, four GiB, two minutes of warm-up, and ten steady-state minutes.
func TestPriorityAdmissionSustainedLoad(t *testing.T) {
	if os.Getenv("RUN_PRIORITY_ADMISSION_LOAD_TEST") != "1" {
		t.Skip("set RUN_PRIORITY_ADMISSION_LOAD_TEST=1 to run the sustained admission test")
	}

	warmup := priorityAdmissionLoadDuration(t, "PRIORITY_ADMISSION_LOAD_WARMUP", priorityAdmissionLoadDefaultWarmup)
	steady := priorityAdmissionLoadDuration(t, "PRIORITY_ADMISSION_LOAD_STEADY", priorityAdmissionLoadDefaultSteady)
	slotHold := priorityAdmissionLoadDuration(t, "PRIORITY_ADMISSION_LOAD_SLOT_HOLD", priorityAdmissionLoadDefaultSlotHold)
	require.GreaterOrEqual(t, steady, 10*time.Second)

	rdb := priorityAdmissionLoadRedis(t)
	cache := NewConcurrencyCache(rdb, 15, 60).(*concurrencyCache)
	concurrency := service.NewConcurrencyService(cache)
	concurrency.SetPriorityAdmissionRuntimeConfig(service.PriorityAdmissionRuntimeConfig{
		Enabled:                 true,
		PendingLimitPerInstance: priorityAdmissionLoadMaxWaiting,
		PendingBytesPerInstance: service.DefaultPriorityPendingBytesPerInstance,
	})

	accountID := time.Now().UnixNano()
	userID := accountID + 1
	priorityAdmissionLoadCleanup(t, rdb, accountID, userID)

	totalDuration := warmup + steady
	runCtx, cancelRun := context.WithTimeout(context.Background(), totalDuration)
	defer cancelRun()

	var (
		counters  priorityAdmissionLoadCounters
		requests  sync.WaitGroup
		samplesMu sync.Mutex
		samples   []priorityAdmissionResourceSample
	)

	samplerDone := make(chan struct{})
	go func() {
		defer close(samplerDone)
		ticker := time.NewTicker(priorityAdmissionLoadSampleInterval)
		defer ticker.Stop()
		startedAt := time.Now()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				pending := concurrency.PriorityAdmissionPendingSnapshot()
				atomicMax(&counters.maxPendingCount, pending.TotalCount)
				atomicMax(&counters.maxPendingBytes, pending.TotalBytes)
				redisPending, err := priorityAdmissionRedisPendingCount(runCtx, rdb, accountID)
				if err != nil && runCtx.Err() == nil {
					counters.redisErrors.Add(1)
				} else {
					atomicMax(&counters.maxRedisPendingCount, redisPending)
				}
				elapsed := time.Since(startedAt)
				if elapsed < warmup {
					continue
				}
				sample, err := readPriorityAdmissionResourceSample(runCtx, rdb, elapsed-warmup)
				if err != nil {
					if runCtx.Err() == nil {
						counters.redisErrors.Add(1)
					}
					continue
				}
				samplesMu.Lock()
				samples = append(samples, sample)
				samplesMu.Unlock()
			}
		}
	}()

	batchInterval := time.Minute * priorityAdmissionLoadBatchSize / priorityAdmissionLoadRPM
	ticker := time.NewTicker(batchInterval)
	defer ticker.Stop()
	sequence := int64(0)
generate:
	for {
		select {
		case <-runCtx.Done():
			break generate
		case <-ticker.C:
			for range priorityAdmissionLoadBatchSize {
				sequence++
				tier := priorityAdmissionLoadTier(sequence)
				counters.started.Add(1)
				requests.Add(1)
				go func() {
					defer requests.Done()
					runPriorityAdmissionLoadRequest(runCtx, concurrency, accountID, userID, tier, slotHold, &counters)
				}()
			}
		}
	}

	cancelRun()
	<-samplerDone
	waitDone := make(chan struct{})
	go func() {
		requests.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(priorityAdmissionLoadRequestTimeout + 5*time.Second):
		t.Fatal("load-test requests did not drain after cancellation")
	}

	samplesMu.Lock()
	steadySamples := append([]priorityAdmissionResourceSample(nil), samples...)
	samplesMu.Unlock()
	assertPriorityAdmissionLoadResults(t, warmup, steady, counters, steadySamples)
}

func runPriorityAdmissionLoadRequest(
	runCtx context.Context,
	concurrency *service.ConcurrencyService,
	accountID int64,
	userID int64,
	tier service.RequestSchedulingTier,
	slotHold time.Duration,
	counters *priorityAdmissionLoadCounters,
) {
	requestBody := make([]byte, priorityAdmissionLoadPendingBytes)
	requestCtx, enabled := concurrency.WithPriorityAdmissionRequestSnapshot(runCtx, tier)
	if !enabled {
		counters.redisErrors.Add(1)
		return
	}

	userSlot, err := concurrency.AcquireUserSlotForTier(requestCtx, userID, priorityAdmissionLoadUserSlots, tier)
	if err != nil {
		if runCtx.Err() == nil {
			counters.redisErrors.Add(1)
		}
		return
	}
	if !userSlot.Acquired {
		priorityAdmissionLoadReject(counters, tier)
		return
	}
	defer userSlot.ReleaseFunc()

	accountSlot, err := concurrency.AcquireAccountSlotForTier(requestCtx, accountID, priorityAdmissionLoadAccountSlots, tier)
	if err != nil {
		if runCtx.Err() == nil {
			counters.redisErrors.Add(1)
		}
		return
	}
	if accountSlot.Acquired {
		priorityAdmissionLoadHoldSlot(runCtx, accountSlot, tier, slotHold, counters)
		runtime.KeepAlive(requestBody)
		return
	}
	if tier == service.RequestSchedulingTierLow {
		waiter, allowed, waitErr := concurrency.BeginPriorityAccountWaitForContext(
			requestCtx,
			accountID,
			priorityAdmissionLoadAccountSlots,
			priorityAdmissionLoadMaxWaiting,
			tier,
			int64(len(requestBody)),
			priorityAdmissionLoadRequestTimeout,
		)
		if waitErr != nil {
			if runCtx.Err() == nil {
				counters.redisErrors.Add(1)
			}
			return
		}
		if allowed || waiter != nil {
			counters.lowWaitAttempts.Add(1)
			if waiter != nil {
				waiter.Close()
			}
		}
		priorityAdmissionLoadReject(counters, tier)
		runtime.KeepAlive(requestBody)
		return
	}

	waiter, allowed, err := concurrency.BeginPriorityAccountWaitForContext(
		requestCtx,
		accountID,
		priorityAdmissionLoadAccountSlots,
		priorityAdmissionLoadMaxWaiting,
		tier,
		int64(len(requestBody)),
		priorityAdmissionLoadRequestTimeout,
	)
	if err != nil {
		if runCtx.Err() == nil {
			counters.redisErrors.Add(1)
		}
		return
	}
	if !allowed {
		priorityAdmissionLoadReject(counters, tier)
		return
	}
	defer waiter.Close()

	waitCtx, cancelWait := context.WithTimeout(requestCtx, priorityAdmissionLoadRequestTimeout)
	defer cancelWait()
	backoff := 5 * time.Millisecond
	for {
		result, status, acquireErr := waiter.TryAcquire(waitCtx)
		if acquireErr != nil {
			if !errors.Is(acquireErr, context.Canceled) && !errors.Is(acquireErr, context.DeadlineExceeded) && runCtx.Err() == nil {
				counters.redisErrors.Add(1)
			}
			priorityAdmissionLoadReject(counters, tier)
			return
		}
		if status == service.PriorityAccountAdmissionAcquired && result != nil {
			pending := concurrency.PriorityAdmissionPendingSnapshot()
			if pending.PriorityCount > 0 && pending.NormalCount > 0 {
				if tier == service.RequestSchedulingTierPriority {
					counters.fairPriorityGranted.Add(1)
				} else {
					counters.fairNormalGranted.Add(1)
				}
			}
			priorityAdmissionLoadHoldSlot(runCtx, result, tier, slotHold, counters)
			runtime.KeepAlive(requestBody)
			return
		}
		if status == service.PriorityAccountAdmissionQueueFull || status == service.PriorityAccountAdmissionRejected {
			priorityAdmissionLoadReject(counters, tier)
			return
		}

		timer := time.NewTimer(backoff)
		select {
		case <-waitCtx.Done():
			stopAndDrainPriorityAdmissionTimer(timer)
			priorityAdmissionLoadReject(counters, tier)
			return
		case <-timer.C:
		}
		if backoff < 250*time.Millisecond {
			backoff *= 2
			if backoff > 250*time.Millisecond {
				backoff = 250 * time.Millisecond
			}
		}
	}
}

func priorityAdmissionLoadHoldSlot(ctx context.Context, result *service.AcquireResult, tier service.RequestSchedulingTier, hold time.Duration, counters *priorityAdmissionLoadCounters) {
	active := counters.activeUpstream.Add(1)
	atomicMax(&counters.maxActiveUpstream, active)
	priorityAdmissionLoadSucceed(counters, tier)
	timer := time.NewTimer(hold)
	select {
	case <-ctx.Done():
		stopAndDrainPriorityAdmissionTimer(timer)
	case <-timer.C:
	}
	counters.activeUpstream.Add(-1)
	result.ReleaseFunc()
}

func stopAndDrainPriorityAdmissionTimer(timer *time.Timer) {
	if timer == nil || timer.Stop() {
		return
	}
	select {
	case <-timer.C:
	default:
	}
}

func priorityAdmissionLoadTier(sequence int64) service.RequestSchedulingTier {
	switch sequence % 10 {
	case 1:
		return service.RequestSchedulingTierPriority
	case 2:
		return service.RequestSchedulingTierNormal
	default:
		return service.RequestSchedulingTierLow
	}
}

func priorityAdmissionLoadSucceed(counters *priorityAdmissionLoadCounters, tier service.RequestSchedulingTier) {
	switch tier {
	case service.RequestSchedulingTierPriority:
		counters.prioritySucceeded.Add(1)
	case service.RequestSchedulingTierNormal:
		counters.normalSucceeded.Add(1)
	case service.RequestSchedulingTierLow:
		counters.lowSucceeded.Add(1)
	}
}

func priorityAdmissionLoadReject(counters *priorityAdmissionLoadCounters, tier service.RequestSchedulingTier) {
	switch tier {
	case service.RequestSchedulingTierPriority:
		counters.priorityRejected.Add(1)
	case service.RequestSchedulingTierNormal:
		counters.normalRejected.Add(1)
	case service.RequestSchedulingTierLow:
		counters.lowRejected.Add(1)
	}
}

func assertPriorityAdmissionLoadResults(t *testing.T, warmup, steady time.Duration, counters priorityAdmissionLoadCounters, samples []priorityAdmissionResourceSample) {
	t.Helper()
	require.Zero(t, counters.redisErrors.Load(), "Redis/admission errors must remain zero")
	require.Zero(t, counters.lowWaitAttempts.Load(), "low-tier traffic must never create a waiter")
	require.LessOrEqual(t, counters.maxActiveUpstream.Load(), int64(priorityAdmissionLoadAccountSlots))
	require.LessOrEqual(t, counters.maxPendingCount.Load(), int64(priorityAdmissionLoadMaxWaiting))
	require.LessOrEqual(t, counters.maxPendingBytes.Load(), int64(service.DefaultPriorityPendingBytesPerInstance))
	require.LessOrEqual(t, counters.maxRedisPendingCount.Load(), int64(priorityAdmissionLoadMaxWaiting))
	require.Positive(t, counters.lowRejected.Load(), "the saturated scenario must reject low-tier traffic")

	expected := int64(float64(priorityAdmissionLoadRPM) * (warmup + steady).Minutes())
	actual := counters.started.Load()
	require.InDelta(t, expected, actual, math.Max(float64(expected)*0.02, 20), "offered load must remain within 2%% of 50,000 RPM")

	priorityFair := counters.fairPriorityGranted.Load()
	normalFair := counters.fairNormalGranted.Load()
	minimumFairSamples := int64(100)
	if steady < 2*time.Minute {
		minimumFairSamples = 20
	}
	require.Greater(t, normalFair, minimumFairSamples, "not enough contended normal grants to assess fairness")
	ratio := float64(priorityFair) / float64(normalFair)
	require.InDelta(t, 4.0, ratio, 4.0*priorityAdmissionLoadFairnessMaxError, "contended grants must remain within 10%% of 4:1")

	require.GreaterOrEqual(t, len(samples), 8, "steady-state resource sampling is incomplete")
	firstWindow, lastWindow := priorityAdmissionLoadEdgeSamples(samples)
	firstRSS, firstGoroutines := averagePriorityAdmissionResources(firstWindow)
	lastRSS, lastGoroutines := averagePriorityAdmissionResources(lastWindow)
	require.LessOrEqual(t, growthRatio(firstRSS, lastRSS), priorityAdmissionLoadResourceMaxGrowth, "steady-state RSS grew by more than 5%%")
	require.LessOrEqual(t, growthRatio(firstGoroutines, lastGoroutines), priorityAdmissionLoadResourceMaxGrowth, "steady-state goroutines grew by more than 5%%")

	firstCPU, secondCPU := priorityAdmissionLoadHalfRates(samples, func(sample priorityAdmissionResourceSample) float64 { return sample.processCPU })
	require.LessOrEqual(t, growthRatio(firstCPU, secondCPU), priorityAdmissionLoadCPUMaxGrowth, "process CPU/request drifted by more than 10%%")
	firstRedisCPU, secondRedisCPU := priorityAdmissionLoadHalfRates(samples, func(sample priorityAdmissionResourceSample) float64 { return sample.redisCPU })
	require.LessOrEqual(t, growthRatio(firstRedisCPU, secondRedisCPU), priorityAdmissionLoadCPUMaxGrowth, "Redis CPU/request drifted by more than 10%%")

	t.Logf("offered=%d success(priority=%d normal=%d low=%d) rejected(priority=%d normal=%d low=%d) fairness=%d:%d max_pending=%d max_pending_bytes=%d max_active=%d rss=%d->%d goroutines=%.1f->%.1f cpu_rate=%.3f->%.3f redis_cpu_rate=%.3f->%.3f",
		actual,
		counters.prioritySucceeded.Load(), counters.normalSucceeded.Load(), counters.lowSucceeded.Load(),
		counters.priorityRejected.Load(), counters.normalRejected.Load(), counters.lowRejected.Load(),
		priorityFair, normalFair,
		counters.maxPendingCount.Load(), counters.maxPendingBytes.Load(), counters.maxActiveUpstream.Load(),
		int64(firstRSS), int64(lastRSS), firstGoroutines, lastGoroutines,
		firstCPU, secondCPU, firstRedisCPU, secondRedisCPU,
	)
}

func priorityAdmissionLoadEdgeSamples(samples []priorityAdmissionResourceSample) ([]priorityAdmissionResourceSample, []priorityAdmissionResourceSample) {
	window := len(samples) / 10
	if window < 3 {
		window = 3
	}
	if window*2 > len(samples) {
		window = len(samples) / 2
	}
	return samples[:window], samples[len(samples)-window:]
}

func averagePriorityAdmissionResources(samples []priorityAdmissionResourceSample) (float64, float64) {
	var rss, goroutines float64
	for _, sample := range samples {
		rss += float64(sample.rssBytes)
		goroutines += float64(sample.goroutines)
	}
	return rss / float64(len(samples)), goroutines / float64(len(samples))
}

func priorityAdmissionLoadHalfRates(samples []priorityAdmissionResourceSample, value func(priorityAdmissionResourceSample) float64) (float64, float64) {
	first := samples[0]
	middle := samples[len(samples)/2]
	last := samples[len(samples)-1]
	firstSeconds := middle.at.Seconds() - first.at.Seconds()
	secondSeconds := last.at.Seconds() - middle.at.Seconds()
	return (value(middle) - value(first)) / firstSeconds, (value(last) - value(middle)) / secondSeconds
}

func growthRatio(before, after float64) float64 {
	if before <= 0 || after <= before {
		return 0
	}
	return (after - before) / before
}

func readPriorityAdmissionResourceSample(ctx context.Context, rdb *redis.Client, at time.Duration) (priorityAdmissionResourceSample, error) {
	rss, err := readPriorityAdmissionRSS()
	if err != nil {
		return priorityAdmissionResourceSample{}, err
	}
	processCPU, err := readPriorityAdmissionProcessCPU()
	if err != nil {
		return priorityAdmissionResourceSample{}, err
	}
	redisCPU, err := readPriorityAdmissionRedisCPU(ctx, rdb)
	if err != nil {
		return priorityAdmissionResourceSample{}, err
	}
	return priorityAdmissionResourceSample{
		at:         at,
		rssBytes:   rss,
		goroutines: runtime.NumGoroutine(),
		processCPU: processCPU,
		redisCPU:   redisCPU,
	}, nil
}

func readPriorityAdmissionRSS() (int64, error) {
	content, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(content))
	if len(fields) < 2 {
		return 0, fmt.Errorf("unexpected /proc/self/statm format")
	}
	pages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return pages * int64(os.Getpagesize()), nil
}

func readPriorityAdmissionProcessCPU() (float64, error) {
	content, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0, err
	}
	line := string(content)
	endCommand := strings.LastIndex(line, ")")
	if endCommand < 0 {
		return 0, fmt.Errorf("unexpected /proc/self/stat format")
	}
	fields := strings.Fields(line[endCommand+1:])
	if len(fields) <= 12 {
		return 0, fmt.Errorf("unexpected /proc/self/stat field count")
	}
	userTicks, err := strconv.ParseFloat(fields[11], 64)
	if err != nil {
		return 0, err
	}
	systemTicks, err := strconv.ParseFloat(fields[12], 64)
	if err != nil {
		return 0, err
	}
	return userTicks + systemTicks, nil
}

func readPriorityAdmissionRedisCPU(ctx context.Context, rdb *redis.Client) (float64, error) {
	info, err := rdb.Info(ctx, "cpu").Result()
	if err != nil {
		return 0, err
	}
	var total float64
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "used_cpu_user:") && !strings.HasPrefix(line, "used_cpu_sys:") {
			continue
		}
		_, raw, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value, parseErr := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if parseErr != nil {
			return 0, parseErr
		}
		total += value
	}
	return total, nil
}

func priorityAdmissionLoadRedis(t *testing.T) *redis.Client {
	t.Helper()
	rawURL := os.Getenv("TEST_REDIS_URL")
	if rawURL == "" {
		t.Fatal("TEST_REDIS_URL is required for the sustained admission test")
	}
	options, err := redis.ParseURL(rawURL)
	require.NoError(t, err)
	rdb := redis.NewClient(options)
	t.Cleanup(func() { _ = rdb.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, rdb.Ping(ctx).Err())
	return rdb
}

func priorityAdmissionLoadDuration(t *testing.T, key string, fallback time.Duration) time.Duration {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	require.NoError(t, err, key)
	require.Positive(t, value, key)
	return value
}

func priorityAdmissionRedisPendingCount(ctx context.Context, rdb *redis.Client, accountID int64) (int64, error) {
	keys := priorityWaitKeys(priorityAccountWaitPrefix, accountID)
	pipe := rdb.Pipeline()
	priority := pipe.ZCard(ctx, keys.priority)
	normal := pipe.ZCard(ctx, keys.normal)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return priority.Val() + normal.Val(), nil
}

func priorityAdmissionLoadCleanup(t *testing.T, rdb *redis.Client, accountID, userID int64) {
	t.Helper()
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		accountKeys := priorityWaitKeys(priorityAccountWaitPrefix, accountID)
		userKeys := priorityWaitKeys(priorityUserWaitPrefix, userID)
		_ = rdb.Del(ctx,
			accountSlotKey(accountID), accountKeys.priority, accountKeys.normal, accountKeys.deadline, accountKeys.state,
			priorityWaitCountKey(priorityAccountWaitPrefix, accountID),
			userSlotKey(userID), userKeys.priority, userKeys.normal, userKeys.deadline, userKeys.state,
			priorityWaitCountKey(priorityUserWaitPrefix, userID),
		).Err()
		_ = rdb.ZRem(ctx, accountActiveIndexKey, accountID).Err()
		_ = rdb.ZRem(ctx, userActiveIndexKey, userID).Err()
	}
	cleanup()
	t.Cleanup(cleanup)
}

func atomicMax(target *atomic.Int64, value int64) {
	for current := target.Load(); value > current; current = target.Load() {
		if target.CompareAndSwap(current, value) {
			return
		}
	}
}
