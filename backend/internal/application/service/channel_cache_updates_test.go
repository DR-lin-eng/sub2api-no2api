//go:build unit

package service

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"sync/atomic"
	"testing"
	"time"
)

type channelUpdateBusStub struct {
	callbacks            []func()
	notifications, stops int
}

func (b *channelUpdateBusStub) NotifyUpdate(context.Context) error {
	b.notifications++
	for _, callback := range b.callbacks {
		callback()
	}
	return nil
}
func (b *channelUpdateBusStub) SubscribeUpdates(callback func()) func() {
	b.callbacks = append(b.callbacks, callback)
	return func() { b.stops++ }
}

func TestChannelCacheUpdatesPropagateWithoutNotificationLoop(t *testing.T) {
	bus := &channelUpdateBusStub{}
	repo := &mockChannelRepository{}
	first := ProvideChannelService(repo, nil, nil, nil, bus)
	second := ProvideChannelService(repo, nil, nil, nil, bus)
	t.Cleanup(first.Stop)
	t.Cleanup(second.Stop)
	old, err := second.loadCache(context.Background())
	require.NoError(t, err)
	first.InvalidateCache()
	fresh, err := second.loadCache(context.Background())
	require.NoError(t, err)
	require.NotSame(t, old, fresh)
	require.Equal(t, 1, bus.notifications)
	first.Stop()
	first.Stop()
	second.Stop()
	require.Equal(t, 2, bus.stops)
}

func TestChannelCacheInvalidationRejectsOldInFlightReload(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "old_success", true: "old_failure"}[failed], func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			repo := &mockChannelRepository{listAllFn: func(context.Context) ([]Channel, error) {
				if calls.Add(1) == 1 {
					close(entered)
					<-release
					if failed {
						return nil, errors.New("old database failure")
					}
					return []Channel{{ID: 1, Name: "old"}}, nil
				}
				return []Channel{{ID: 1, Name: "new"}}, nil
			}}
			svc := NewChannelService(repo, nil, nil, nil)
			result := make(chan error, 1)
			go func() { _, err := svc.buildCache(context.Background()); result <- err }()
			<-entered
			svc.InvalidateCache()
			close(release)
			select {
			case err := <-result:
				require.ErrorIs(t, err, errChannelCacheInvalidated)
			case <-time.After(time.Second):
				t.Fatal("old reload did not finish")
			}
			cache, err := svc.loadCache(context.Background())
			require.NoError(t, err)
			require.Equal(t, "new", cache.byID[1].Name)
		})
	}
}
