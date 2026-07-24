package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
)

const (
	DefaultPriorityPendingLimitPerInstance int   = 256
	DefaultPriorityPendingBytesPerInstance int64 = 256 << 20
	priorityAdmissionOperationTimeout            = 5 * time.Second
	priorityAdmissionCleanupTimeout              = 500 * time.Millisecond
)

var (
	ErrPriorityAdmissionDisabled    = errors.New("request priority admission is disabled")
	ErrPriorityAdmissionUnavailable = errors.New("request priority admission is unavailable")
)

// PriorityAdmissionRuntimeConfig is an immutable, process-local snapshot. It
// is updated from persisted gateway settings and is safe to read on every
// request without touching Redis or the database.
type PriorityAdmissionRuntimeConfig struct {
	Enabled                 bool
	PendingLimitPerInstance int
	PendingBytesPerInstance int64
}

type priorityAdmissionRuntimeConfig struct {
	enabled                 bool
	pendingLimitPerInstance int64
	pendingBytesPerInstance int64
}

type priorityAdmissionRequestSnapshot struct {
	config            priorityAdmissionRuntimeConfig
	baseContext       context.Context
	lowAccountAttempt atomic.Bool
}

type priorityAdmissionRequestSnapshotContextKey struct{}
type priorityAdmissionLoadCountsContextKey struct{}

func withPriorityAdmissionLoadCounts(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, priorityAdmissionLoadCountsContextKey{}, true)
}

// PriorityAdmissionLoadCountsEnabled lets the Redis repository include the
// independent priority wait counters only on feature-enabled requests.
func PriorityAdmissionLoadCountsEnabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	enabled, _ := ctx.Value(priorityAdmissionLoadCountsContextKey{}).(bool)
	return enabled
}

// WithPriorityAdmissionRequestSnapshot captures the admission configuration
// once for a gateway request. Later user/account stages keep using this value
// even when an administrator changes the global switch in the meantime.
func (s *ConcurrencyService) WithPriorityAdmissionRequestSnapshot(ctx context.Context, tier RequestSchedulingTier) (context.Context, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if snapshot, ok := ctx.Value(priorityAdmissionRequestSnapshotContextKey{}).(*priorityAdmissionRequestSnapshot); ok && snapshot != nil {
		return ctx, snapshot.config.enabled
	}
	config := s.priorityAdmissionConfigSnapshot()
	if !config.enabled {
		return ctx, false
	}
	baseContext := ctx
	ctx = WithRequestSchedulingTier(baseContext, tier)
	ctx = context.WithValue(ctx, priorityAdmissionRequestSnapshotContextKey{}, &priorityAdmissionRequestSnapshot{config: config, baseContext: baseContext})
	return ctx, true
}

// RefreshPriorityAdmissionRequestSnapshot starts a new admission attempt scope
// on long-lived transports such as WebSockets while preserving the parent
// cancellation/deadline context.
func (s *ConcurrencyService) RefreshPriorityAdmissionRequestSnapshot(ctx context.Context, tier RequestSchedulingTier) (context.Context, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if previous, ok := ctx.Value(priorityAdmissionRequestSnapshotContextKey{}).(*priorityAdmissionRequestSnapshot); ok && previous != nil && previous.baseContext != nil {
		ctx = previous.baseContext
	}
	config := s.priorityAdmissionConfigSnapshot()
	baseContext := ctx
	ctx = WithRequestSchedulingTier(baseContext, tier)
	ctx = context.WithValue(ctx, priorityAdmissionRequestSnapshotContextKey{}, &priorityAdmissionRequestSnapshot{config: config, baseContext: baseContext})
	return ctx, config.enabled
}

func (s *ConcurrencyService) priorityAdmissionRequestConfig(ctx context.Context) (priorityAdmissionRuntimeConfig, *priorityAdmissionRequestSnapshot) {
	if ctx != nil {
		if snapshot, ok := ctx.Value(priorityAdmissionRequestSnapshotContextKey{}).(*priorityAdmissionRequestSnapshot); ok && snapshot != nil {
			return snapshot.config, snapshot
		}
	}
	return s.priorityAdmissionConfigSnapshot(), nil
}

