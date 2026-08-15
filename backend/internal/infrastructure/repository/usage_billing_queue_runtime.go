package repository

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/google/uuid"
)

const (
	usageBillingWakeChannel          = "billing:usage:wake:v1"
	usageBillingConsumerLockKeyHigh  = int32(1398096434) // SUB2
	usageBillingConsumerLockKeyBase  = int32(210000000)
	usageBillingRescueLockKeyBase    = int32(220000000)
	usageBillingBatchDurationSamples = 256
	usageBillingLoadEWMAAlpha        = 0.3
	usageBillingWakeErrorLogInterval = time.Minute
)

type usageBillingQueueRuntime struct {
	instanceID string
	mode       string

	configuredConsumers int
	clusterMaxConsumers int
	rescueEnabled       bool

	effectiveConsumers atomic.Int64
	activeBatches      atomic.Int64
	slotCursor         atomic.Uint64

	enqueuedTotal         atomic.Uint64
	settledTotal          atomic.Uint64
	cleanupTotal          atomic.Uint64
	rescuedUnsettledTotal atomic.Uint64
	rescuedCleanupTotal   atomic.Uint64
	retryTotal            atomic.Uint64
	errorTotal            atomic.Uint64
	errorClassMu          sync.RWMutex
	retryClassTotals      map[string]uint64

	loadMu                sync.RWMutex
	requestLoadSource     service.UsageBillingRequestLoadSource
	workerLoadSource      service.UsageBillingWorkerLoadSource
	loadSampler           service.ClusterLoadSampler
	cpuEWMA               float64
	cpuKnown              bool
	inFlightRequests      int64
	usageWorkerBacklog    int64
	dbPoolWaitMS          float64
	processedPerSecond    float64
	lastDBWait            time.Duration
	lastProcessedTotal    uint64
	lastLoadSampleAt      time.Time
	lastScaleAt           time.Time
	readyBacklog          bool
	batchDurations        [usageBillingBatchDurationSamples]int64
	batchDurationCount    int
	batchDurationPosition int

	lastWakeErrorLog atomic.Int64
}

func newUsageBillingQueueRuntime(queueCfg config.UsageBillingQueueConfig, configuredConsumers int) *usageBillingQueueRuntime {
	mode := queueCfg.ConsumerModeResolved()
	effective := configuredConsumers
	if mode == config.UsageBillingConsumerModeStandby || mode == config.UsageBillingConsumerModeProducerOnly {
		effective = 0
	}
	runtime := &usageBillingQueueRuntime{
		instanceID:          uuid.NewString(),
		mode:                mode,
		configuredConsumers: configuredConsumers,
		clusterMaxConsumers: queueCfg.ClusterMaxConsumers,
		rescueEnabled:       queueCfg.Rescue.Enabled && mode != config.UsageBillingConsumerModeProducerOnly,
		loadSampler:         service.NewClusterLoadSampler(),
		retryClassTotals:    make(map[string]uint64),
	}
	runtime.effectiveConsumers.Store(int64(effective))
	return runtime
}

func (r *queuedUsageBillingRepository) usageBillingInstanceID() string {
	if r == nil || r.runtime == nil || r.runtime.instanceID == "" {
		return "usage-billing-legacy"
	}
	return r.runtime.instanceID
}

func (r *queuedUsageBillingRepository) retryAlertThreshold() int {
	if r == nil || r.retryAlertAttempts <= 0 {
		return 10
	}
	return r.retryAlertAttempts
}

func (r *queuedUsageBillingRepository) oldestAgeThreshold() time.Duration {
	if r == nil || r.oldestAgeAlert <= 0 {
		return 2 * time.Minute
	}
	return r.oldestAgeAlert
}

func (r *queuedUsageBillingRepository) reconcileRetryInterval() time.Duration {
	if r == nil || r.reconcileRetryDelay <= 0 {
		return 5 * time.Minute
	}
	return r.reconcileRetryDelay
}

func (r *queuedUsageBillingRepository) SetUsageBillingRequestLoadSource(source service.UsageBillingRequestLoadSource) {
	if r == nil || r.runtime == nil {
		return
	}
	r.runtime.loadMu.Lock()
	r.runtime.requestLoadSource = source
	r.runtime.loadMu.Unlock()
}

