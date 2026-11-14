package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

const openAIUsageSyncLeaseKey = "openai:usage-push:background"

type openAIUsageSyncOptions struct {
	scanInterval  time.Duration
	leaseTTL      time.Duration
	leaseInterval time.Duration
	retryMin      time.Duration
	retryMax      time.Duration
	handshakes    int
	idleWindow    time.Duration
	idleInterval  time.Duration
}

type openAIUsageSyncWorker struct {
	identity string
	cancel   context.CancelFunc
	done     chan struct{}
	lastUsed time.Time
}

// OpenAIUsageSyncService owns upstream quota WS for all active OpenAI OAuth
// accounts, independently of browsers, list pagination and virtual scrolling.
type OpenAIUsageSyncService struct {
	quota      *OpenAIQuotaService
	locks      LeaderLockCache
	opts       openAIUsageSyncOptions
	mu         sync.Mutex
	cancel     context.CancelFunc
	done       chan struct{}
	activityMu sync.Mutex
	activity   map[int64]time.Time
	wake       chan struct{}
}

func NewOpenAIUsageSyncService(quota *OpenAIQuotaService, locks LeaderLockCache) *OpenAIUsageSyncService {
	if quota != nil {
		quota.usageSyncManaged.Store(true)
	}
	return &OpenAIUsageSyncService{quota: quota, locks: locks, activity: make(map[int64]time.Time), wake: make(chan struct{}, 1), opts: openAIUsageSyncOptions{
		scanInterval: 30 * time.Second, leaseTTL: 45 * time.Second, leaseInterval: 10 * time.Second,
		retryMin: 30 * time.Second, retryMax: 5 * time.Minute, handshakes: 1000, idleWindow: 10 * time.Minute, idleInterval: time.Second,
	}}
}

func (s *OpenAIUsageSyncService) Start() {
	if s == nil || s.quota == nil || s.quota.accountRepo == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.done = cancel, make(chan struct{})
	go func() { defer close(s.done); s.run(ctx) }()
}

func (s *OpenAIUsageSyncService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel == nil {
		return
	}
	s.cancel()
	<-s.done
	s.cancel, s.done = nil, nil
}

func (s *OpenAIUsageSyncService) run(ctx context.Context) {
	for ctx.Err() == nil {
		owner := uuid.NewString()
		var acquired bool
		if s.locks == nil {
			acquired = true // Focused tests; production always supplies Redis.
		} else if _, ok := s.locks.(RenewableLeaderLockCache); ok {
			callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			acquired, _ = s.locks.TryAcquireLeaderLock(callCtx, openAIUsageSyncLeaseKey, owner, s.opts.leaseTTL)
			cancel()
		}
		if acquired {
			s.runLeader(ctx, owner)
			if s.locks != nil {
				releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_ = s.locks.ReleaseLeaderLock(releaseCtx, openAIUsageSyncLeaseKey, owner)
				cancel()
			}
		}
		if !waitUsageSync(ctx, s.opts.leaseInterval) {
			return
		}
	}
}

func (s *OpenAIUsageSyncService) runLeader(parent context.Context, owner string) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	leaseDone := make(chan struct{})
	go func() {
		defer close(leaseDone)
		if s.locks == nil {
			<-ctx.Done()
			return
		}
		locks, ok := s.locks.(RenewableLeaderLockCache)
		if !ok {
			cancel()
			return
		}
		ticker := time.NewTicker(s.opts.leaseInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			callCtx, stop := context.WithTimeout(ctx, 2*time.Second)
			ok, err := locks.RenewLeaderLock(callCtx, openAIUsageSyncLeaseKey, owner, s.opts.leaseTTL)
			stop()
			if err != nil || !ok {
				cancel()
				return
			}
		}
	}()
	workers := make(map[int64]*openAIUsageSyncWorker)
	defer func() {
		cancel()
		for _, worker := range workers {
			worker.cancel()
		}
		for _, worker := range workers {
			<-worker.done
		}
		<-leaseDone
	}()
	permits := make(chan struct{}, s.opts.handshakes)
	ticker := time.NewTicker(s.opts.scanInterval)
	defer ticker.Stop()
	idle := time.NewTicker(s.opts.idleInterval)
	defer idle.Stop()
	s.reconcile(ctx, workers, permits)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reconcile(ctx, workers, permits)
		case <-s.wake:
			// Coalesce simultaneous first calls across a large pool. No request
			// waits for this scan or an upstream handshake.
			if !waitUsageSync(ctx, 100*time.Millisecond) {
				return
			}
			select {
			case <-s.wake:
			default:
			}
			s.reconcile(ctx, workers, permits)
		case <-idle.C:
			for id, worker := range workers {
				if used := s.lastActivity(id, worker.lastUsed); used.After(worker.lastUsed) {
					worker.lastUsed = used
				}
				if time.Since(worker.lastUsed) >= s.opts.idleWindow {
					worker.cancel()
					<-worker.done
					delete(workers, id)
				}
			}
		}
	}
}

func usageSyncIdentity(account *Account) string {
	proxy := ""
	if account.Proxy != nil {
		proxy = account.Proxy.URL()
	}
	encoded, err := json.Marshal(struct {
		Credentials map[string]any
		Route       any
		ParentID    *int64
	}{account.Credentials, accountEgressRoute(account, proxy), account.ParentAccountID})
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func (s *OpenAIUsageSyncService) reconcile(ctx context.Context, workers map[int64]*openAIUsageSyncWorker, permits chan struct{}) {
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	// This repository query reads all active accounts on the platform, with no
	// UI filters, page size or 256-account observer capacity limit.
	accounts, err := s.quota.accountRepo.ListByPlatform(callCtx, PlatformOpenAI)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("openai_usage_sync.account_list_failed")
		}
		return // A transient database failure must not tear down healthy WS.
	}
	byID := make(map[int64]*Account, len(accounts))
	for i := range accounts {
		byID[accounts[i].ID] = &accounts[i]
	}
	desired := make(map[int64]string, len(accounts))
	lastUsed := make(map[int64]time.Time, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if account.ID <= 0 || !account.IsOpenAIOAuth() || account.Status != StatusActive {
			continue
		}
		used := time.Time{}
		if account.LastUsedAt != nil {
			used = *account.LastUsedAt
		}
		used = s.lastActivity(account.ID, used)
		if used.IsZero() || time.Since(used) >= s.opts.idleWindow {
			continue
		}
		lastUsed[account.ID] = used
		credential := account
		if account.IsShadow() {
			credential = byID[*account.ParentAccountID]
			if credential == nil || credential.Status != StatusActive || !credential.IsOpenAIOAuth() {
				continue
			}
		}
		if identity := usageSyncIdentity(credential); identity != "" {
			desired[account.ID] = identity
		}
	}
	for id, worker := range workers {
		if lastUsed[id].After(worker.lastUsed) {
			worker.lastUsed = lastUsed[id]
		}
		exited := false
		select {
		case <-worker.done:
			exited = true
		default:
		}
		if identity, exists := desired[id]; exited || !exists || identity != worker.identity {
			worker.cancel()
			<-worker.done
			delete(workers, id)
		}
	}
	for id, identity := range desired {
		if ctx.Err() != nil {
			return
		}
		if _, exists := workers[id]; exists {
			continue
		}
		workerCtx, stop := context.WithCancel(ctx)
		worker := &openAIUsageSyncWorker{identity: identity, cancel: stop, done: make(chan struct{}), lastUsed: lastUsed[id]}
		workers[id] = worker
		go func() { defer close(worker.done); s.runAccount(workerCtx, id, permits) }()
	}
}

func waitUsageSync(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}
