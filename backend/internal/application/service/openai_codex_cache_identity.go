package service

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// OpenAICodexSessionCache atomically keeps the first UUIDv7 for a virtual
// conversation. Ordinary get/set cannot preserve its creation time when two
// replicas see the conversation concurrently for the first time.
type OpenAICodexSessionCache interface {
	GetOrCreateCodexSessionID(ctx context.Context, key, candidate string, ttl time.Duration) (string, error)
}

func (s *codexSimulationStateStore) sessionIDWithTTL(ctx context.Context, key, candidate string, ttl time.Duration) string {
	if s == nil || key == "" {
		return candidate
	}
	ttl = normalizeCodexSimulationStateTTL(ttl)
	s.maybeCleanup()
	// Seed Redis with a known local identity after a transient shared-cache
	// failure or eviction, so a warm process can preserve the original session.
	if local, found := s.getLocal(key); found && validCodexSessionID(local) {
		candidate = local
	}
	if shared, ok := s.shared.(OpenAICodexSessionCache); ok {
		cacheCtx, cancel := withOpenAIWSStateStoreRedisTimeout(ctx)
		value, err := shared.GetOrCreateCodexSessionID(cacheCtx, key, candidate, ttl)
		cancel()
		if err == nil && validCodexSessionID(value) {
			s.setLocalWithTTL(key, value, ttl)
			return value
		}
	}

	// Keep the existing bounded, expiring local fallback. Creation and TTL
	// refresh share the lock so concurrent requests cannot replace the winner.
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if binding, found := s.entries[key]; found && now.Before(binding.expiresAt) && validCodexSessionID(binding.value) {
		candidate = binding.value
	}
	ensureBindingCapacity(s.entries, key, codexSimulationStateMaxEntries)
	s.entries[key] = codexSimulationStateBinding{value: candidate, expiresAt: now.Add(ttl)}
	return candidate
}

func validCodexSessionID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122
}

func codexSimulationSessionStateKey(rootKey, principalKey string) string {
	if rootKey == "" || principalKey == "" {
		return ""
	}
	return codexSimulationStatePrefix + "session:" + rootKey + ":" + principalKey
}
