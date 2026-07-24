package service

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/cespare/xxhash/v2"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// ConcurrencyCache 定义并发控制的缓存接口
// 使用有序集合存储槽位，按时间戳清理过期条目
type ConcurrencyCache interface {
	// 账号槽位管理
	// 键格式: concurrency:account:{accountID}（有序集合，成员为 requestID）
	AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error)
	ReleaseAccountSlot(ctx context.Context, accountID int64, requestID string) error
	GetAccountConcurrency(ctx context.Context, accountID int64) (int, error)
	GetAccountConcurrencyBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error)

	// 账号等待队列（账号级）
	IncrementAccountWaitCount(ctx context.Context, accountID int64, maxWait int) (bool, error)
	DecrementAccountWaitCount(ctx context.Context, accountID int64) error
	GetAccountWaitingCount(ctx context.Context, accountID int64) (int, error)

	// 用户槽位管理
	// 键格式: concurrency:user:{userID}（有序集合，成员为 requestID）
	AcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error)
	ReleaseUserSlot(ctx context.Context, userID int64, requestID string) error
	GetUserConcurrency(ctx context.Context, userID int64) (int, error)

	// 等待队列计数（每次入队都会刷新 TTL，避免长时间排队时计数提前过期）
	IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error)
	DecrementWaitCount(ctx context.Context, userID int64) error

	// 批量负载查询（只读）
	GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error)
	GetUsersLoadBatch(ctx context.Context, users []UserWithConcurrency) (map[int64]*UserLoadInfo, error)

	// 清理过期槽位（后台任务）
	CleanupExpiredAccountSlots(ctx context.Context, accountID int64) error
	CleanupExpiredAccountSlotKeys(ctx context.Context) error

	// 启动时清理旧进程遗留槽位与等待计数
	CleanupStaleProcessSlots(ctx context.Context, activeRequestPrefix string) error
}

type APIKeyConcurrencyCache interface {
	AcquireAPIKeySlot(ctx context.Context, apiKeyID int64, maxConcurrency int, requestID string) (bool, error)
	TrackAPIKeySlot(ctx context.Context, apiKeyID int64, requestID string) error
	ReleaseAPIKeySlot(ctx context.Context, apiKeyID int64, requestID string) error
	GetAPIKeyConcurrencyBatch(ctx context.Context, apiKeyIDs []int64) (map[int64]int, error)
}

// OpenAIWSIngressLeaseCache owns the short-lived distributed lease used to
// bound live client WebSocket sessions. It is deliberately independent of the
// request-slot namespace: idle ingress connections do not occupy turn slots.
type OpenAIWSIngressLeaseCache interface {
	AcquireOpenAIWSIngressLease(ctx context.Context, apiKeyID int64, maxConnections int, leaseID string) (bool, error)
	RefreshOpenAIWSIngressLease(ctx context.Context, apiKeyID int64, leaseID string) (bool, error)
	ReleaseOpenAIWSIngressLease(ctx context.Context, apiKeyID int64, leaseID string) error
}

const (
	openAIWSIngressLeaseTTL             = 60 * time.Second
	openAIWSIngressLeaseRefreshInterval = 20 * time.Second
	openAIWSIngressLeaseOperationTO     = 2 * time.Second
)

var ErrOpenAIWSIngressLeaseLost = errors.New("openai websocket ingress lease lost")

// OpenAIWSIngressLease keeps a Redis-backed ingress lease alive and cancels
// its context if Redis cannot confirm ownership for a full lease lifetime.
// Call Release on every handler exit to reclaim capacity immediately.
type OpenAIWSIngressLease struct {
	ctx      context.Context
	cancel   context.CancelCauseFunc
	cache    OpenAIWSIngressLeaseCache
	apiKeyID int64
	leaseID  string

	stopOnce    sync.Once
	stopCh      chan struct{}
	refreshDone chan struct{}
}

func (l *OpenAIWSIngressLease) Context() context.Context {
	if l == nil || l.ctx == nil {
		return context.Background()
	}
	return l.ctx
}

func (l *OpenAIWSIngressLease) Release() {
	if l == nil {
		return
	}
	l.stopOnce.Do(func() {
		if l.stopCh != nil {
			close(l.stopCh)
		}
		if l.cancel != nil {
			l.cancel(nil)
		}
		if l.refreshDone != nil {
			<-l.refreshDone
		}
		if l.cache == nil || l.apiKeyID <= 0 || l.leaseID == "" {
			return
		}
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), openAIWSIngressLeaseOperationTO)
		defer releaseCancel()
		if err := l.cache.ReleaseOpenAIWSIngressLease(releaseCtx, l.apiKeyID, l.leaseID); err != nil {
			logger.L().Warn("openai_ws_ingress_lease_release_failed",
				zap.Int64("api_key_id", l.apiKeyID),
				zap.Error(err),
			)
		}
	})
}