func (r *queuedUsageBillingRepository) SetUsageBillingWorkerLoadSource(source service.UsageBillingWorkerLoadSource) {
	if r == nil || r.runtime == nil {
		return
	}
	r.runtime.loadMu.Lock()
	r.runtime.workerLoadSource = source
	r.runtime.loadMu.Unlock()
}

func (r *queuedUsageBillingRepository) UsageBillingQueueNodeStats() service.UsageBillingQueueNodeStats {
	if r == nil || r.runtime == nil {
		return service.UsageBillingQueueNodeStats{CollectedAt: time.Now().UTC()}
	}
	rt := r.runtime
	rt.loadMu.RLock()
	durations := append([]int64(nil), rt.batchDurations[:rt.batchDurationCount]...)
	stats := service.UsageBillingQueueNodeStats{
		ConsumerMode:           rt.mode,
		ConfiguredConsumers:    rt.configuredConsumers,
		EffectiveConsumers:     int(rt.effectiveConsumers.Load()),
		ActiveConsumerBatches:  rt.activeBatches.Load(),
		ClusterMaxConsumers:    rt.clusterMaxConsumers,
		RescueEnabled:          rt.rescueEnabled,
		CPUUsageEWMA:           rt.cpuEWMA,
		CPUUsageKnown:          rt.cpuKnown,
		InFlightRequests:       rt.inFlightRequests,
		UsageWorkerBacklog:     rt.usageWorkerBacklog,
		DBPoolWaitMilliseconds: rt.dbPoolWaitMS,
		ReadyBacklog:           rt.readyBacklog,
		EnqueuedTotal:          rt.enqueuedTotal.Load(),
		SettledTotal:           rt.settledTotal.Load(),
		CleanupTotal:           rt.cleanupTotal.Load(),
		RescuedUnsettledTotal:  rt.rescuedUnsettledTotal.Load(),
		RescuedCleanupTotal:    rt.rescuedCleanupTotal.Load(),
		RetryTotal:             rt.retryTotal.Load(),
		ErrorTotal:             rt.errorTotal.Load(),
		ProcessedPerSecond:     rt.processedPerSecond,
		CollectedAt:            time.Now().UTC(),
	}
	rt.loadMu.RUnlock()
	rt.errorClassMu.RLock()
	stats.RetryClassTotals = make(map[string]uint64, len(rt.retryClassTotals))
	for class, total := range rt.retryClassTotals {
		stats.RetryClassTotals[class] = total
	}
	rt.errorClassMu.RUnlock()
	if len(durations) > 0 {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		index := (len(durations)*95 + 99) / 100
		if index > 0 {
			index--
		}
		stats.BatchDurationP95MS = float64(durations[index]) / float64(time.Millisecond)
	}
	return stats
}

func (r *queuedUsageBillingRepository) recordUsageBillingRetryClass(class string, count int) {
	if r == nil || r.runtime == nil || count <= 0 {
		return
	}
	class = strings.TrimSpace(class)
	if class == "" {
		class = "unknown"
	}
	r.runtime.retryTotal.Add(uint64(count))
	r.runtime.errorClassMu.Lock()
	if r.runtime.retryClassTotals == nil {
		r.runtime.retryClassTotals = make(map[string]uint64)
	}
	r.runtime.retryClassTotals[class] += uint64(count)
	r.runtime.errorClassMu.Unlock()
}

func (r *queuedUsageBillingRepository) recordUsageBillingBatch(started time.Time, processed int, rescue, cleanup bool, err error) {
	if r == nil || r.runtime == nil {
		return
	}
	rt := r.runtime
	if err != nil && !errors.Is(err, context.Canceled) {
		rt.errorTotal.Add(1)
	}
	if processed > 0 {
		if cleanup {
			rt.cleanupTotal.Add(uint64(processed))
			if rescue {
				rt.rescuedCleanupTotal.Add(uint64(processed))
			}
		} else {
			rt.settledTotal.Add(uint64(processed))
			if rescue {
				rt.rescuedUnsettledTotal.Add(uint64(processed))
			}
		}
	}
	duration := time.Since(started).Nanoseconds()
	rt.loadMu.Lock()
	rt.batchDurations[rt.batchDurationPosition] = duration
	rt.batchDurationPosition = (rt.batchDurationPosition + 1) % len(rt.batchDurations)
	if rt.batchDurationCount < len(rt.batchDurations) {
		rt.batchDurationCount++
	}
	rt.loadMu.Unlock()
}

