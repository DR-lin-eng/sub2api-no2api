package repository

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestOpenAIFailureCounterCacheIsAtomicAndResettable(t *testing.T) {
	mini, err := miniredis.Run()
	require.NoError(t, err)
	defer mini.Close()

	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	counter, ok := NewOpenAIFailureCounterCache(client).(*openAIFailureCounterCache)
	require.True(t, ok)

	const workers = 32
	var wg sync.WaitGroup
	results := make(chan int64, workers)
	errors := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			count, incrementErr := counter.IncrementOpenAIFailureCount(context.Background(), 77)
			if incrementErr != nil {
				errors <- incrementErr
				return
			}
			results <- count
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for incrementErr := range errors {
		require.NoError(t, incrementErr)
	}

	seen := make(map[int64]struct{}, workers)
	for count := range results {
		seen[count] = struct{}{}
	}
	require.Len(t, seen, workers, "each atomic increment should produce a distinct count")
	require.Equal(t, workers, len(seen))
	require.Greater(t, mini.TTL(openAIFailureCounterPrefix+"77"), time.Duration(0))

	require.NoError(t, counter.ResetOpenAIFailureCount(context.Background(), 77))
	require.Equal(t, int64(1), func() int64 {
		count, incrementErr := counter.IncrementOpenAIFailureCount(context.Background(), 77)
		require.NoError(t, incrementErr)
		return count
	}())
}

func TestOpenAIFailureCounterCacheLiveRedis(t *testing.T) {
	rawURL := os.Getenv("TEST_REDIS_URL")
	if rawURL == "" {
		t.Skip("TEST_REDIS_URL is not set")
	}
	options, err := redis.ParseURL(rawURL)
	require.NoError(t, err)
	client := redis.NewClient(options)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	counter, ok := NewOpenAIFailureCounterCache(client).(*openAIFailureCounterCache)
	require.True(t, ok)
	accountID := time.Now().UnixNano()
	t.Cleanup(func() { _ = counter.ResetOpenAIFailureCount(context.Background(), accountID) })

	for want := int64(1); want <= 3; want++ {
		got, incrementErr := counter.IncrementOpenAIFailureCount(context.Background(), accountID)
		require.NoError(t, incrementErr)
		require.Equal(t, want, got)
	}
	require.NoError(t, counter.ResetOpenAIFailureCount(context.Background(), accountID))
	got, err := counter.IncrementOpenAIFailureCount(context.Background(), accountID)
	require.NoError(t, err)
	require.Equal(t, int64(1), got)
}