func (l *OpenAIWSIngressLease) refreshLoop() {
	defer func() {
		if l != nil && l.refreshDone != nil {
			close(l.refreshDone)
		}
	}()
	if l == nil || l.cache == nil {
		return
	}
	ticker := time.NewTicker(openAIWSIngressLeaseRefreshInterval)
	defer ticker.Stop()
	lastConfirmedAt := time.Now()
	for {
		select {
		case <-l.ctx.Done():
			return
		case <-l.stopCh:
			return
		case <-ticker.C:
			var lost bool
			lastConfirmedAt, lost = l.refresh(lastConfirmedAt)
			if lost {
				l.cancel(ErrOpenAIWSIngressLeaseLost)
				return
			}
		}
	}
}

// refresh confirms the lease is still owned. A missing member is an immediate
// lease loss; transient Redis errors are tolerated only for one full lease TTL.
func (l *OpenAIWSIngressLease) refresh(lastConfirmedAt time.Time) (time.Time, bool) {
	refreshCtx, refreshCancel := context.WithTimeout(context.Background(), openAIWSIngressLeaseOperationTO)
	owned, err := l.cache.RefreshOpenAIWSIngressLease(refreshCtx, l.apiKeyID, l.leaseID)
	refreshCancel()
	if err == nil && owned {
		return time.Now(), false
	}
	if err == nil {
		err = ErrOpenAIWSIngressLeaseLost
	}
	elapsed := time.Since(lastConfirmedAt)
	logger.L().Warn("openai_ws_ingress_lease_refresh_failed",
		zap.Int64("api_key_id", l.apiKeyID),
		zap.Duration("unconfirmed_for", elapsed),
		zap.Error(err),
	)
	if errors.Is(err, ErrOpenAIWSIngressLeaseLost) || elapsed >= openAIWSIngressLeaseTTL {
		logger.L().Error("openai_ws_ingress_lease_lost",
			zap.Int64("api_key_id", l.apiKeyID),
			zap.Duration("unconfirmed_for", elapsed),
			zap.Error(err),
		)
		return lastConfirmedAt, true
	}
	return lastConfirmedAt, false
}

var (
	requestIDPrefix  = initRequestIDPrefix()
	requestIDCounter atomic.Uint64
)

func initRequestIDPrefix() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err == nil {
		return "r" + strconv.FormatUint(binary.BigEndian.Uint64(b), 36)
	}
	fallback := uint64(time.Now().UnixNano()) ^ (uint64(os.Getpid()) << 16)
	return "r" + strconv.FormatUint(fallback, 36)
}

func RequestIDPrefix() string {
	return requestIDPrefix
}

func generateRequestID() string {
	seq := requestIDCounter.Add(1)
	return requestIDPrefix + "-" + strconv.FormatUint(seq, 36)
}

func (s *ConcurrencyService) CleanupStaleProcessSlots(ctx context.Context) error {
	if s == nil || s.cache == nil {
		return nil
	}
	return s.cache.CleanupStaleProcessSlots(ctx, RequestIDPrefix())
}

const (
	// 默认等待队列额外槽位
	defaultExtraWaitSlots = 20

	defaultAccountLoadBatchCacheTTL = 200 * time.Millisecond
	accountLoadBatchFetchTimeout    = 3 * time.Second
	maxAccountLoadBatchCacheEntries = 256
	apiKeyConcurrencyFetchTimeout   = 3 * time.Second
	apiKeySlotTrackTimeout          = 2 * time.Second
	localRequestSlotCapacity        = 32768
	localRequestSlotIdleTTL         = 10 * time.Minute
)

// ConcurrencyService 管理账号和用户的并发限制。
type ConcurrencyService struct {
	cache ConcurrencyCache

	standaloneRequestSlots atomic.Bool
	localAPIKeySlots       localSlotRegistry
	localUserSlots         localSlotRegistry

	priorityAdmissionConfig atomic.Pointer[priorityAdmissionRuntimeConfig]
	priorityPendingMu       sync.Mutex
	priorityPendingCount    [2]int64
	priorityPendingBytes    [2]int64

	accountLoadCacheTTL atomic.Int64
	accountLoadCacheMu  sync.Mutex
	accountLoadCache    sync.Map // map[accountLoadBatchKey]*cachedAccountLoadBatch; hits stay lock-free
	accountLoadCacheLen atomic.Int64
	accountLoadGroup    singleflight.Group
	cleanupMu           sync.Mutex
	cleanupCancel       context.CancelFunc
	cleanupWG           sync.WaitGroup
	cleanupStopped      bool
}

type localSlotRegistry struct {
	slots   sync.Map // int64 ID -> *localRequestSlot
	size    atomic.Int64
	pruneMu sync.Mutex
}

type localRequestSlot struct {
	active   atomic.Int64
	waiting  atomic.Int64
	lastUsed atomic.Int64
	retired  atomic.Bool
}

