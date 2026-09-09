package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
	"log/slog"
	"sync"
)

const channelCachePubSubKey = "channel_cache_updated"

type channelCacheNotifier struct{ rdb *redis.Client }

func NewChannelCache(rdb *redis.Client) service.ChannelCachePubSub {
	return &channelCacheNotifier{rdb: rdb}
}

func (c *channelCacheNotifier) NotifyUpdate(ctx context.Context) error {
	return c.rdb.Publish(ctx, channelCachePubSubKey, "refresh").Err()
}

func (c *channelCacheNotifier) SubscribeUpdates(handler func()) func() {
	ctx, cancel := context.WithCancel(context.Background())
	pubsub := c.rdb.Subscribe(ctx, channelCachePubSubKey)
	messages := pubsub.ChannelWithSubscriptions(redis.WithChannelSize(1))
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if err := pubsub.Close(); err != nil {
				slog.Warn("channel cache subscriber close failed", "error", err)
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-messages:
				if !ok {
					return
				}
				// Subscription acknowledgements invalidate after initial connect
				// or reconnect; normal messages invalidate after administrative writes.
				handler()
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { cancel(); <-done }) }
}
