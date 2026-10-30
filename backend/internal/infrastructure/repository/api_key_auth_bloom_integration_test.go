//go:build integration

package repository

import (
	"context"
	"crypto/sha256"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyBloomIntegration_RealRedisAndPostgres(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	cache := &apiKeyCache{rdb: rdb}

	// A cold/partial filter must fail open for exact authentication.
	maybe, err := cache.MayContainAuthKey(ctx, sha256.Sum256([]byte("missing")))
	require.Error(t, err)
	require.True(t, maybe)

	user := mustCreateUser(t, testEntClient(t), &service.User{Email: "bloom-real-redis@example.com"})
	repo := newAPIKeyRepositoryWithSQL(testEntClient(t), integrationDB)
	repo.SetAuthKeyBloomCache(cache)
	key := &service.APIKey{UserID: user.ID, Key: "sk-real-redis-bloom-key", Name: "Bloom", Status: service.StatusActive}
	require.NoError(t, repo.Create(ctx, key))

	// The pre-insert path registers the credential, but readiness is only
	// published after a complete PostgreSQL scan.
	require.NoError(t, cache.InitializeAuthKeyBloom(ctx, repo.WalkAuthKeyDigests))
	maybe, err = cache.MayContainAuthKey(ctx, sha256.Sum256([]byte(key.Key)))
	require.NoError(t, err)
	require.True(t, maybe)
	maybe, err = cache.MayContainAuthKey(ctx, sha256.Sum256([]byte("random-invalid-key")))
	require.NoError(t, err)
	require.False(t, maybe)

	// A second cache client observes the same ready membership filter.
	other := &apiKeyCache{rdb: rdb}
	maybe, err = other.MayContainAuthKey(ctx, sha256.Sum256([]byte(key.Key)))
	require.NoError(t, err)
	require.True(t, maybe)

	// Rebuilding while a new key is registered must preserve both members.
	key2 := &service.APIKey{UserID: user.ID, Key: "sk-real-redis-bloom-key-2", Name: "Bloom 2", Status: service.StatusActive}
	require.NoError(t, repo.Create(ctx, key2))
	require.NoError(t, cache.InitializeAuthKeyBloom(ctx, repo.WalkAuthKeyDigests))
	for _, value := range []string{key.Key, key2.Key} {
		maybe, err = cache.MayContainAuthKey(ctx, sha256.Sum256([]byte(value)))
		require.NoError(t, err)
		require.True(t, maybe, value)
	}

	// Confirm the database rows are visible to the same repository used by auth.
	got, err := repo.GetByKeyForAuth(ctx, key.Key)
	require.NoError(t, err)
	require.Equal(t, key.ID, got.ID)

	// Redis loss is reported as an indeterminate result. The service layer
	// treats this error as a fail-open signal and continues with exact lookup.
	require.NoError(t, rdb.Close())
	maybe, err = cache.MayContainAuthKey(ctx, sha256.Sum256([]byte("random-after-redis-loss")))
	require.Error(t, err)
	require.True(t, maybe)

	// Keep the generated client referenced so this test fails loudly if the
	// fixture changes its transaction ownership assumptions.
	var _ *dbent.Client = testEntClient(t)
}
