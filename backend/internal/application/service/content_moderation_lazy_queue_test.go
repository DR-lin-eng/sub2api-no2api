//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContentModerationService_LazyQueueStartsEmptyAndPreservesFIFOOnGrow(t *testing.T) {
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil)
	t.Cleanup(svc.Stop)

	svc.asyncMu.Lock()
	initialCap := cap(svc.asyncBuf)
	svc.asyncMu.Unlock()
	require.Zero(t, initialCap, "idle moderation service must not preallocate async queue buffer")

	cfg := &ContentModerationConfig{QueueSize: 200}
	// Enqueue 40, pop 20 to advance head, then enqueue 80 to force wrap-around + grow (64 -> 128).
	for i := 0; i < 40; i++ {
		svc.enqueueAsync(ContentModerationCheckInput{RequestID: fmt.Sprintf("req-%d", i)}, cfg, ContentModerationInput{}, "")
	}
	for i := 0; i < 20; i++ {
		task, ok := svc.dequeueAsyncTask(context.Background(), 0)
		require.True(t, ok)
		require.Equal(t, fmt.Sprintf("req-%d", i), task.input.RequestID)
	}
	for i := 40; i < 120; i++ {
		svc.enqueueAsync(ContentModerationCheckInput{RequestID: fmt.Sprintf("req-%d", i)}, cfg, ContentModerationInput{}, "")
	}
	require.Equal(t, 100, svc.asyncQueueLen())

	for i := 20; i < 120; i++ {
		task, ok := svc.dequeueAsyncTask(context.Background(), 0)
		require.True(t, ok)
		require.Equal(t, fmt.Sprintf("req-%d", i), task.input.RequestID)
	}
	require.Zero(t, svc.asyncQueueLen())
}

func TestContentModerationService_LazyQueueRespectsConfiguredCap(t *testing.T) {
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil)
	t.Cleanup(svc.Stop)

	cfg := &ContentModerationConfig{QueueSize: 3}
	for i := 0; i < 5; i++ {
		svc.enqueueAsync(ContentModerationCheckInput{RequestID: fmt.Sprintf("r-%d", i)}, cfg, ContentModerationInput{}, "")
	}
	require.Equal(t, 3, svc.asyncQueueLen())
	require.Equal(t, int64(3), svc.asyncEnqueued.Load())
	require.Equal(t, int64(2), svc.asyncDropped.Load())
}

func BenchmarkContentModerationDequeueNoTimer(b *testing.B) {
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil)
	defer svc.Stop()
	cfg := &ContentModerationConfig{QueueSize: 1024}
	taskIn := ContentModerationCheckInput{RequestID: "bench"}
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		svc.enqueueAsync(taskIn, cfg, ContentModerationInput{}, "hash")
		_, ok := svc.dequeueAsyncTask(ctx, 0)
		if !ok {
			b.Fatal("expected dequeued task")
		}
	}
}
