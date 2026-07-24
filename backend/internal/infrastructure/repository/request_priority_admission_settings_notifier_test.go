package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRequestPriorityAdmissionSettingsNotifier(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	notifier := NewRequestPriorityAdmissionSettingsNotifier(redisClient)
	subscription := notifier.Subscribe(context.Background())
	require.NotNil(t, subscription)
	t.Cleanup(func() { require.NoError(t, subscription.Close()) })

	require.Eventually(t, func() bool {
		return redisClient.PubSubNumSub(context.Background(), requestPriorityAdmissionSettingsChannel).Val()[requestPriorityAdmissionSettingsChannel] == 1
	}, time.Second, time.Millisecond)
	require.NoError(t, notifier.Publish(context.Background()))
	require.Eventually(t, func() bool {
		select {
		case <-subscription.Messages():
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
}