func (s *ConcurrencyService) PriorityAdmissionEnabledForRequest(ctx context.Context) bool {
	config, _ := s.priorityAdmissionRequestConfig(ctx)
	return config.enabled
}

func DefaultPriorityAdmissionRuntimeConfig() PriorityAdmissionRuntimeConfig {
	return PriorityAdmissionRuntimeConfig{
		Enabled:                 false,
		PendingLimitPerInstance: DefaultPriorityPendingLimitPerInstance,
		PendingBytesPerInstance: DefaultPriorityPendingBytesPerInstance,
	}
}

func (s *ConcurrencyService) SetPriorityAdmissionRuntimeConfig(config PriorityAdmissionRuntimeConfig) {
	if s == nil {
		return
	}
	if config.PendingLimitPerInstance <= 0 {
		config.PendingLimitPerInstance = DefaultPriorityPendingLimitPerInstance
	}
	if config.PendingBytesPerInstance <= 0 {
		config.PendingBytesPerInstance = DefaultPriorityPendingBytesPerInstance
	}
	s.priorityAdmissionConfig.Store(&priorityAdmissionRuntimeConfig{
		enabled:                 config.Enabled,
		pendingLimitPerInstance: int64(config.PendingLimitPerInstance),
		pendingBytesPerInstance: config.PendingBytesPerInstance,
	})
}

// ApplyRequestPriorityAdmissionSettings is the sink adapter used by
// SettingService. Settings publish only on startup/update; the request path
// reads ConcurrencyService's own atomic snapshot.
func (s *ConcurrencyService) ApplyRequestPriorityAdmissionSettings(settings RequestPriorityAdmissionSettings) {
	s.SetPriorityAdmissionRuntimeConfig(PriorityAdmissionRuntimeConfig{
		Enabled:                 settings.Enabled,
		PendingLimitPerInstance: settings.PendingLimitPerInstance,
		PendingBytesPerInstance: settings.PendingBytesPerInstance(),
	})
}

func (s *ConcurrencyService) PriorityAdmissionRuntimeConfig() PriorityAdmissionRuntimeConfig {
	config := s.priorityAdmissionConfigSnapshot()
	return PriorityAdmissionRuntimeConfig{
		Enabled:                 config.enabled,
		PendingLimitPerInstance: int(config.pendingLimitPerInstance),
		PendingBytesPerInstance: config.pendingBytesPerInstance,
	}
}

func (s *ConcurrencyService) PriorityAdmissionEnabled() bool {
	return s != nil && s.priorityAdmissionConfigSnapshot().enabled
}

func (s *ConcurrencyService) priorityAdmissionConfigSnapshot() priorityAdmissionRuntimeConfig {
	if s != nil {
		if config := s.priorityAdmissionConfig.Load(); config != nil {
			return *config
		}
	}
	defaults := DefaultPriorityAdmissionRuntimeConfig()
	return priorityAdmissionRuntimeConfig{
		enabled:                 defaults.Enabled,
		pendingLimitPerInstance: int64(defaults.PendingLimitPerInstance),
		pendingBytesPerInstance: defaults.PendingBytesPerInstance,
	}
}

type PriorityAdmissionPendingSnapshot struct {
	PriorityCount int64
	NormalCount   int64
	TotalCount    int64
	PriorityBytes int64
	NormalBytes   int64
	TotalBytes    int64
}

func (s *ConcurrencyService) PriorityAdmissionPendingSnapshot() PriorityAdmissionPendingSnapshot {
	if s == nil {
		return PriorityAdmissionPendingSnapshot{}
	}
	s.priorityPendingMu.Lock()
	defer s.priorityPendingMu.Unlock()
	return PriorityAdmissionPendingSnapshot{
		PriorityCount: s.priorityPendingCount[0],
		NormalCount:   s.priorityPendingCount[1],
		TotalCount:    s.priorityPendingCount[0] + s.priorityPendingCount[1],
		PriorityBytes: s.priorityPendingBytes[0],
		NormalBytes:   s.priorityPendingBytes[1],
		TotalBytes:    s.priorityPendingBytes[0] + s.priorityPendingBytes[1],
	}
}