type cachedAccountLoadBatch struct {
	loadMap   map[int64]*AccountLoadInfo
	expiresAt time.Time
}

// Two independently seeded hashes keep the cache key comparable and allocation-free
// on hits while retaining collision resistance appropriate for process-local caching.
type accountLoadBatchKey struct {
	count             int
	hashA             uint64
	hashB             uint64
	priorityAdmission bool
}

func (k accountLoadBatchKey) singleflightKey() string {
	var buf [64]byte
	encoded := strconv.AppendInt(buf[:0], int64(k.count), 10)
	encoded = append(encoded, ':')
	encoded = strconv.AppendUint(encoded, k.hashA, 16)
	encoded = append(encoded, ':')
	encoded = strconv.AppendUint(encoded, k.hashB, 16)
	if k.priorityAdmission {
		encoded = append(encoded, ':', 'p')
	}
	return string(encoded)
}

// NewConcurrencyService 创建并发控制服务。
func NewConcurrencyService(cache ConcurrencyCache) *ConcurrencyService {
	svc := &ConcurrencyService{cache: cache}
	svc.SetAccountLoadBatchCacheTTL(defaultAccountLoadBatchCacheTTL)
	svc.SetPriorityAdmissionRuntimeConfig(DefaultPriorityAdmissionRuntimeConfig())
	return svc
}

func (s *ConcurrencyService) SetStandaloneRequestSlots(enabled bool) {
	if s == nil {
		return
	}
	s.standaloneRequestSlots.Store(enabled)
}

// AcquireOpenAIWSIngressLease atomically reserves one live ingress connection
// for an API key. A non-positive limit explicitly disables this protection.
func (s *ConcurrencyService) AcquireOpenAIWSIngressLease(ctx context.Context, apiKeyID int64, maxConnections int) (*OpenAIWSIngressLease, bool, error) {
	if maxConnections <= 0 {
		return nil, true, nil
	}
	if s == nil || s.cache == nil || apiKeyID <= 0 {
		return nil, false, errors.New("openai websocket ingress lease cache is unavailable")
	}
	cache, ok := s.cache.(OpenAIWSIngressLeaseCache)
	if !ok {
		return nil, false, errors.New("openai websocket ingress lease cache is unsupported")
	}
	leaseID := generateRequestID()
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}
	acquireCtx, acquireCancel := context.WithTimeout(baseCtx, openAIWSIngressLeaseOperationTO)
	acquired, err := cache.AcquireOpenAIWSIngressLease(acquireCtx, apiKeyID, maxConnections, leaseID)
	acquireCancel()
	if err != nil || !acquired {
		return nil, acquired, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	leaseCtx, leaseCancel := context.WithCancelCause(ctx)
	lease := &OpenAIWSIngressLease{
		ctx:         leaseCtx,
		cancel:      leaseCancel,
		cache:       cache,
		apiKeyID:    apiKeyID,
		leaseID:     leaseID,
		stopCh:      make(chan struct{}),
		refreshDone: make(chan struct{}),
	}
	go lease.refreshLoop()
	return lease, true, nil
}

// SetAccountLoadBatchCacheTTL 设置账号负载批量读取的极短 TTL 缓存；非正数表示禁用缓存。
func (s *ConcurrencyService) SetAccountLoadBatchCacheTTL(ttl time.Duration) {
	if s == nil {
		return
	}
	s.accountLoadCacheTTL.Store(int64(ttl))
	if ttl <= 0 {
		s.accountLoadCacheMu.Lock()
		s.accountLoadCache.Range(func(key, _ any) bool {
			s.accountLoadCache.Delete(key)
			return true
		})
		s.accountLoadCacheLen.Store(0)
		s.accountLoadCacheMu.Unlock()
	}
}

// AcquireResult represents the result of acquiring a concurrency slot
type AcquireResult struct {
	Acquired                  bool
	ReleaseFunc               func() // Must be called when done (typically via defer)
	PriorityAdmissionTerminal bool   // low-tier account admission failed; do not probe another account
}

var rejectedAcquireResult = &AcquireResult{}
var priorityAdmissionTerminalAcquireResult = &AcquireResult{PriorityAdmissionTerminal: true}

type AccountWithConcurrency struct {
	ID             int64
	MaxConcurrency int
}

type UserWithConcurrency struct {
	ID             int64
	MaxConcurrency int
}

type AccountLoadInfo struct {
	AccountID          int64
	CurrentConcurrency int
	WaitingCount       int
	LoadRate           int // 0-100+ (percent)
}

type UserLoadInfo struct {
	UserID             int64
	CurrentConcurrency int
	WaitingCount       int
	LoadRate           int // 0-100+ (percent)
}

// AcquireAccountSlot attempts to acquire a concurrency slot for an account.
// If the account is at max concurrency, it waits until a slot is available or timeout.
// Returns a release function that MUST be called when the request completes.
func (s *ConcurrencyService) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (*AcquireResult, error) {
	return s.acquireAccountSlotLegacy(ctx, accountID, maxConcurrency)
}

