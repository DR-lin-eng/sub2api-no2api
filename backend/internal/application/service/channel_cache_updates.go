package service

import (
	"context"
	"log/slog"
	"time"
)

// ChannelCachePubSub invalidates local pricing and mapping snapshots across nodes.
// SubscribeUpdates returns an idempotent stop function. Reconnection must also
// signal invalidation because Redis Pub/Sub does not replay missed updates.
type ChannelCachePubSub interface {
	NotifyUpdate(context.Context) error
	SubscribeUpdates(func()) func()
}

func ProvideChannelService(repo ChannelRepository, groupRepo GroupRepository, auth APIKeyAuthCacheInvalidator, pricing *PricingService, bus ChannelCachePubSub) *ChannelService {
	s := NewChannelService(repo, groupRepo, auth, pricing)
	s.cachePubSub = bus
	if bus != nil {
		s.stopCacheUpdates = bus.SubscribeUpdates(s.clearCache)
	}
	return s
}

// clearCache never publishes, so receiving our own notification cannot loop.
func (s *ChannelService) clearCache() {
	s.cacheMu.Lock()
	s.cacheGeneration++
	s.cache.Store((*channelCache)(nil))
	s.cacheMu.Unlock()
	s.cacheSF.Forget("channel_cache")
}

func (s *ChannelService) notifyCacheUpdate() {
	if s.cachePubSub == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.cachePubSub.NotifyUpdate(ctx); err != nil {
		slog.Warn("channel cache invalidation publish failed", "error", err)
	}
}

func (s *ChannelService) Stop() {
	if s == nil {
		return
	}
	s.stopCacheOnce.Do(func() {
		if s.stopCacheUpdates != nil {
			s.stopCacheUpdates()
		}
	})
}
