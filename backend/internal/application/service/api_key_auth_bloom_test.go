//go:build unit

package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type bloomTestCache struct {
	*authCacheStub
	check      func(context.Context, [sha256.Size]byte) (bool, error)
	initialize func(context.Context) error
}

func (c *bloomTestCache) MayContainAuthKey(ctx context.Context, d [sha256.Size]byte) (bool, error) {
	return c.check(ctx, d)
}
func (c *bloomTestCache) RegisterAuthKey(context.Context, string) error { return nil }
func (c *bloomTestCache) InitializeAuthKeyBloom(ctx context.Context, _ func(context.Context, func([][sha256.Size]byte) error) error) error {
	if c.initialize != nil {
		return c.initialize(ctx)
	}
	return nil
}

type bloomTestRepo struct {
	*authRepoStub
	cache APIKeyAuthBloomCache
}

func (r *bloomTestRepo) SetAuthKeyBloomCache(c APIKeyAuthBloomCache) { r.cache = c }
func (r *bloomTestRepo) WalkAuthKeyDigests(context.Context, func([][sha256.Size]byte) error) error {
	return nil
}

func TestAPIKeyBloomUniqueRandomKeysSkipBackingLookups(t *testing.T) {
	var database, redis atomic.Int64
	r := &bloomTestRepo{authRepoStub: &authRepoStub{getByKeyForAuth: func(context.Context, string) (*APIKey, error) { database.Add(1); return nil, ErrAPIKeyNotFound }}}
	c := &bloomTestCache{authCacheStub: &authCacheStub{getAuthCache: func(context.Context, string) (*APIKeyAuthCacheEntry, error) {
		redis.Add(1)
		return nil, errors.New("miss")
	}}, check: func(context.Context, [sha256.Size]byte) (bool, error) { return false, nil }}
	cfg := &config.Config{APIKeyAuth: config.APIKeyAuthCacheConfig{Bloom: config.APIKeyAuthBloomConfig{Enabled: true}, L2TTLSeconds: 60, Singleflight: true}}
	svc := NewAPIKeyService(r, nil, nil, nil, nil, c, cfg)
	for i := 0; i < 10000; i++ {
		_, err := svc.GetByKey(context.Background(), fmt.Sprintf("sk-random-%064x", i))
		require.ErrorIs(t, err, ErrAPIKeyNotFound)
	}
	t.Logf("random-invalid requests=10000 redis_l2=%d database=%d bloom_rejected=%d", redis.Load(), database.Load(), svc.AuthBloomHealth().Rejected)
	require.Zero(t, database.Load())
	require.Zero(t, redis.Load())
	require.EqualValues(t, 10000, svc.AuthBloomHealth().Rejected)
}

func TestAPIKeyBloomRandomAttackComparison(t *testing.T) {
	run := func(enabled bool) (redisCalls, databaseCalls int64) {
		var redis, database atomic.Int64
		repo := &bloomTestRepo{authRepoStub: &authRepoStub{getByKeyForAuth: func(context.Context, string) (*APIKey, error) {
			database.Add(1)
			return nil, ErrAPIKeyNotFound
		}}}
		cache := &bloomTestCache{authCacheStub: &authCacheStub{getAuthCache: func(context.Context, string) (*APIKeyAuthCacheEntry, error) {
			redis.Add(1)
			return nil, errors.New("miss")
		}}, check: func(context.Context, [sha256.Size]byte) (bool, error) { return false, nil }}
		cfg := &config.Config{APIKeyAuth: config.APIKeyAuthCacheConfig{Bloom: config.APIKeyAuthBloomConfig{Enabled: enabled}, L2TTLSeconds: 60, Singleflight: true}}
		svc := NewAPIKeyService(repo, nil, nil, nil, nil, cache, cfg)
		for i := 0; i < 2000; i++ {
			_, _ = svc.GetByKey(context.Background(), fmt.Sprintf("sk-comparison-%064x", i))
		}
		t.Logf("comparison bloom_enabled=%t requests=2000 redis_l2=%d database=%d", enabled, redis.Load(), database.Load())
		return redis.Load(), database.Load()
	}
	withoutRedis, withoutDatabase := run(false)
	withRedis, withDatabase := run(true)
	require.EqualValues(t, 2000, withoutRedis)
	require.EqualValues(t, 2000, withoutDatabase)
	require.Zero(t, withRedis)
	require.Zero(t, withDatabase)
}