type priorityPendingReservation struct {
	service *ConcurrencyService
	tier    int
	bytes   int64
	once    sync.Once
}

func (s *ConcurrencyService) reservePriorityPending(config priorityAdmissionRuntimeConfig, tier RequestSchedulingTier, pendingBytes int64) (*priorityPendingReservation, bool) {
	if s == nil || !config.enabled || tier == RequestSchedulingTierLow {
		return nil, false
	}
	if !tier.Valid() {
		tier = RequestSchedulingTierNormal
	}
	tierIndex := 1
	if tier == RequestSchedulingTierPriority {
		tierIndex = 0
	}
	if pendingBytes < 0 {
		pendingBytes = 0
	}
	tierCountLimit := threeQuarterFloor(config.pendingLimitPerInstance)
	tierBytesLimit := threeQuarterFloor(config.pendingBytesPerInstance)

	s.priorityPendingMu.Lock()
	defer s.priorityPendingMu.Unlock()
	totalCount := s.priorityPendingCount[0] + s.priorityPendingCount[1]
	totalBytes := s.priorityPendingBytes[0] + s.priorityPendingBytes[1]
	if totalCount >= config.pendingLimitPerInstance ||
		s.priorityPendingCount[tierIndex] >= tierCountLimit ||
		pendingBytes > config.pendingBytesPerInstance-totalBytes ||
		pendingBytes > tierBytesLimit-s.priorityPendingBytes[tierIndex] {
		return nil, false
	}
	s.priorityPendingCount[tierIndex]++
	s.priorityPendingBytes[tierIndex] += pendingBytes
	return &priorityPendingReservation{service: s, tier: tierIndex, bytes: pendingBytes}, true
}

func threeQuarterFloor(value int64) int64 {
	if value <= 0 {
		return 0
	}
	reservedQuarter := value / 4
	if value%4 != 0 {
		reservedQuarter++
	}
	return value - reservedQuarter
}

func (r *priorityPendingReservation) Release() {
	if r == nil || r.service == nil {
		return
	}
	r.once.Do(func() {
		s := r.service
		s.priorityPendingMu.Lock()
		if s.priorityPendingCount[r.tier] > 0 {
			s.priorityPendingCount[r.tier]--
		}
		if s.priorityPendingBytes[r.tier] >= r.bytes {
			s.priorityPendingBytes[r.tier] -= r.bytes
		} else {
			s.priorityPendingBytes[r.tier] = 0
		}
		s.priorityPendingMu.Unlock()
	})
}

type PriorityAccountAdmissionStatus int64

const (
	PriorityAccountAdmissionRejected PriorityAccountAdmissionStatus = iota
	PriorityAccountAdmissionAcquired
	PriorityAccountAdmissionWaiting
	PriorityAccountAdmissionQueueFull
)

// PriorityAccountAdmissionRequest is consumed atomically by the Redis cache.
// Register=false is the scheduler fast path: it may take an idle slot but can
// never jump ahead of an existing queue.
type PriorityAccountAdmissionRequest struct {
	AccountID      int64
	MaxConcurrency int
	MaxWaiting     int
	Tier           RequestSchedulingTier
	RequestID      string
	WaitTimeout    time.Duration
	Register       bool
}

type PriorityUserAdmissionRequest struct {
	UserID         int64
	MaxConcurrency int
	MaxWaiting     int
	Tier           RequestSchedulingTier
	RequestID      string
	WaitTimeout    time.Duration
	Register       bool
}

type PriorityAdmissionCache interface {
	AcquirePriorityAccountSlot(ctx context.Context, request PriorityAccountAdmissionRequest) (PriorityAccountAdmissionStatus, error)
	CancelPriorityAccountWait(ctx context.Context, accountID int64, requestID string) error
	AcquirePriorityUserSlot(ctx context.Context, request PriorityUserAdmissionRequest) (PriorityAccountAdmissionStatus, error)
	CancelPriorityUserWait(ctx context.Context, userID int64, requestID string) error
}

