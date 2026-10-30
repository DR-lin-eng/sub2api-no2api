package repository

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func bloomCacheFixture(t *testing.T) (*apiKeyCache, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return &apiKeyCache{rdb: client}, server
}
func bloomDigestLoader(keys ...string) func(context.Context, func([][sha256.Size]byte) error) error {
	return func(_ context.Context, visit func([][sha256.Size]byte) error) error {
		digests := make([][sha256.Size]byte, len(keys))
		for i, key := range keys {
			digests[i] = sha256.Sum256([]byte(key))
		}
		return visit(digests)
	}
}
func TestAPIKeyBloomCacheColdStartAndCrossInstanceRegistration(t *testing.T) {
	ctx := context.Background()
	cache, server := bloomCacheFixture(t)
	_, err := cache.MayContainAuthKey(ctx, sha256.Sum256([]byte("known")))
	require.Error(t, err)
	require.NoError(t, cache.RegisterAuthKey(ctx, "registered-before-scan"))
	_, err = cache.MayContainAuthKey(ctx, sha256.Sum256([]byte("absent")))
	require.Error(t, err, "partial filter must never reject")
	require.NoError(t, cache.InitializeAuthKeyBloom(ctx, bloomDigestLoader("known")))
	other := &apiKeyCache{rdb: redis.NewClient(&redis.Options{Addr: server.Addr()})}
	t.Cleanup(func() { _ = other.rdb.Close() })
	require.NoError(t, other.RegisterAuthKey(ctx, "new-on-other-node"))
	for _, key := range []string{"known", "registered-before-scan", "new-on-other-node"} {
		maybe, err := cache.MayContainAuthKey(ctx, sha256.Sum256([]byte(key)))
		require.NoError(t, err)
		require.True(t, maybe, key)
	}
	for i := 0; i < 100; i++ {
		maybe, err := cache.MayContainAuthKey(ctx, sha256.Sum256([]byte(fmt.Sprint("random-", i))))
		require.NoError(t, err)
		require.False(t, maybe)
	}
	size, err := cache.rdb.StrLen(ctx, authKeyBloomRedisKey).Result()
	require.NoError(t, err)
	require.EqualValues(t, authKeyBloomBytes, size)
}
func TestAPIKeyBloomCacheIncompleteScanNeverBecomesReady(t *testing.T) {
	ctx := context.Background()
	cache, _ := bloomCacheFixture(t)
	require.Error(t, cache.InitializeAuthKeyBloom(ctx, func(ctx context.Context, visit func([][sha256.Size]byte) error) error {
		require.NoError(t, bloomDigestLoader("partial")(ctx, visit))
		return errors.New("scan failed")
	}))
	_, err := cache.MayContainAuthKey(ctx, sha256.Sum256([]byte("not-scanned")))
	require.Error(t, err)
	require.NoError(t, cache.InitializeAuthKeyBloom(ctx, bloomDigestLoader("partial", "complete")))
	maybe, err := cache.MayContainAuthKey(ctx, sha256.Sum256([]byte("complete")))
	require.NoError(t, err)
	require.True(t, maybe)
}

func TestAPIKeyBloomCacheRepairsCorruptBitmapBeforeRegistration(t *testing.T) {
	ctx := context.Background()
	cache, _ := bloomCacheFixture(t)
	require.NoError(t, cache.rdb.Set(ctx, authKeyBloomRedisKey, "corrupt", 0).Err())
	require.NoError(t, cache.RegisterAuthKey(ctx, "registered-after-corruption"))
	_, err := cache.MayContainAuthKey(ctx, sha256.Sum256([]byte("registered-after-corruption")))
	require.Error(t, err, "corrupt values remain unready until a complete scan")
	require.NoError(t, cache.InitializeAuthKeyBloom(ctx, bloomDigestLoader("registered-after-corruption")))
	maybe, err := cache.MayContainAuthKey(ctx, sha256.Sum256([]byte("registered-after-corruption")))
	require.NoError(t, err)
	require.True(t, maybe)
}
func TestAPIKeyBloomCacheEvictionAndExpiredBuilderAreFenced(t *testing.T) {
	for _, loseBitmap := range []bool{true, false} {
		t.Run(fmt.Sprint(loseBitmap), func(t *testing.T) {
			ctx := context.Background()
			cache, server := bloomCacheFixture(t)
			err := cache.InitializeAuthKeyBloom(ctx, func(ctx context.Context, visit func([][sha256.Size]byte) error) error {
				require.NoError(t, bloomDigestLoader("old")(ctx, visit))
				if loseBitmap {
					require.NoError(t, cache.rdb.Del(ctx, authKeyBloomRedisKey).Err())
					require.NoError(t, cache.RegisterAuthKey(ctx, "new"))
				} else {
					server.FastForward(31 * time.Second)
				}
				return nil
			})
			require.Error(t, err, "stale builder must not publish readiness")
			_, err = cache.MayContainAuthKey(ctx, sha256.Sum256([]byte("any")))
			require.Error(t, err)
			require.NoError(t, cache.InitializeAuthKeyBloom(ctx, bloomDigestLoader("old", "new")))
			for _, key := range []string{"old", "new"} {
				maybe, err := cache.MayContainAuthKey(ctx, sha256.Sum256([]byte(key)))
				require.NoError(t, err)
				require.True(t, maybe)
			}
		})
	}
}
