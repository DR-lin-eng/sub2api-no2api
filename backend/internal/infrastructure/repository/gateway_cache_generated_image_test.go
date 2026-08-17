package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCacheGeneratedImageURLExpiresAfterThirtyMinutes(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := NewGatewayCache(rdb)
	store, ok := cache.(service.GeneratedImageURLStore)
	require.True(t, ok)

	ctx := context.Background()
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	rawURL := "https://cdn.example/generated/image.png"
	require.NoError(t, store.SetGeneratedImageURL(ctx, hash, rawURL, 30*time.Minute))
	value, found, err := store.GetGeneratedImageURL(ctx, hash)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, rawURL, value)
	require.Equal(t, 30*time.Minute, mr.TTL(buildGeneratedImageURLKey(hash)))

	mr.FastForward(30 * time.Minute)
	_, found, err = store.GetGeneratedImageURL(ctx, hash)
	require.NoError(t, err)
	require.False(t, found)
}

func TestGatewayCacheGeneratedImageRouteExpiresWithURL(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := NewGatewayCache(rdb)
	routeStore, ok := cache.(service.GeneratedImageRouteStore)
	require.True(t, ok)

	ctx := context.Background()
	hash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	route := platformegress.IPv6PoolRoute("2001:db8::50", 5, 2, false)
	require.NoError(t, routeStore.SetGeneratedImageRoute(ctx, hash, route, 50, 30*time.Minute))
	storedRoute, accountID, found, err := routeStore.GetGeneratedImageRoute(ctx, hash)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, route, storedRoute)
	require.Equal(t, int64(50), accountID)
	require.Equal(t, 30*time.Minute, mr.TTL(buildGeneratedImageRouteKey(hash)))

	mr.FastForward(30 * time.Minute)
	_, _, found, err = routeStore.GetGeneratedImageRoute(ctx, hash)
	require.NoError(t, err)
	require.False(t, found)
}
