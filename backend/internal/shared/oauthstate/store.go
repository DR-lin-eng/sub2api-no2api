// Package oauthstate provides short-lived OAuth flow state storage.
package oauthstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultOperationTimeout = 2 * time.Second

type localEntry struct {
	payload   []byte
	expiresAt time.Time
}

// Store keeps OAuth state in Redis when a client is configured and otherwise
// falls back to an in-process map for single-instance and unit-test use.
type Store[T any] struct {
	redisClient *redis.Client
	keyPrefix   string
	ttl         time.Duration
	expiresAt   func(*T) time.Time

	mu       sync.RWMutex
	local    map[string]localEntry
	stopCh   chan struct{}
	stopOnce sync.Once
}

// New creates an OAuth state store. keyPrefix must be unique per provider so
// structurally similar session payloads cannot be read by the wrong flow.
func New[T any](redisClient *redis.Client, keyPrefix string, ttl time.Duration, expiresAt func(*T) time.Time) *Store[T] {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	store := &Store[T]{
		redisClient: redisClient,
		keyPrefix:   keyPrefix,
		ttl:         ttl,
		expiresAt:   expiresAt,
		stopCh:      make(chan struct{}),
	}
	if redisClient == nil {
		store.local = make(map[string]localEntry)
		go store.cleanupLocal()
	}
	return store
}

// Set stores a value using a bounded background context. Callers that need to
// surface Redis errors should use SetContext.
func (s *Store[T]) Set(sessionID string, value *T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultOperationTimeout)
	defer cancel()
	_ = s.SetContext(ctx, sessionID, value)
}

func (s *Store[T]) SetContext(ctx context.Context, sessionID string, value *T) error {
	if s == nil || value == nil {
		return errors.New("oauth state value is nil")
	}
	if sessionID == "" {
		return errors.New("oauth state session id is empty")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal oauth state: %w", err)
	}
	expiresAt := time.Now().Add(s.ttl)
	if s.expiresAt != nil {
		expiresAt = s.expiresAt(value)
	}
	remaining := time.Until(expiresAt)
	if remaining <= 0 {
		_ = s.DeleteContext(ctx, sessionID)
		return nil
	}

	if s.redisClient != nil {
		if ctx == nil {
			ctx = context.Background()
		}
		if err := s.redisClient.Set(ctx, s.keyPrefix+sessionID, payload, remaining).Err(); err != nil {
			return fmt.Errorf("store oauth state in redis: %w", err)
		}
		return nil
	}

	s.mu.Lock()
	s.local[sessionID] = localEntry{payload: payload, expiresAt: expiresAt}
	s.mu.Unlock()
	return nil
}

// Get retrieves a value using a bounded background context. Callers that need
// to distinguish a Redis failure from a cache miss should use GetContext.
func (s *Store[T]) Get(sessionID string) (*T, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultOperationTimeout)
	defer cancel()
	value, ok, _ := s.GetContext(ctx, sessionID)
	return value, ok
}

func (s *Store[T]) GetContext(ctx context.Context, sessionID string) (*T, bool, error) {
	if s == nil || sessionID == "" {
		return nil, false, nil
	}
	var payload []byte
	if s.redisClient != nil {
		if ctx == nil {
			ctx = context.Background()
		}
		value, err := s.redisClient.Get(ctx, s.keyPrefix+sessionID).Bytes()
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("load oauth state from redis: %w", err)
		}
		payload = value
	} else {
		s.mu.RLock()
		entry, ok := s.local[sessionID]
		s.mu.RUnlock()
		if !ok {
			return nil, false, nil
		}
		if !time.Now().Before(entry.expiresAt) {
			_ = s.DeleteContext(ctx, sessionID)
			return nil, false, nil
		}
		payload = entry.payload
	}

	var value T
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, false, fmt.Errorf("unmarshal oauth state: %w", err)
	}
	if s.expiresAt != nil && !time.Now().Before(s.expiresAt(&value)) {
		_ = s.DeleteContext(ctx, sessionID)
		return nil, false, nil
	}
	return &value, true, nil
}

func (s *Store[T]) Delete(sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultOperationTimeout)
	defer cancel()
	_ = s.DeleteContext(ctx, sessionID)
}

func (s *Store[T]) DeleteContext(ctx context.Context, sessionID string) error {
	if s == nil || sessionID == "" {
		return nil
	}
	if s.redisClient != nil {
		if ctx == nil {
			ctx = context.Background()
		}
		if err := s.redisClient.Del(ctx, s.keyPrefix+sessionID).Err(); err != nil {
			return fmt.Errorf("delete oauth state from redis: %w", err)
		}
		return nil
	}
	s.mu.Lock()
	delete(s.local, sessionID)
	s.mu.Unlock()
	return nil
}

func (s *Store[T]) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stopCh) })
}

// Done is closed after Stop and is primarily useful for lifecycle checks.
func (s *Store[T]) Done() <-chan struct{} {
	if s == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return s.stopCh
}

func (s *Store[T]) cleanupLocal() {
	interval := s.ttl / 2
	if interval <= 0 || interval > 5*time.Minute {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.mu.Lock()
			for id, entry := range s.local {
				if !now.Before(entry.expiresAt) {
					delete(s.local, id)
				}
			}
			s.mu.Unlock()
		}
	}
}