func (r *queuedUsageBillingRepository) runLoadController(ctx context.Context) {
	defer r.wg.Done()
	interval := r.autoScaleInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	r.sampleAndAdjustUsageBillingLoad(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.sampleAndAdjustUsageBillingLoad(ctx)
		}
	}
}

func (r *queuedUsageBillingRepository) sampleAndAdjustUsageBillingLoad(ctx context.Context) {
	if r == nil || r.runtime == nil {
		return
	}
	rt := r.runtime
	now := time.Now()
	var cpuPercent float64
	cpuKnown := false
	if rt.loadSampler != nil {
		load := rt.loadSampler.Sample(ctx)
		if load.CPUUsagePercent != nil {
			cpuPercent = *load.CPUUsagePercent
			cpuKnown = true
		}
	}
	rt.loadMu.RLock()
	requestSource := rt.requestLoadSource
	workerSource := rt.workerLoadSource
	rt.loadMu.RUnlock()
	var inFlight, workerBacklog int64
	if requestSource != nil {
		inFlight = max(int64(0), requestSource.InFlightRequests())
	}
	if workerSource != nil {
		workerBacklog = max(int64(0), workerSource.UsageBillingWorkerBacklog())
	}
	dbStats := sql.DBStats{}
	if r.db != nil {
		dbStats = r.db.Stats()
	}
	// Keep a cheap durable signal for auto mode. Process-local load sources can
	// all be quiet while a node has already scaled to zero consumers; ready rows
	// are the only signal that can wake it back up without relying on Redis.
	readyBacklog := false
	if rt.mode == config.UsageBillingConsumerModeAuto && r.db != nil {
		backlogCtx, cancel := context.WithTimeout(ctx, min(r.commandTimeout, 500*time.Millisecond))
		if err := r.db.QueryRowContext(backlogCtx, `
			SELECT EXISTS (
				SELECT 1
				FROM usage_billing_jobs
				WHERE available_at <= NOW()
			)
		`).Scan(&readyBacklog); err != nil {
			readyBacklog = false
		}
		cancel()
	}

	rt.loadMu.Lock()
	if cpuKnown {
		if rt.lastLoadSampleAt.IsZero() || !rt.cpuKnown {
			rt.cpuEWMA = cpuPercent
		} else {
			rt.cpuEWMA = usageBillingLoadEWMAAlpha*cpuPercent + (1-usageBillingLoadEWMAAlpha)*rt.cpuEWMA
		}
		rt.cpuKnown = true
	}
	dbWaitDelta := time.Duration(0)
	if !rt.lastLoadSampleAt.IsZero() {
		dbWaitDelta = dbStats.WaitDuration - rt.lastDBWait
		if dbWaitDelta < 0 {
			dbWaitDelta = 0
		}
	}
	rt.lastDBWait = dbStats.WaitDuration
	rt.dbPoolWaitMS = float64(dbWaitDelta) / float64(time.Millisecond)
	processedTotal := rt.settledTotal.Load() + rt.cleanupTotal.Load()
	if !rt.lastLoadSampleAt.IsZero() {
		elapsed := now.Sub(rt.lastLoadSampleAt).Seconds()
		if elapsed > 0 {
			instant := float64(processedTotal-rt.lastProcessedTotal) / elapsed
			if rt.processedPerSecond == 0 {
				rt.processedPerSecond = instant
			} else {
				rt.processedPerSecond = usageBillingLoadEWMAAlpha*instant + (1-usageBillingLoadEWMAAlpha)*rt.processedPerSecond
			}
		}
	}
	rt.lastProcessedTotal = processedTotal
	rt.lastLoadSampleAt = now
	rt.inFlightRequests = inFlight
	rt.usageWorkerBacklog = workerBacklog
	rt.readyBacklog = readyBacklog

	if rt.mode == config.UsageBillingConsumerModeAuto && (rt.lastScaleAt.IsZero() || now.Sub(rt.lastScaleAt) >= r.autoScaleCooldown) {
		current := int(rt.effectiveConsumers.Load())
		highLoad := inFlight >= r.autoInFlightHigh ||
			workerBacklog >= r.autoUsageWorkerBacklogHigh ||
			rt.dbPoolWaitMS >= float64(r.autoDBPoolWaitHigh.Milliseconds())
		lowLoad := inFlight < max(int64(1), r.autoInFlightHigh/2) &&
			workerBacklog < max(int64(1), r.autoUsageWorkerBacklogHigh/2) &&
			rt.dbPoolWaitMS < float64(r.autoDBPoolWaitHigh.Milliseconds())/2
		if rt.cpuKnown {
			highLoad = highLoad || rt.cpuEWMA >= r.autoCPUHighPercent
			lowLoad = lowLoad && rt.cpuEWMA <= r.autoCPULowPercent
		}
		// A ready durable job is a positive demand signal. It prevents an
		// explicitly configured min_consumers=0 from leaving every auto node
		// asleep after a quiet period.
		if readyBacklog {
			lowLoad = false
		}
		switch {
		case highLoad && current > r.autoMinConsumers && !readyBacklog:
			rt.effectiveConsumers.Store(int64(current - 1))
			rt.lastScaleAt = now
		case readyBacklog && (current == 0 || !highLoad) && current < max(1, r.consumerCount):
			rt.effectiveConsumers.Store(int64(current + 1))
			rt.lastScaleAt = now
			r.wakeConsumers()
		case lowLoad && current < r.consumerCount:
			rt.effectiveConsumers.Store(int64(current + 1))
			rt.lastScaleAt = now
			r.wakeConsumers()
		}
	}
	rt.loadMu.Unlock()
}

