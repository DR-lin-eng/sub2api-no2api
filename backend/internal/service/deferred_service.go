package service

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

const (
	apiKeyLastUsedFlushInterval = time.Second
	apiKeyLastUsedBatchSize     = 1000
	apiKeyLastUsedBatchesPerRun = 8
	apiKeyLastUsedPendingLimit  = 32768
)

type apiKeyLastUsedBatchRepository interface {
	BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error
}

// DeferredService provides deferred batch update functionality
type DeferredService struct {
	accountRepo AccountRepository
	apiKeyRepo  APIKeyRepository
	timingWheel *TimingWheelService
	interval    time.Duration

	lastUsedUpdates       sync.Map
	apiKeyLastUsedUpdates sync.Map
	apiKeyLastUsedPending atomic.Int64
	stopOnce              sync.Once
}

// NewDeferredService creates a new DeferredService instance
func NewDeferredService(accountRepo AccountRepository, timingWheel *TimingWheelService, interval time.Duration) *DeferredService {
	return &DeferredService{
		accountRepo: accountRepo,
		timingWheel: timingWheel,
		interval:    interval,
	}
}

// Start starts the deferred service
func (s *DeferredService) Start() {
	s.timingWheel.ScheduleRecurring("deferred:last_used", s.interval, s.flushLastUsed)
	s.timingWheel.ScheduleRecurring("deferred:api_key_last_used", apiKeyLastUsedFlushInterval, s.flushAPIKeyLastUsed)
	log.Printf("[DeferredService] Started (interval: %v)", s.interval)
}

func (s *DeferredService) SetAPIKeyRepository(apiKeyRepo APIKeyRepository) {
	s.apiKeyRepo = apiKeyRepo
}

// Stop stops the deferred service
func (s *DeferredService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		s.timingWheel.Cancel("deferred:last_used")
		s.timingWheel.Cancel("deferred:api_key_last_used")
		s.flushLastUsed()
		s.flushAPIKeyLastUsed()
		log.Printf("[DeferredService] Service stopped")
	})
}

func (s *DeferredService) ScheduleLastUsedUpdate(accountID int64) {
	s.lastUsedUpdates.Store(accountID, time.Now())
}

func (s *DeferredService) ScheduleAPIKeyLastUsedUpdate(apiKeyID int64, usedAt time.Time) bool {
	if s == nil || s.apiKeyRepo == nil || apiKeyID <= 0 {
		return false
	}
	for {
		existing, loaded := s.apiKeyLastUsedUpdates.LoadOrStore(apiKeyID, usedAt)
		if !loaded {
			if s.apiKeyLastUsedPending.Add(1) > apiKeyLastUsedPendingLimit {
				if s.apiKeyLastUsedUpdates.CompareAndDelete(apiKeyID, usedAt) {
					s.apiKeyLastUsedPending.Add(-1)
				}
				return false
			}
			return true
		}
		existingAt, ok := existing.(time.Time)
		if !ok || !existingAt.Before(usedAt) {
			return true
		}
		if s.apiKeyLastUsedUpdates.CompareAndSwap(apiKeyID, existing, usedAt) {
			return true
		}
	}
}

func (s *DeferredService) CancelAPIKeyLastUsedUpdate(apiKeyID int64) {
	if s == nil || apiKeyID <= 0 {
		return
	}
	if _, loaded := s.apiKeyLastUsedUpdates.LoadAndDelete(apiKeyID); loaded {
		s.apiKeyLastUsedPending.Add(-1)
	}
}

func (s *DeferredService) flushLastUsed() {
	updates := make(map[int64]time.Time)
	s.lastUsedUpdates.Range(func(key, value any) bool {
		id, ok := key.(int64)
		if !ok {
			return true
		}
		ts, ok := value.(time.Time)
		if !ok {
			return true
		}
		updates[id] = ts
		s.lastUsedUpdates.Delete(key)
		return true
	})

	if len(updates) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.accountRepo.BatchUpdateLastUsed(ctx, updates); err != nil {
		log.Printf("[DeferredService] BatchUpdateLastUsed failed (%d accounts): %v", len(updates), err)
		for id, ts := range updates {
			s.lastUsedUpdates.Store(id, ts)
		}
	} else {
		log.Printf("[DeferredService] BatchUpdateLastUsed flushed %d accounts", len(updates))
	}
}

func (s *DeferredService) flushAPIKeyLastUsed() {
	if s == nil || s.apiKeyRepo == nil {
		return
	}
	for batch := 0; batch < apiKeyLastUsedBatchesPerRun; batch++ {
		updates := s.takeAPIKeyLastUsedBatch(apiKeyLastUsedBatchSize)
		if len(updates) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := s.flushAPIKeyLastUsedBatch(ctx, updates)
		cancel()
		if err == nil {
			continue
		}
		log.Printf("[DeferredService] BatchUpdateAPIKeyLastUsed failed (%d keys): %v", len(updates), err)
		for id, ts := range updates {
			_ = s.ScheduleAPIKeyLastUsedUpdate(id, ts)
		}
		return
	}
}

func (s *DeferredService) takeAPIKeyLastUsedBatch(limit int) map[int64]time.Time {
	updates := make(map[int64]time.Time, limit)
	s.apiKeyLastUsedUpdates.Range(func(key, value any) bool {
		id, idOK := key.(int64)
		ts, tsOK := value.(time.Time)
		if !idOK || !tsOK {
			if s.apiKeyLastUsedUpdates.CompareAndDelete(key, value) {
				s.apiKeyLastUsedPending.Add(-1)
			}
			return len(updates) < limit
		}
		if s.apiKeyLastUsedUpdates.CompareAndDelete(id, ts) {
			s.apiKeyLastUsedPending.Add(-1)
			updates[id] = ts
		}
		return len(updates) < limit
	})
	return updates
}

func (s *DeferredService) flushAPIKeyLastUsedBatch(ctx context.Context, updates map[int64]time.Time) error {
	if batchRepo, ok := s.apiKeyRepo.(apiKeyLastUsedBatchRepository); ok {
		return batchRepo.BatchUpdateLastUsed(ctx, updates)
	}
	for id, ts := range updates {
		if err := s.apiKeyRepo.UpdateLastUsed(ctx, id, ts); err != nil {
			return err
		}
	}
	return nil
}