func (s *ConcurrencyService) priorityAdmissionCache() (PriorityAdmissionCache, error) {
	if s == nil || s.cache == nil {
		return nil, ErrPriorityAdmissionUnavailable
	}
	cache, ok := s.cache.(PriorityAdmissionCache)
	if !ok {
		return nil, fmt.Errorf("%w: cache does not support priority admission", ErrPriorityAdmissionUnavailable)
	}
	return cache, nil
}

func priorityAdmissionCacheError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrPriorityAdmissionUnavailable, err)
}

// AcquireAccountSlotForTier is the non-queuing scheduler fast path. When the
// feature is disabled it is exactly the legacy acquisition. When enabled it
// refuses to bypass already queued requests and fails closed on Redis errors.
func (s *ConcurrencyService) AcquireAccountSlotForTier(ctx context.Context, accountID int64, maxConcurrency int, tier RequestSchedulingTier) (*AcquireResult, error) {
	config, requestSnapshot := s.priorityAdmissionRequestConfig(ctx)
	if s == nil || !config.enabled {
		return s.acquireAccountSlotLegacy(ctx, accountID, maxConcurrency)
	}
	if maxConcurrency <= 0 {
		return &AcquireResult{Acquired: true, ReleaseFunc: func() {}}, nil
	}
	if !tier.Valid() {
		tier = RequestSchedulingTierNormal
	}
	if tier == RequestSchedulingTierLow && requestSnapshot != nil && !requestSnapshot.lowAccountAttempt.CompareAndSwap(false, true) {
		return priorityAdmissionTerminalAcquireResult, nil
	}
	cache, err := s.priorityAdmissionCache()
	if err != nil {
		return nil, err
	}
	requestID := generateRequestID()
	status, err := cache.AcquirePriorityAccountSlot(ctx, PriorityAccountAdmissionRequest{
		AccountID:      accountID,
		MaxConcurrency: maxConcurrency,
		Tier:           tier,
		RequestID:      requestID,
		Register:       false,
	})
	if err != nil {
		return nil, priorityAdmissionCacheError(err)
	}
	if status != PriorityAccountAdmissionAcquired {
		if tier == RequestSchedulingTierLow {
			return priorityAdmissionTerminalAcquireResult, nil
		}
		return rejectedAcquireResult, nil
	}
	return s.acquiredPriorityAccountResult(accountID, requestID), nil
}

func (s *ConcurrencyService) acquiredPriorityAccountResult(accountID int64, requestID string) *AcquireResult {
	return &AcquireResult{
		Acquired: true,
		ReleaseFunc: func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), priorityAdmissionOperationTimeout)
			defer cancel()
			if err := s.cache.ReleaseAccountSlot(bgCtx, accountID, requestID); err != nil {
				logger.LegacyPrintf("service.concurrency", "Warning: failed to release priority account slot for %d (req=%s): %v", accountID, requestID, err)
			}
		},
	}
}

// AcquireUserSlotForTier preserves the existing user-slot algorithm but makes
// Redis availability mandatory while priority admission is enabled. Tier
// affects waiting policy in BeginPriorityUserWait; the immediate slot itself
// remains shared by all tiers.
func (s *ConcurrencyService) AcquireUserSlotForTier(ctx context.Context, userID int64, maxConcurrency int, tier RequestSchedulingTier) (*AcquireResult, error) {
	config, _ := s.priorityAdmissionRequestConfig(ctx)
	if s == nil || !config.enabled {
		return s.acquireUserSlotLegacy(ctx, userID, maxConcurrency)
	}
	if maxConcurrency <= 0 {
		return &AcquireResult{Acquired: true, ReleaseFunc: func() {}}, nil
	}
	if !tier.Valid() {
		tier = RequestSchedulingTierNormal
	}
	cache, err := s.priorityAdmissionCache()
	if err != nil {
		return nil, err
	}
	requestID := generateRequestID()
	status, err := cache.AcquirePriorityUserSlot(ctx, PriorityUserAdmissionRequest{
		UserID:         userID,
		MaxConcurrency: maxConcurrency,
		Tier:           tier,
		RequestID:      requestID,
		Register:       false,
	})
	if err != nil {
		return nil, priorityAdmissionCacheError(err)
	}
	if status != PriorityAccountAdmissionAcquired {
		return rejectedAcquireResult, nil
	}
	return s.acquiredPriorityUserResult(userID, requestID), nil
}

