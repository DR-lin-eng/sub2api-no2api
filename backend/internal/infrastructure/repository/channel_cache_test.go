package repository

import (
	"context"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"sync/atomic"
	"testing"
	"time"
)

func TestChannelCacheSubscriberReceivesUpdatesAndStops(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	bus := NewChannelCache(client)
	var calls atomic.Int32
	stop := bus.SubscribeUpdates(func() { calls.Add(1) })
	t.Cleanup(stop)
	require.Eventually(t, func() bool { return calls.Load() >= 1 }, time.Second, time.Millisecond)
	before := calls.Load()
	require.NoError(t, bus.NotifyUpdate(context.Background()))
	require.Eventually(t, func() bool { return calls.Load() > before }, time.Second, time.Millisecond)
	stop()
	stop()
	stopped := calls.Load()
	require.NoError(t, bus.NotifyUpdate(context.Background()))
	require.Equal(t, stopped, calls.Load())
}