func (r *queuedUsageBillingRepository) consumerAllowed(workerID int) bool {
	if r == nil || workerID < 0 {
		return false
	}
	if r.runtime == nil {
		return workerID < r.consumerCount
	}
	return workerID < int(r.runtime.effectiveConsumers.Load())
}

// rescueAllowed keeps the low-priority stale-job path from adding database
// pressure while an API node or its usage worker is already saturated. A
// standby node has no normal consumers but remains eligible while idle.
func (r *queuedUsageBillingRepository) rescueAllowed() bool {
	if r == nil || r.runtime == nil {
		return true
	}
	r.runtime.loadMu.RLock()
	defer r.runtime.loadMu.RUnlock()
	return (!r.runtime.cpuKnown || r.runtime.cpuEWMA < r.autoCPUHighPercent) &&
		r.runtime.inFlightRequests < r.autoInFlightHigh &&
		r.runtime.usageWorkerBacklog < r.autoUsageWorkerBacklogHigh &&
		r.runtime.dbPoolWaitMS < float64(r.autoDBPoolWaitHigh.Milliseconds())
}

func (r *queuedUsageBillingRepository) tryAcquireUsageBillingClusterSlot(ctx context.Context, tx *sql.Tx) (bool, error) {
	if r == nil || tx == nil {
		return false, nil
	}
	limit := r.clusterMaxConsumers
	base := usageBillingConsumerLockKeyBase
	if limit <= 0 {
		return true, nil
	}
	start := 0
	if r.runtime != nil {
		start = int(r.runtime.slotCursor.Add(1) % uint64(limit))
	}
	var slot sql.NullInt64
	err := tx.QueryRowContext(ctx, `
		SELECT ((slot + $4) % $1)::bigint
		FROM generate_series(0, $1 - 1) AS slots(slot)
		WHERE pg_try_advisory_xact_lock($2, $3 + ((slot + $4) % $1))
		LIMIT 1
	`, limit, usageBillingConsumerLockKeyHigh, base, start).Scan(&slot)
	if errors.Is(err, sql.ErrNoRows) {
		// A full cluster budget is expected backpressure, not a database
		// failure. The caller will wait for the next wake/poll cycle.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return slot.Valid, nil
}

type usageBillingTxBeginner interface {
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

// tryAcquireUsageBillingRescueSlot holds a session advisory lock for the
// complete rescue cycle, including Redis overlay cleanup. Transactions run on
// the same connection so the slot does not consume an extra pool connection.
func (r *queuedUsageBillingRepository) tryAcquireUsageBillingRescueSlot(ctx context.Context) (*sql.Conn, func(), bool, error) {
	if r == nil || r.db == nil {
		return nil, nil, false, nil
	}
	limit := r.rescueClusterMaxConcurrency
	if limit <= 0 {
		return nil, func() {}, true, nil
	}
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return nil, nil, false, err
	}
	start := 0
	if r.runtime != nil {
		start = int(r.runtime.slotCursor.Add(1) % uint64(limit))
	}
	var acquiredSlot sql.NullInt64
	if err := conn.QueryRowContext(ctx, `
		SELECT ((slot + $4) % $1)::bigint
		FROM generate_series(0, $1 - 1) AS slots(slot)
		WHERE pg_try_advisory_lock($2, $3 + ((slot + $4) % $1))
		LIMIT 1
	`, limit, usageBillingConsumerLockKeyHigh, usageBillingRescueLockKeyBase, start).Scan(&acquiredSlot); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = conn.Close()
			return nil, nil, false, nil
		}
		_ = conn.Close()
		return nil, nil, false, err
	}
	if acquiredSlot.Valid {
		slot := int(acquiredSlot.Int64)
		release := func() {
			unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var unlocked bool
			if err := conn.QueryRowContext(
				unlockCtx,
				"SELECT pg_advisory_unlock($1, $2)",
				usageBillingConsumerLockKeyHigh,
				usageBillingRescueLockKeyBase+int32(slot),
			).Scan(&unlocked); err != nil {
				slog.Warn("release durable usage billing rescue slot failed", "slot", slot, "error", err)
			}
			_ = conn.Close()
		}
		return conn, release, true, nil
	}
	_ = conn.Close()
	return nil, nil, false, nil
}