func (s *ConcurrencyService) acquiredPriorityUserResult(userID int64, requestID string) *AcquireResult {
	return &AcquireResult{
		Acquired: true,
		ReleaseFunc: func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), priorityAdmissionOperationTimeout)
			defer cancel()
			if err := s.cache.ReleaseUserSlot(bgCtx, userID, requestID); err != nil {
				logger.LegacyPrintf("service.concurrency", "Warning: failed to release priority user slot for %d (req=%s): %v", userID, requestID, err)
			}
		},
	}
}

// PriorityAccountWaiter owns both the per-instance memory/count reservation
// and the distributed queue member. Close must be called on every exit path.
type PriorityAccountWaiter struct {
	service     *ConcurrencyService
	cache       PriorityAdmissionCache
	request     PriorityAccountAdmissionRequest
	reservation *priorityPendingReservation
	closed      atomic.Bool
	acquired    atomic.Bool
}

func (s *ConcurrencyService) BeginPriorityAccountWait(accountID int64, maxConcurrency int, maxWaiting int, tier RequestSchedulingTier, pendingBytes int64, timeout time.Duration) (*PriorityAccountWaiter, bool, error) {
	return s.BeginPriorityAccountWaitForContext(context.Background(), accountID, maxConcurrency, maxWaiting, tier, pendingBytes, timeout)
}

func (s *ConcurrencyService) BeginPriorityAccountWaitForContext(ctx context.Context, accountID int64, maxConcurrency int, maxWaiting int, tier RequestSchedulingTier, pendingBytes int64, timeout time.Duration) (*PriorityAccountWaiter, bool, error) {
	config, _ := s.priorityAdmissionRequestConfig(ctx)
	if s == nil || !config.enabled {
		return nil, false, ErrPriorityAdmissionDisabled
	}
	if !tier.Valid() {
		tier = RequestSchedulingTierNormal
	}
	if tier == RequestSchedulingTierLow {
		return nil, false, nil
	}
	cache, err := s.priorityAdmissionCache()
	if err != nil {
		return nil, false, err
	}
	reservation, ok := s.reservePriorityPending(config, tier, pendingBytes)
	if !ok {
		return nil, false, nil
	}
	if maxWaiting <= 0 || timeout <= 0 {
		reservation.Release()
		return nil, false, nil
	}
	return &PriorityAccountWaiter{
		service: s,
		cache:   cache,
		request: PriorityAccountAdmissionRequest{
			AccountID:      accountID,
			MaxConcurrency: maxConcurrency,
			MaxWaiting:     maxWaiting,
			Tier:           tier,
			RequestID:      generateRequestID(),
			WaitTimeout:    timeout,
			Register:       true,
		},
		reservation: reservation,
	}, true, nil
}

func (w *PriorityAccountWaiter) TryAcquire(ctx context.Context) (*AcquireResult, PriorityAccountAdmissionStatus, error) {
	if w == nil || w.closed.Load() {
		return nil, PriorityAccountAdmissionRejected, context.Canceled
	}
	status, err := w.cache.AcquirePriorityAccountSlot(ctx, w.request)
	if err != nil {
		w.Close()
		return nil, PriorityAccountAdmissionRejected, priorityAdmissionCacheError(err)
	}
	if status == PriorityAccountAdmissionAcquired {
		w.acquired.Store(true)
		w.reservation.Release()
		return w.service.acquiredPriorityAccountResult(w.request.AccountID, w.request.RequestID), status, nil
	}
	if status == PriorityAccountAdmissionQueueFull || status == PriorityAccountAdmissionRejected {
		w.closeWithoutRemoteCleanup()
	}
	return nil, status, nil
}

func (w *PriorityAccountWaiter) closeWithoutRemoteCleanup() {
	if w == nil || !w.closed.CompareAndSwap(false, true) {
		return
	}
	w.reservation.Release()
}