func TestAPIKeyBloomFalsePositiveAndFailureUseExactLookup(t *testing.T) {
	for _, checkErr := range []error{nil, errors.New("cold or unavailable")} {
		t.Run(fmt.Sprint(checkErr), func(t *testing.T) {
			calls := 0
			r := &bloomTestRepo{authRepoStub: &authRepoStub{getByKeyForAuth: func(_ context.Context, key string) (*APIKey, error) {
				calls++
				if key == "sk-valid" {
					return &APIKey{ID: 1, UserID: 2, Status: StatusActive, User: &User{ID: 2, Status: StatusActive}}, nil
				}
				return nil, ErrAPIKeyNotFound
			}}}
			c := &bloomTestCache{authCacheStub: &authCacheStub{}, check: func(context.Context, [sha256.Size]byte) (bool, error) { return true, checkErr }}
			svc := NewAPIKeyService(r, nil, nil, nil, nil, c, &config.Config{APIKeyAuth: config.APIKeyAuthCacheConfig{Bloom: config.APIKeyAuthBloomConfig{Enabled: true}}})
			_, err := svc.GetByKey(context.Background(), "sk-false-positive")
			require.ErrorIs(t, err, ErrAPIKeyNotFound)
			key, err := svc.GetByKey(context.Background(), "sk-valid")
			require.NoError(t, err)
			require.EqualValues(t, 1, key.ID)
			require.Equal(t, 2, calls)
		})
	}
}

func TestAPIKeyBloomDoesNotAddRedisToHotCacheHits(t *testing.T) {
	c := &bloomTestCache{authCacheStub: &authCacheStub{}, check: func(context.Context, [sha256.Size]byte) (bool, error) {
		t.Fatal("hot cache hit reached Bloom Redis")
		return false, nil
	}}
	r := &bloomTestRepo{authRepoStub: &authRepoStub{}}
	svc := NewAPIKeyService(r, nil, nil, nil, nil, c, &config.Config{APIKeyAuth: config.APIKeyAuthCacheConfig{Bloom: config.APIKeyAuthBloomConfig{Enabled: true}, L1Size: 16, L1TTLSeconds: 60}})
	t.Cleanup(func() { svc.authCacheL1.Close() })
	entry := &APIKeyAuthCacheEntry{Snapshot: &APIKeyAuthSnapshot{Version: apiKeyAuthSnapshotVersion, APIKeyID: 7, User: APIKeyAuthUserSnapshot{ID: 3, Status: StatusActive}}}
	svc.setAuthHotCacheEntry(authCacheDigest("sk-hot"), "sk-hot", entry)
	key, err := svc.GetByKey(context.Background(), "sk-hot")
	require.NoError(t, err)
	require.EqualValues(t, 7, key.ID)
}

func TestAPIKeyBloomWorkerStopsWithLocalCachesDisabled(t *testing.T) {
	started := make(chan struct{})
	c := &bloomTestCache{authCacheStub: &authCacheStub{}, initialize: func(ctx context.Context) error { close(started); <-ctx.Done(); return ctx.Err() }}
	r := &bloomTestRepo{authRepoStub: &authRepoStub{}}
	svc := NewAPIKeyService(r, nil, nil, nil, nil, c, &config.Config{APIKeyAuth: config.APIKeyAuthCacheConfig{Bloom: config.APIKeyAuthBloomConfig{Enabled: true}}})
	svc.StartAuthCacheInvalidationSubscriber(context.Background())
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Bloom worker did not start")
	}
	done := make(chan struct{})
	go func() {
		svc.StopAuthCacheInvalidationSubscriber()
		svc.StopAuthCacheInvalidationSubscriber()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Bloom worker did not stop")
	}
}