// acquireAccountSlotLegacy is deliberately kept as the feature-off path. Do
// not add priority queue work here: internal callers without an explicitly
// tagged gateway context must retain the existing behavior.
func (s *ConcurrencyService) acquireAccountSlotLegacy(ctx context.Context, accountID int64, maxConcurrency int) (*AcquireResult, error) {
	// If maxConcurrency is 0 or negative, no limit
	if maxConcurrency <= 0 {
		return &AcquireResult{
			Acquired:    true,
			ReleaseFunc: func() {}, // no-op
		}, nil
	}

	// Generate unique request ID for this slot
	requestID := generateRequestID()

	acquired, err := s.cache.AcquireAccountSlot(ctx, accountID, maxConcurrency, requestID)
	if err != nil {
		return nil, err
	}

	if acquired {
		return &AcquireResult{
			Acquired: true,
			ReleaseFunc: func() {
				bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.cache.ReleaseAccountSlot(bgCtx, accountID, requestID); err != nil {
					logger.LegacyPrintf("service.concurrency", "Warning: failed to release account slot for %d (req=%s): %v", accountID, requestID, err)
				}
			},
		}, nil
	}

	return &AcquireResult{
		Acquired:    false,
		ReleaseFunc: nil,
	}, nil
}

// AcquireUserSlot attempts to acquire a concurrency slot for a user.
// If the user is at max concurrency, it waits until a slot is available or timeout.
// Returns a release function that MUST be called when the request completes.
func (s *ConcurrencyService) AcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int) (*AcquireResult, error) {
	return s.acquireUserSlotLegacy(ctx, userID, maxConcurrency)
}

func (s *ConcurrencyService) acquireUserSlotLegacy(ctx context.Context, userID int64, maxConcurrency int) (*AcquireResult, error) {
	// If maxConcurrency is 0 or negative, no limit
	if maxConcurrency <= 0 {
		return &AcquireResult{
			Acquired:    true,
			ReleaseFunc: func() {}, // no-op
		}, nil
	}
	if s != nil && s.standaloneRequestSlots.Load() {
		if result, ok := s.acquireStandaloneSlot(&s.localUserSlots, userID, maxConcurrency); ok {
			return result, nil
		}
	}
	if s == nil || s.cache == nil {
		return &AcquireResult{Acquired: true, ReleaseFunc: func() {}}, nil
	}

	// Generate unique request ID for this slot
	requestID := generateRequestID()

	acquired, err := s.cache.AcquireUserSlot(ctx, userID, maxConcurrency, requestID)
	if err != nil {
		return nil, err
	}

	if acquired {
		return &AcquireResult{
			Acquired: true,
			ReleaseFunc: func() {
				bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.cache.ReleaseUserSlot(bgCtx, userID, requestID); err != nil {
					logger.LegacyPrintf("service.concurrency", "Warning: failed to release user slot for %d (req=%s): %v", userID, requestID, err)
				}
			},
		}, nil
	}

	return &AcquireResult{
		Acquired:    false,
		ReleaseFunc: nil,
	}, nil
}

// AcquireAPIKeySlot records one active request for an API key and enforces its
// configured concurrency limit. A non-positive limit keeps stats without
// restricting requests.
func (s *ConcurrencyService) AcquireAPIKeySlot(ctx context.Context, apiKeyID int64, maxConcurrency int) (*AcquireResult, error) {
	if s == nil || apiKeyID <= 0 {
		return &AcquireResult{Acquired: true, ReleaseFunc: func() {}}, nil
	}
	if s.standaloneRequestSlots.Load() {
		if result, ok := s.acquireStandaloneSlot(&s.localAPIKeySlots, apiKeyID, maxConcurrency); ok {
			return result, nil
		}
	}
	if s.cache == nil {
		return &AcquireResult{Acquired: true, ReleaseFunc: func() {}}, nil
	}
	cache, ok := s.cache.(APIKeyConcurrencyCache)
	if !ok {
		return &AcquireResult{Acquired: true, ReleaseFunc: func() {}}, nil
	}

	requestID := generateRequestID()
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}
	trackCtx, cancel := context.WithTimeout(baseCtx, apiKeySlotTrackTimeout)
	var acquired bool
	var err error
	if maxConcurrency > 0 {
		acquired, err = cache.AcquireAPIKeySlot(trackCtx, apiKeyID, maxConcurrency, requestID)
	} else {
		err = cache.TrackAPIKeySlot(trackCtx, apiKeyID, requestID)
		acquired = err == nil
	}
	cancel()
	if err != nil {
		if maxConcurrency <= 0 {
			logger.LegacyPrintf("service.concurrency", "Warning: failed to track api key slot for %d (req=%s): %v", apiKeyID, requestID, err)
			return &AcquireResult{Acquired: true, ReleaseFunc: func() {}}, nil
		}
		return nil, err
	}
	if !acquired {
		return &AcquireResult{Acquired: false}, nil
	}

	return &AcquireResult{
		Acquired: true,
		ReleaseFunc: func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := cache.ReleaseAPIKeySlot(bgCtx, apiKeyID, requestID); err != nil {
				logger.LegacyPrintf("service.concurrency", "Warning: failed to release api key slot for %d (req=%s): %v", apiKeyID, requestID, err)
			}
		},
	}, nil
}