func (w *PriorityAccountWaiter) Close() {
	if w == nil || !w.closed.CompareAndSwap(false, true) {
		return
	}
	w.reservation.Release()
	if w.acquired.Load() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), priorityAdmissionCleanupTimeout)
	defer cancel()
	if err := w.cache.CancelPriorityAccountWait(ctx, w.request.AccountID, w.request.RequestID); err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: failed to cancel priority account wait for %d (req=%s): %v", w.request.AccountID, w.request.RequestID, err)
	}
}

type PriorityUserWaitLease struct {
	service     *ConcurrencyService
	cache       PriorityAdmissionCache
	request     PriorityUserAdmissionRequest
	reservation *priorityPendingReservation
	closed      atomic.Bool
	acquired    atomic.Bool
}

// BeginPriorityUserWait creates a distributed user-slot queue member backed by
// the same atomic 4:1 state machine as account admission.
func (s *ConcurrencyService) BeginPriorityUserWait(userID int64, maxConcurrency int, maxWaiting int, tier RequestSchedulingTier, pendingBytes int64, timeout time.Duration) (*PriorityUserWaitLease, bool, error) {
	return s.BeginPriorityUserWaitForContext(context.Background(), userID, maxConcurrency, maxWaiting, tier, pendingBytes, timeout)
}

func (s *ConcurrencyService) BeginPriorityUserWaitForContext(ctx context.Context, userID int64, maxConcurrency int, maxWaiting int, tier RequestSchedulingTier, pendingBytes int64, timeout time.Duration) (*PriorityUserWaitLease, bool, error) {
	config, _ := s.priorityAdmissionRequestConfig(ctx)
	if s == nil || !config.enabled {
		return nil, false, ErrPriorityAdmissionDisabled
	}
	if !tier.Valid() {
		tier = RequestSchedulingTierNormal
	}
	if tier == RequestSchedulingTierLow {
		return nil, false, nil
	}
	cache, err := s.priorityAdmissionCache()
	if err != nil {
		return nil, false, err
	}
	reservation, ok := s.reservePriorityPending(config, tier, pendingBytes)
	if !ok {
		return nil, false, nil
	}
	if maxWaiting <= 0 || timeout <= 0 {
		reservation.Release()
		return nil, false, nil
	}
	return &PriorityUserWaitLease{
		service: s,
		cache:   cache,
		request: PriorityUserAdmissionRequest{
			UserID:         userID,
			MaxConcurrency: maxConcurrency,
			MaxWaiting:     maxWaiting,
			Tier:           tier,
			RequestID:      generateRequestID(),
			WaitTimeout:    timeout,
			Register:       true,
		},
		reservation: reservation,
	}, true, nil
}

func (l *PriorityUserWaitLease) TryAcquire(ctx context.Context) (*AcquireResult, PriorityAccountAdmissionStatus, error) {
	if l == nil || l.closed.Load() {
		return nil, PriorityAccountAdmissionRejected, context.Canceled
	}
	status, err := l.cache.AcquirePriorityUserSlot(ctx, l.request)
	if err != nil {
		l.Close()
		return nil, PriorityAccountAdmissionRejected, priorityAdmissionCacheError(err)
	}
	if status == PriorityAccountAdmissionAcquired {
		l.acquired.Store(true)
		l.reservation.Release()
		return l.service.acquiredPriorityUserResult(l.request.UserID, l.request.RequestID), status, nil
	}
	if status == PriorityAccountAdmissionQueueFull || status == PriorityAccountAdmissionRejected {
		l.closeWithoutRemoteCleanup()
	}
	return nil, status, nil
}

func (l *PriorityUserWaitLease) closeWithoutRemoteCleanup() {
	if l == nil || !l.closed.CompareAndSwap(false, true) {
		return
	}
	l.reservation.Release()
}

func (l *PriorityUserWaitLease) Close() {
	if l == nil || !l.closed.CompareAndSwap(false, true) {
		return
	}
	l.reservation.Release()
	if l.acquired.Load() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), priorityAdmissionCleanupTimeout)
	defer cancel()
	if err := l.cache.CancelPriorityUserWait(ctx, l.request.UserID, l.request.RequestID); err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: failed to cancel priority user wait for %d (req=%s): %v", l.request.UserID, l.request.RequestID, err)
	}
}
