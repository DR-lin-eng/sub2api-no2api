package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestPasskeySessionStoreConsumesSessionOnce(t *testing.T) {
	ctx := context.Background()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	store := NewPasskeySessionStore(client)
	token, err := store.Store(ctx, &service.PasskeySession{
		Kind:   "login",
		UserID: 42,
	}, 5*time.Minute)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.Equal(t, 5*time.Minute, server.TTL(passkeySessionPrefix+token))

	session, err := store.Consume(ctx, token)
	require.NoError(t, err)
	require.Equal(t, "login", session.Kind)
	require.Equal(t, int64(42), session.UserID)

	_, err = store.Consume(ctx, token)
	require.ErrorIs(t, err, service.ErrPasskeySession)
}