// GetAPIKeyConcurrencyBatch gets real-time active request counts for API keys.
// Stats are best-effort: missing Redis support or Redis errors return zeroes.
func (s *ConcurrencyService) GetAPIKeyConcurrencyBatch(ctx context.Context, apiKeyIDs []int64) (map[int64]int, error) {
	result := zeroAPIKeyConcurrencyMap(apiKeyIDs)
	if len(apiKeyIDs) == 0 {
		return result, nil
	}
	if s != nil && s.standaloneRequestSlots.Load() {
		for _, id := range apiKeyIDs {
			if value, ok := s.localAPIKeySlots.slots.Load(id); ok {
				if slot, ok := value.(*localRequestSlot); ok && slot != nil && !slot.retired.Load() {
					count := slot.active.Load()
					maxInt := int64(^uint(0) >> 1)
					if count > maxInt {
						count = maxInt
					}
					if count > 0 {
						result[id] = int(count)
					}
				}
			}
		}
		return result, nil
	}
	if s == nil || s.cache == nil {
		return result, nil
	}
	cache, ok := s.cache.(APIKeyConcurrencyCache)
	if !ok {
		return result, nil
	}

	redisCtx, cancel := context.WithTimeout(context.Background(), apiKeyConcurrencyFetchTimeout)
	defer cancel()

	counts, err := cache.GetAPIKeyConcurrencyBatch(redisCtx, apiKeyIDs)
	if err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: get api key concurrency batch failed: %v", err)
		return result, nil
	}
	for _, apiKeyID := range apiKeyIDs {
		result[apiKeyID] = counts[apiKeyID]
	}
	return result, nil
}

func (s *ConcurrencyService) acquireStandaloneSlot(registry *localSlotRegistry, id int64, maxConcurrency int) (*AcquireResult, bool) {
	for {
		slot := registry.getOrCreate(id)
		if slot == nil {
			return nil, false
		}
		for !slot.retired.Load() {
			current := slot.active.Load()
			if maxConcurrency > 0 && current >= int64(maxConcurrency) {
				slot.lastUsed.Store(time.Now().UnixNano())
				return rejectedAcquireResult, true
			}
			if !slot.active.CompareAndSwap(current, current+1) {
				continue
			}
			if slot.retired.Load() {
				slot.active.Add(-1)
				break
			}
			slot.lastUsed.Store(time.Now().UnixNano())
			released := &atomic.Bool{}
			return &AcquireResult{
				Acquired: true,
				ReleaseFunc: func() {
					if released.CompareAndSwap(false, true) {
						slot.active.Add(-1)
						slot.lastUsed.Store(time.Now().UnixNano())
					}
				},
			}, true
		}
	}
}

func (r *localSlotRegistry) getOrCreate(id int64) *localRequestSlot {
	if value, ok := r.slots.Load(id); ok {
		if slot, ok := value.(*localRequestSlot); ok && slot != nil && !slot.retired.Load() {
			return slot
		}
	}
	if r.size.Load() >= localRequestSlotCapacity {
		r.prune(time.Now())
		if r.size.Load() >= localRequestSlotCapacity {
			return nil
		}
	}
	if r.size.Add(1) > localRequestSlotCapacity {
		r.size.Add(-1)
		return nil
	}
	candidate := &localRequestSlot{}
	candidate.lastUsed.Store(time.Now().UnixNano())
	actual, loaded := r.slots.LoadOrStore(id, candidate)
	if !loaded {
		return candidate
	}
	r.size.Add(-1)
	slot, _ := actual.(*localRequestSlot)
	if slot == nil || slot.retired.Load() {
		if r.slots.CompareAndDelete(id, actual) {
			r.size.Add(-1)
		}
		return r.getOrCreate(id)
	}
	return slot
}

func (r *localSlotRegistry) prune(now time.Time) {
	if !r.pruneMu.TryLock() {
		return
	}
	defer r.pruneMu.Unlock()
	staleBefore := now.Add(-localRequestSlotIdleTTL).UnixNano()
	r.slots.Range(func(key, value any) bool {
		slot, ok := value.(*localRequestSlot)
		if !ok || slot == nil {
			if r.slots.CompareAndDelete(key, value) {
				r.size.Add(-1)
			}
			return true
		}
		if slot.active.Load() != 0 || slot.waiting.Load() != 0 || slot.lastUsed.Load() > staleBefore || !slot.retired.CompareAndSwap(false, true) {
			return true
		}
		if r.slots.CompareAndDelete(key, slot) {
			r.size.Add(-1)
		} else {
			slot.retired.Store(false)
		}
		return true
	})
}

