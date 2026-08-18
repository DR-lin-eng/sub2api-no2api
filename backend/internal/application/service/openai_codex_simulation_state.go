package service

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	codexSimulationStatePrefix      = "codex:simulation:v1:"
	codexSimulationStateMaxEntries  = 65536
	codexSimulationCleanupInterval  = time.Minute
	codexSimulationCleanupScanLimit = 512
	codexSimulationDefaultStateTTL  = 7 * 24 * time.Hour
)

type codexSimulationStateBinding struct {
	value     string
	expiresAt time.Time
}

// codexSimulationStateStore keeps only short-lived derived state. Redis is
// shared across replicas when GatewayCache implements OpenAIWSSharedStateCache;
// the bounded local copy preserves best-effort continuity during cache errors.
type codexSimulationStateStore struct {
	shared OpenAIWSSharedStateCache
	ttl    time.Duration

	mu      sync.RWMutex
	entries map[string]codexSimulationStateBinding

	lastCleanupUnixNano atomic.Int64
}

func newCodexSimulationStateStore(cache GatewayCache, ttl time.Duration) *codexSimulationStateStore {
	ttl = normalizeCodexSimulationStateTTL(ttl)
	shared, _ := cache.(OpenAIWSSharedStateCache)
	store := &codexSimulationStateStore{
		shared:  shared,
		ttl:     ttl,
		entries: make(map[string]codexSimulationStateBinding, 256),
	}
	store.lastCleanupUnixNano.Store(time.Now().UnixNano())
	return store
}

func (s *codexSimulationStateStore) get(ctx context.Context, key string) (string, bool, error) {
	if s == nil {
		return "", false, nil
	}
	return s.getWithTTL(ctx, key, s.ttl)
}

func (s *codexSimulationStateStore) getWithTTL(ctx context.Context, key string, ttl time.Duration) (string, bool, error) {
	if s == nil || strings.TrimSpace(key) == "" {
		return "", false, nil
	}
	ttl = normalizeCodexSimulationStateTTL(ttl)
	s.maybeCleanup()
	var sharedErr error
	if s.shared != nil {
		cacheCtx, cancel := withOpenAIWSStateStoreRedisTimeout(ctx)
		value, found, err := s.shared.GetOpenAIWSState(cacheCtx, key)
		cancel()
		if err == nil && found && strings.TrimSpace(value) != "" {
			s.setLocalWithTTL(key, value, ttl)
			return value, true, nil
		}
		sharedErr = err
	}

	value, found := s.getLocal(key)
	if found {
		return value, true, sharedErr
	}
	return "", false, sharedErr
}

func (s *codexSimulationStateStore) set(ctx context.Context, key, value string) error {
	if s == nil {
		return nil
	}
	return s.setWithTTL(ctx, key, value, s.ttl)
}

func (s *codexSimulationStateStore) setWithTTL(ctx context.Context, key, value string, ttl time.Duration) error {
	if s == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return nil
	}
	ttl = normalizeCodexSimulationStateTTL(ttl)
	s.maybeCleanup()
	s.setLocalWithTTL(key, value, ttl)
	if s.shared == nil {
		return nil
	}
	cacheCtx, cancel := withOpenAIWSStateStoreRedisTimeout(ctx)
	defer cancel()
	return s.shared.SetOpenAIWSState(cacheCtx, key, value, ttl)
}

func (s *codexSimulationStateStore) generationWithTTL(ctx context.Context, key string, ttl time.Duration) (uint64, error) {
	value, found, err := s.getWithTTL(ctx, key, ttl)
	if !found {
		return 0, err
	}
	generation, parseErr := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if parseErr != nil {
		return 0, parseErr
	}
	return generation, err
}

func (s *codexSimulationStateStore) setGenerationWithTTL(ctx context.Context, key string, generation uint64, ttl time.Duration) error {
	return s.setWithTTL(ctx, key, strconv.FormatUint(generation, 10), ttl)
}

func (s *codexSimulationStateStore) setLocalWithTTL(key, value string, ttl time.Duration) {
	if s == nil {
		return
	}
	ttl = normalizeCodexSimulationStateTTL(ttl)
	s.mu.Lock()
	ensureBindingCapacity(s.entries, key, codexSimulationStateMaxEntries)
	s.entries[key] = codexSimulationStateBinding{value: value, expiresAt: time.Now().Add(ttl)}
	s.mu.Unlock()
}

func normalizeCodexSimulationStateTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return codexSimulationDefaultStateTTL
	}
	return ttl
}

func (s *codexSimulationStateStore) getLocal(key string) (string, bool) {
	if s == nil {
		return "", false
	}
	now := time.Now()
	s.mu.RLock()
	binding, found := s.entries[key]
	s.mu.RUnlock()
	if !found || now.After(binding.expiresAt) || strings.TrimSpace(binding.value) == "" {
		return "", false
	}
	return binding.value, true
}

func (s *codexSimulationStateStore) maybeCleanup() {
	if s == nil {
		return
	}
	now := time.Now()
	last := time.Unix(0, s.lastCleanupUnixNano.Load())
	if now.Sub(last) < codexSimulationCleanupInterval ||
		!s.lastCleanupUnixNano.CompareAndSwap(last.UnixNano(), now.UnixNano()) {
		return
	}

	s.mu.Lock()
	scanned := 0
	for key, binding := range s.entries {
		if now.After(binding.expiresAt) {
			delete(s.entries, key)
		}
		scanned++
		if scanned >= codexSimulationCleanupScanLimit {
			break
		}
	}
	s.mu.Unlock()
}

func (s *OpenAIGatewayService) codexSimulationStateTTL() time.Duration {
	return s.codexSimulationSettingsSnapshot(context.Background(), nil).stateTTL()
}

func (s *OpenAIGatewayService) getCodexSimulationStateStore() *codexSimulationStateStore {
	if s == nil {
		return nil
	}
	s.codexSimulationStateStoreOnce.Do(func() {
		if s.codexSimulationStateStore == nil {
			s.codexSimulationStateStore = newCodexSimulationStateStore(s.cache, s.codexSimulationStateTTL())
		}
	})
	return s.codexSimulationStateStore
}