func (r *queuedUsageBillingRepository) notifyConsumersAfterCommit(inserted int) {
	if r == nil || inserted <= 0 {
		return
	}
	if r.runtime != nil {
		r.runtime.enqueuedTotal.Add(uint64(inserted))
	}
}

func (r *queuedUsageBillingRepository) signalConsumersAfterCommit() {
	if r == nil {
		return
	}
	r.wakeConsumers()
	if !r.pubSubWakeupEnabled || r.rdb == nil || r.wakePublishCh == nil {
		return
	}
	select {
	case r.wakePublishCh <- struct{}{}:
	default:
	}
}

func (r *queuedUsageBillingRepository) runWakePublisher(ctx context.Context) {
	defer r.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.wakePublishCh:
			publishCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			err := r.rdb.Publish(publishCtx, usageBillingWakeChannel, r.usageBillingInstanceID()).Err()
			cancel()
			if err != nil && ctx.Err() == nil {
				r.logUsageBillingWakeError("publish", err)
			}
		}
	}
}

func (r *queuedUsageBillingRepository) runWakeSubscriber(ctx context.Context) {
	defer r.wg.Done()
	for ctx.Err() == nil {
		pubsub := r.rdb.Subscribe(ctx, usageBillingWakeChannel)
		_, err := pubsub.Receive(ctx)
		if err == nil {
			for err == nil && ctx.Err() == nil {
				_, err = pubsub.ReceiveMessage(ctx)
				if err == nil {
					r.wakeConsumers()
				}
			}
		}
		_ = pubsub.Close()
		if err != nil && ctx.Err() == nil {
			r.logUsageBillingWakeError("subscribe", err)
		}
		timer := time.NewTimer(min(time.Second, r.pollInterval))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (r *queuedUsageBillingRepository) logUsageBillingWakeError(operation string, err error) {
	if r == nil || r.runtime == nil || err == nil {
		return
	}
	now := time.Now().UnixNano()
	last := r.runtime.lastWakeErrorLog.Load()
	if last != 0 && time.Duration(now-last) < usageBillingWakeErrorLogInterval {
		return
	}
	if r.runtime.lastWakeErrorLog.CompareAndSwap(last, now) {
		slog.Warn("durable usage billing Redis wakeup failed; PostgreSQL polling remains active", "operation", operation, "error", err)
	}
}

var _ service.UsageBillingQueueLoadSink = (*queuedUsageBillingRepository)(nil)
var _ service.UsageBillingQueueLoadSource = (*queuedUsageBillingRepository)(nil)
