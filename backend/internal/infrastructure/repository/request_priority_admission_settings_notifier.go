package repository

import (
	"context"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

const requestPriorityAdmissionSettingsChannel = "settings:request_priority_admission:updated:v1"

type requestPriorityAdmissionSettingsNotifier struct {
	redis *redis.Client
}

type requestPriorityAdmissionSettingsSubscription struct {
	pubsub    *redis.PubSub
	messages  <-chan struct{}
	cancel    context.CancelFunc
	closeOnce sync.Once
	closeErr  error
}

func NewRequestPriorityAdmissionSettingsNotifier(redisClient *redis.Client) service.RequestPriorityAdmissionSettingsNotifier {
	return &requestPriorityAdmissionSettingsNotifier{redis: redisClient}
}

func (n *requestPriorityAdmissionSettingsNotifier) Subscribe(ctx context.Context) service.RequestPriorityAdmissionSettingsSubscription {
	if n == nil || n.redis == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	pubsub := n.redis.Subscribe(ctx, requestPriorityAdmissionSettingsChannel)
	forwardCtx, cancel := context.WithCancel(ctx)
	events := make(chan struct{}, 1)
	rawMessages := pubsub.Channel()
	go func() {
		defer close(events)
		for {
			select {
			case <-forwardCtx.Done():
				return
			case _, ok := <-rawMessages:
				if !ok {
					return
				}
				select {
				case events <- struct{}{}:
				default:
				}
			}
		}
	}()

	return &requestPriorityAdmissionSettingsSubscription{
		pubsub:   pubsub,
		messages: events,
		cancel:   cancel,
	}
}

func (n *requestPriorityAdmissionSettingsNotifier) Publish(ctx context.Context) error {
	if n == nil || n.redis == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return n.redis.Publish(ctx, requestPriorityAdmissionSettingsChannel, "refresh").Err()
}

func (s *requestPriorityAdmissionSettingsSubscription) Messages() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.messages
}

func (s *requestPriorityAdmissionSettingsSubscription) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.cancel()
		s.closeErr = s.pubsub.Close()
	})
	return s.closeErr
}
