package oauthstate

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type testState struct {
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
}

func stateExpiry(value *testState) time.Time {
	return value.CreatedAt.Add(30 * time.Minute)
}

func TestRedisStoreSharesStateAcrossInstances(t *testing.T) {
	server := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: server.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = clientA.Close() })
	t.Cleanup(func() { _ = clientB.Close() })

	storeA := New(clientA, "oauth:test:", 30*time.Minute, stateExpiry)
	storeB := New(clientB, "oauth:test:", 30*time.Minute, stateExpiry)
	ctx := context.Background()

	require.NoError(t, storeA.SetContext(ctx, "session-1", &testState{Value: "shared", CreatedAt: time.Now()}))
	got, ok, err := storeB.GetContext(ctx, "session-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "shared", got.Value)

	require.NoError(t, storeB.DeleteContext(ctx, "session-1"))
	_, ok, err = storeA.GetContext(ctx, "session-1")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestRedisStoreExpiresState(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := New(client, "oauth:test:", 30*time.Minute, stateExpiry)

	require.NoError(t, store.SetContext(context.Background(), "expired", &testState{
		Value:     "old",
		CreatedAt: time.Now().Add(-31 * time.Minute),
	}))
	_, ok, err := store.GetContext(context.Background(), "expired")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestLocalStorePreservesSingleInstanceBehavior(t *testing.T) {
	store := New[testState](nil, "ignored:", 30*time.Minute, stateExpiry)
	t.Cleanup(store.Stop)

	store.Set("local", &testState{Value: "value", CreatedAt: time.Now()})
	got, ok := store.Get("local")
	require.True(t, ok)
	require.Equal(t, "value", got.Value)

	store.Delete("local")
	_, ok = store.Get("local")
	require.False(t, ok)
}