func zeroAPIKeyConcurrencyMap(apiKeyIDs []int64) map[int64]int {
	result := make(map[int64]int, len(apiKeyIDs))
	for _, apiKeyID := range apiKeyIDs {
		result[apiKeyID] = 0
	}
	return result
}

// ============================================
// Wait Queue Count Methods
// ============================================

// IncrementWaitCount attempts to increment the wait queue counter for a user.
// Returns true if successful, false if the wait queue is full.
// maxWait should be user.Concurrency + defaultExtraWaitSlots
func (s *ConcurrencyService) IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	if s != nil && s.standaloneRequestSlots.Load() {
		slot := s.localUserSlots.getOrCreate(userID)
		if slot != nil {
			for {
				current := slot.waiting.Load()
				if maxWait > 0 && current >= int64(maxWait) {
					return false, nil
				}
				if slot.waiting.CompareAndSwap(current, current+1) {
					slot.lastUsed.Store(time.Now().UnixNano())
					return true, nil
				}
			}
		}
	}
	if s.cache == nil {
		// Redis not available, allow request
		return true, nil
	}

	result, err := s.cache.IncrementWaitCount(ctx, userID, maxWait)
	if err != nil {
		// On error, allow the request to proceed (fail open)
		logger.LegacyPrintf("service.concurrency", "Warning: increment wait count failed for user %d: %v", userID, err)
		return true, nil
	}
	return result, nil
}

// DecrementWaitCount decrements the wait queue counter for a user.
// Should be called when a request completes or exits the wait queue.
func (s *ConcurrencyService) DecrementWaitCount(ctx context.Context, userID int64) {
	if s != nil && s.standaloneRequestSlots.Load() {
		if value, ok := s.localUserSlots.slots.Load(userID); ok {
			if slot, ok := value.(*localRequestSlot); ok && slot != nil && !slot.retired.Load() {
				for {
					current := slot.waiting.Load()
					if current <= 0 || slot.waiting.CompareAndSwap(current, current-1) {
						break
					}
				}
				slot.lastUsed.Store(time.Now().UnixNano())
				return
			}
		}
	}
	if s.cache == nil {
		return
	}

	// Use background context to ensure decrement even if original context is cancelled
	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.cache.DecrementWaitCount(bgCtx, userID); err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: decrement wait count failed for user %d: %v", userID, err)
	}
}

// IncrementAccountWaitCount increments the wait queue counter for an account.
func (s *ConcurrencyService) IncrementAccountWaitCount(ctx context.Context, accountID int64, maxWait int) (bool, error) {
	if s.cache == nil {
		return true, nil
	}

	result, err := s.cache.IncrementAccountWaitCount(ctx, accountID, maxWait)
	if err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: increment wait count failed for account %d: %v", accountID, err)
		return true, nil
	}
	return result, nil
}

// DecrementAccountWaitCount decrements the wait queue counter for an account.
func (s *ConcurrencyService) DecrementAccountWaitCount(ctx context.Context, accountID int64) {
	if s.cache == nil {
		return
	}

	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.cache.DecrementAccountWaitCount(bgCtx, accountID); err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: decrement wait count failed for account %d: %v", accountID, err)
	}
}

// GetAccountWaitingCount gets current wait queue count for an account.
func (s *ConcurrencyService) GetAccountWaitingCount(ctx context.Context, accountID int64) (int, error) {
	if s.cache == nil {
		return 0, nil
	}
	config, _ := s.priorityAdmissionRequestConfig(ctx)
	if config.enabled {
		ctx = withPriorityAdmissionLoadCounts(ctx)
	}
	return s.cache.GetAccountWaitingCount(ctx, accountID)
}

// CalculateMaxWait calculates the maximum wait queue size for a user
// maxWait = userConcurrency + defaultExtraWaitSlots
func CalculateMaxWait(userConcurrency int) int {
	if userConcurrency <= 0 {
		userConcurrency = 1
	}
	return userConcurrency + defaultExtraWaitSlots
}

// GetAccountsLoadBatch 批量获取账号负载信息。
func (s *ConcurrencyService) GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	return s.getAccountsLoadBatch(ctx, accounts, true)
}

// GetAccountsLoadBatchFresh 绕过极短 TTL 缓存，用于抢槽失败后的实时刷新兜底。
func (s *ConcurrencyService) GetAccountsLoadBatchFresh(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	return s.getAccountsLoadBatch(ctx, accounts, false)
}

