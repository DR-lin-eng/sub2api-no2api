package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCodexSimulationGenerationAdvanceIsIdempotentLocally(t *testing.T) {
	store := &codexSimulationStateStore{
		ttl:     time.Minute,
		entries: make(map[string]codexSimulationStateBinding),
	}
	store.lastCleanupUnixNano.Store(time.Now().UnixNano())

	require.NoError(t, store.advanceGenerationWithTTL(context.Background(), "generation", 0, 1, time.Minute))
	require.NoError(t, store.advanceGenerationWithTTL(context.Background(), "generation", 0, 1, time.Minute))
	generation, err := store.generationWithTTL(context.Background(), "generation", time.Minute)
	require.NoError(t, err)
	require.Equal(t, uint64(1), generation)

	require.NoError(t, store.advanceGenerationWithTTL(context.Background(), "generation", 1, 2, time.Minute))
	generation, err = store.generationWithTTL(context.Background(), "generation", time.Minute)
	require.NoError(t, err)
	require.Equal(t, uint64(2), generation)
	require.NoError(t, store.advanceGenerationWithTTL(context.Background(), "generation", 0, 1, time.Minute))
	generation, err = store.generationWithTTL(context.Background(), "generation", time.Minute)
	require.NoError(t, err)
	require.Equal(t, uint64(2), generation, "stale completion must not move the window backwards")
}

func TestCodexSimulationGenerationAdvanceSerializesConcurrentLocalCompletions(t *testing.T) {
	store := &codexSimulationStateStore{
		ttl:     time.Minute,
		entries: make(map[string]codexSimulationStateBinding),
	}
	store.lastCleanupUnixNano.Store(time.Now().UnixNano())
	const workers = 32
	var wg sync.WaitGroup
	results := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- store.advanceGenerationWithTTL(context.Background(), "generation", 0, 1, time.Minute)
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	generation, err := store.generationWithTTL(context.Background(), "generation", time.Minute)
	require.NoError(t, err)
	require.Equal(t, uint64(1), generation)
}

type atomicGenerationCacheStub struct {
	mu          sync.Mutex
	generations map[string]uint64
}

func (s *atomicGenerationCacheStub) AdvanceOpenAIWSGeneration(_ context.Context, key string, expected, next uint64, _ time.Duration) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.generations[key]
	if current >= next {
		return current, nil
	}
	if current == expected {
		s.generations[key] = next
		return next, nil
	}
	return current, nil
}

func (s *atomicGenerationCacheStub) SetOpenAIWSState(context.Context, string, string, time.Duration) error {
	return nil
}

func (s *atomicGenerationCacheStub) GetOpenAIWSState(context.Context, string) (string, bool, error) {
	return "", false, nil
}

func (s *atomicGenerationCacheStub) DeleteOpenAIWSState(context.Context, string) error {
	return nil
}

func TestCodexSimulationGenerationAdvanceUsesAtomicSharedCache(t *testing.T) {
	shared := &atomicGenerationCacheStub{generations: make(map[string]uint64)}
	store := &codexSimulationStateStore{
		shared:  shared,
		ttl:     time.Minute,
		entries: make(map[string]codexSimulationStateBinding),
	}
	store.lastCleanupUnixNano.Store(time.Now().UnixNano())
	require.NoError(t, store.advanceGenerationWithTTL(context.Background(), "generation", 0, 1, time.Minute))
	require.NoError(t, store.advanceGenerationWithTTL(context.Background(), "generation", 0, 1, time.Minute))
	require.Equal(t, uint64(1), shared.generations["generation"])
}
