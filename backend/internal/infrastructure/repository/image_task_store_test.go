package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

var benchmarkImageTaskRecord *service.ImageTaskRecord

func BenchmarkImageTaskStoreGet(b *testing.B) {
	mr := miniredis.RunT(b)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	b.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()
	task := &service.ImageTaskRecord{
		ID:     "imgtask_benchmark",
		Status: service.ImageTaskStatusProcessing,
	}

	b.Run("redis", func(b *testing.B) {
		store, ok := NewImageTaskStore(rdb).(*imageTaskStore)
		require.True(b, ok)
		require.NoError(b, store.Save(ctx, task, time.Hour))
		store.l1 = nil
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchmarkImageTaskRecord, _ = store.Get(ctx, task.ID)
		}
	})

	b.Run("l1", func(b *testing.B) {
		store, ok := NewImageTaskStore(rdb).(*imageTaskStore)
		require.True(b, ok)
		require.NoError(b, store.Save(ctx, task, time.Hour))
		store.l1.Wait()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			benchmarkImageTaskRecord, _ = store.Get(ctx, task.ID)
		}
	})
}

func TestImageTaskStoreRoundTripAndTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	task := &service.ImageTaskRecord{
		ID:        "imgtask_123",
		UserID:    7,
		APIKeyID:  9,
		Status:    service.ImageTaskStatusProcessing,
		CreatedAt: 100,
		ExpiresAt: 200,
	}

	require.NoError(t, store.Save(context.Background(), task, 24*time.Hour))
	got, err := store.Get(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, task, got)
	require.Equal(t, 24*time.Hour, mr.TTL(imageTaskKey(task.ID)))
}

func TestImageTaskStoreMissing(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)

	_, err := store.Get(context.Background(), "imgtask_missing")
	require.ErrorIs(t, err, service.ErrImageTaskNotFound)
}

func TestImageTaskStoreTerminalReadUsesBoundedLocalCache(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store, ok := NewImageTaskStore(rdb).(*imageTaskStore)
	require.True(t, ok)
	task := &service.ImageTaskRecord{
		ID:     "imgtask_cached",
		Status: service.ImageTaskStatusCompleted,
		Result: json.RawMessage(`{"data":[{"url":"https://cdn.test/image.png"}]}`),
	}

	require.NoError(t, store.Save(context.Background(), task, time.Hour))
	store.l1.Wait()
	require.True(t, mr.Del(imageTaskKey(task.ID)))

	got, err := store.Get(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, task, got)
}

func TestImageTaskStoreProcessingCacheExpiresAndReturnsToRedis(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store, ok := NewImageTaskStore(rdb).(*imageTaskStore)
	require.True(t, ok)
	store.processingCacheTTL = 25 * time.Millisecond
	task := &service.ImageTaskRecord{ID: "imgtask_processing", Status: service.ImageTaskStatusProcessing}

	require.NoError(t, store.Save(context.Background(), task, time.Hour))
	store.l1.Wait()
	require.True(t, mr.Del(imageTaskKey(task.ID)))

	got, err := store.Get(context.Background(), task.ID)
	require.NoError(t, err, "poll burst should hit the local processing cache")
	require.Equal(t, task, got)

	require.Eventually(t, func() bool {
		_, err := store.Get(context.Background(), task.ID)
		return errors.Is(err, service.ErrImageTaskNotFound)
	}, time.Second, 10*time.Millisecond, "processing state must return to Redis after the short TTL")
}

func TestImageTaskStoreRejectsOversizedSerializedRecord(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewImageTaskStore(rdb)
	task := &service.ImageTaskRecord{
		ID:     "imgtask_oversized",
		Status: service.ImageTaskStatusCompleted,
		Result: json.RawMessage(`"` + strings.Repeat("x", service.MaxImageTaskRecordBytes) + `"`),
	}

	err := store.Save(context.Background(), task, time.Hour)
	require.Error(t, err)
	require.Contains(t, err.Error(), "record exceeds")
	require.False(t, mr.Exists(imageTaskKey(task.ID)))
}
