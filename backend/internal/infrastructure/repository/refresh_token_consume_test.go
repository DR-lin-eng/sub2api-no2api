package repository

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRefreshTokenConsumeHasOneWinnerAndPreservesOtherSessions(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewRefreshTokenCache(client)
	ctx := context.Background()
	data := &service.RefreshTokenData{UserID: 42, FamilyID: "family", BindingHash: "binding", ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, cache.StoreRefreshToken(ctx, "old", data, time.Hour))
	require.NoError(t, cache.StoreRefreshToken(ctx, "other-device", data, time.Hour))
	var winners atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 32 {
		wg.Go(func() {
			<-start
			consumed, err := cache.ConsumeRefreshToken(ctx, "old")
			if err == nil {
				winners.Add(1)
				require.Equal(t, data.UserID, consumed.UserID)
				require.Equal(t, data.BindingHash, consumed.BindingHash)
			} else {
				require.ErrorIs(t, err, service.ErrRefreshTokenNotFound)
			}
		})
	}
	close(start)
	wg.Wait()
	require.Equal(t, int32(1), winners.Load())
	_, err := cache.GetRefreshToken(ctx, "old")
	require.ErrorIs(t, err, service.ErrRefreshTokenNotFound)
	_, err = cache.GetRefreshToken(ctx, "other-device")
	require.NoError(t, err)
}

func TestRefreshTokenConsumeRejectsCorruptAndExpiredData(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewRefreshTokenCache(client)
	ctx := context.Background()
	require.NoError(t, client.Set(ctx, refreshTokenKey("corrupt"), "not-json", time.Hour).Err())
	_, err := cache.ConsumeRefreshToken(ctx, "corrupt")
	require.Error(t, err)
	_, err = cache.ConsumeRefreshToken(ctx, "corrupt")
	require.ErrorIs(t, err, service.ErrRefreshTokenNotFound)
	_, err = cache.ConsumeRefreshToken(ctx, "missing")
	require.ErrorIs(t, err, service.ErrRefreshTokenNotFound)
}