func (s *ConcurrencyService) getAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency, allowCache bool) (map[int64]*AccountLoadInfo, error) {
	if len(accounts) == 0 {
		return map[int64]*AccountLoadInfo{}, nil
	}
	if s.cache == nil {
		return map[int64]*AccountLoadInfo{}, nil
	}

	ttl := time.Duration(s.accountLoadCacheTTL.Load())
	if !allowCache || ttl <= 0 {
		return s.fetchAccountsLoadBatch(ctx, accounts)
	}

	config, _ := s.priorityAdmissionRequestConfig(ctx)
	key := accountLoadBatchCacheKey(accounts, config.enabled)
	if cached, ok := s.getCachedAccountLoadBatch(key, time.Now()); ok {
		return cached, nil
	}

	value, err, _ := s.accountLoadGroup.Do(key.singleflightKey(), func() (any, error) {
		now := time.Now()
		if cached, ok := s.getCachedAccountLoadBatch(key, now); ok {
			return cached, nil
		}
		loadMap, fetchErr := s.fetchAccountsLoadBatch(ctx, accounts)
		if fetchErr != nil {
			return nil, fetchErr
		}
		cached := cloneAccountLoadMap(loadMap)
		s.storeCachedAccountLoadBatch(key, cached, now.Add(ttl))
		return cached, nil
	})
	if err != nil {
		return nil, err
	}
	loadMap, _ := value.(map[int64]*AccountLoadInfo)
	if loadMap == nil {
		return map[int64]*AccountLoadInfo{}, nil
	}
	return loadMap, nil
}

func (s *ConcurrencyService) fetchAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	if s.cache == nil {
		return map[int64]*AccountLoadInfo{}, nil
	}
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}
	config, _ := s.priorityAdmissionRequestConfig(ctx)
	if config.enabled {
		baseCtx = withPriorityAdmissionLoadCounts(baseCtx)
	}
	redisCtx, cancel := context.WithTimeout(baseCtx, accountLoadBatchFetchTimeout)
	defer cancel()
	return s.cache.GetAccountsLoadBatch(redisCtx, accounts)
}

func (s *ConcurrencyService) getCachedAccountLoadBatch(key accountLoadBatchKey, now time.Time) (map[int64]*AccountLoadInfo, bool) {
	value, ok := s.accountLoadCache.Load(key)
	if !ok {
		return nil, false
	}
	cached, ok := value.(*cachedAccountLoadBatch)
	if !ok || cached == nil {
		return nil, false
	}
	if !now.Before(cached.expiresAt) {
		s.accountLoadCacheMu.Lock()
		if current, exists := s.accountLoadCache.Load(key); exists && current == cached {
			s.accountLoadCache.Delete(key)
			s.accountLoadCacheLen.Add(-1)
		}
		s.accountLoadCacheMu.Unlock()
		return nil, false
	}
	return cached.loadMap, true
}

func (s *ConcurrencyService) storeCachedAccountLoadBatch(key accountLoadBatchKey, loadMap map[int64]*AccountLoadInfo, expiresAt time.Time) {
	updated := &cachedAccountLoadBatch{loadMap: loadMap, expiresAt: expiresAt}
	s.accountLoadCacheMu.Lock()
	if _, exists := s.accountLoadCache.Load(key); exists {
		s.accountLoadCache.Store(key, updated)
		s.accountLoadCacheMu.Unlock()
		return
	}
	if s.accountLoadCacheLen.Load() >= maxAccountLoadBatchCacheEntries {
		now := time.Now()
		s.accountLoadCache.Range(func(cacheKey, value any) bool {
			cached, _ := value.(*cachedAccountLoadBatch)
			if cached == nil || !now.Before(cached.expiresAt) {
				s.accountLoadCache.Delete(cacheKey)
				s.accountLoadCacheLen.Add(-1)
			}
			return true
		})
		for s.accountLoadCacheLen.Load() >= maxAccountLoadBatchCacheEntries {
			removed := false
			s.accountLoadCache.Range(func(cacheKey, _ any) bool {
				s.accountLoadCache.Delete(cacheKey)
				s.accountLoadCacheLen.Add(-1)
				removed = true
				return false
			})
			if !removed {
				s.accountLoadCacheLen.Store(0)
				break
			}
		}
	}
	s.accountLoadCache.Store(key, updated)
	s.accountLoadCacheLen.Add(1)
	s.accountLoadCacheMu.Unlock()
}

func accountLoadBatchCacheKey(accounts []AccountWithConcurrency, priorityAdmission bool) accountLoadBatchKey {
	hashA := xxhash.NewWithSeed(0x9e3779b185ebca87)
	hashB := xxhash.NewWithSeed(0xc2b2ae3d27d4eb4f)
	var buf [16]byte
	for _, account := range accounts {
		binary.LittleEndian.PutUint64(buf[:8], uint64(account.ID))
		binary.LittleEndian.PutUint64(buf[8:], uint64(int64(account.MaxConcurrency)))
		_, _ = hashA.Write(buf[:])
		_, _ = hashB.Write(buf[:])
	}
	return accountLoadBatchKey{
		count:             len(accounts),
		hashA:             hashA.Sum64(),
		hashB:             hashB.Sum64(),
		priorityAdmission: priorityAdmission,
	}
}

func cloneAccountLoadMap(loadMap map[int64]*AccountLoadInfo) map[int64]*AccountLoadInfo {
	if len(loadMap) == 0 {
		return map[int64]*AccountLoadInfo{}
	}
	clone := make(map[int64]*AccountLoadInfo, len(loadMap))
	for accountID, loadInfo := range loadMap {
		if loadInfo == nil {
			clone[accountID] = nil
			continue
		}
		copied := *loadInfo
		clone[accountID] = &copied
	}
	return clone
}

// GetUsersLoadBatch returns load info for multiple users.
func (s *ConcurrencyService) GetUsersLoadBatch(ctx context.Context, users []UserWithConcurrency) (map[int64]*UserLoadInfo, error) {
	loadMap := make(map[int64]*UserLoadInfo, len(users))
	if s != nil && s.cache != nil {
		loadCtx := ctx
		config, _ := s.priorityAdmissionRequestConfig(ctx)
		if config.enabled {
			loadCtx = withPriorityAdmissionLoadCounts(loadCtx)
		}
		cached, err := s.cache.GetUsersLoadBatch(loadCtx, users)
		if err != nil {
			return nil, err
		}
		loadMap = cached
		if loadMap == nil {
			loadMap = make(map[int64]*UserLoadInfo, len(users))
		}
	}
	if s == nil || !s.standaloneRequestSlots.Load() {
		return loadMap, nil
	}
	for _, user := range users {
		info := loadMap[user.ID]
		if info == nil {
			info = &UserLoadInfo{UserID: user.ID}
			loadMap[user.ID] = info
		}
		if value, ok := s.localUserSlots.slots.Load(user.ID); ok {
			if slot, ok := value.(*localRequestSlot); ok && slot != nil && !slot.retired.Load() {
				info.CurrentConcurrency = int(slot.active.Load())
				info.WaitingCount = int(slot.waiting.Load())
			}
		}
		if user.MaxConcurrency > 0 {
			info.LoadRate = (info.CurrentConcurrency + info.WaitingCount) * 100 / user.MaxConcurrency
		}
	}
	return loadMap, nil
}

// CleanupExpiredAccountSlots removes expired slots for one account (background task).
func (s *ConcurrencyService) CleanupExpiredAccountSlots(ctx context.Context, accountID int64) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.CleanupExpiredAccountSlots(ctx, accountID)
}

// StartSlotCleanupWorker starts a background cleanup worker for expired account slots.
func (s *ConcurrencyService) StartSlotCleanupWorker(_ AccountRepository, interval time.Duration) {
	if s == nil || s.cache == nil || interval <= 0 {
		return
	}

	runCleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := s.cache.CleanupExpiredAccountSlotKeys(cleanupCtx)
		cancel()
		if err != nil {
			logger.LegacyPrintf("service.concurrency", "Warning: cleanup expired account slots failed: %v", err)
			return
		}
	}

	s.cleanupMu.Lock()
	if s.cleanupStopped || s.cleanupCancel != nil {
		s.cleanupMu.Unlock()
		return
	}
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	s.cleanupCancel = cleanupCancel
	s.cleanupWG.Add(1)
	s.cleanupMu.Unlock()

	go func() {
		defer s.cleanupWG.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		runCleanup()
		for {
			select {
			case <-cleanupCtx.Done():
				return
			case <-ticker.C:
				runCleanup()
			}
		}
	}()
}

// Stop terminates the background slot cleanup worker.
func (s *ConcurrencyService) Stop() {
	if s == nil {
		return
	}
	s.cleanupMu.Lock()
	s.cleanupStopped = true
	cancel := s.cleanupCancel
	s.cleanupCancel = nil
	s.cleanupMu.Unlock()
	if cancel != nil {
		cancel()
		s.cleanupWG.Wait()
	}
}

// GetAccountConcurrencyBatch gets current concurrency counts for multiple accounts.
// Uses a detached context with timeout to prevent HTTP request cancellation from
// causing the entire batch to fail (which would show all concurrency as 0).
func (s *ConcurrencyService) GetAccountConcurrencyBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error) {
	if len(accountIDs) == 0 {
		return map[int64]int{}, nil
	}
	if s.cache == nil {
		result := make(map[int64]int, len(accountIDs))
		for _, accountID := range accountIDs {
			result[accountID] = 0
		}
		return result, nil
	}

	// Use a detached context so that a cancelled HTTP request doesn't cause
	// the Redis pipeline to fail and return all-zero concurrency counts.
	redisCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return s.cache.GetAccountConcurrencyBatch(redisCtx, accountIDs)
}
