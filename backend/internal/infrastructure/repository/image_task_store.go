package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/dgraph-io/ristretto"
	"github.com/redis/go-redis/v9"
)

const (
	imageTaskKeyPrefix          = "image_task:"
	imageTaskProcessingCacheTTL = 2 * time.Second
	imageTaskTerminalCacheTTL   = 5 * time.Minute
	imageTaskLocalCacheMaxCost  = 32 << 20
)

type imageTaskStore struct {
	rdb                *redis.Client
	l1                 *ristretto.Cache
	processingCacheTTL time.Duration
	terminalCacheTTL   time.Duration
}

func NewImageTaskStore(rdb *redis.Client) service.ImageTaskStore {
	store := &imageTaskStore{
		rdb:                rdb,
		processingCacheTTL: imageTaskProcessingCacheTTL,
		terminalCacheTTL:   imageTaskTerminalCacheTTL,
	}
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 100_000,
		MaxCost:     imageTaskLocalCacheMaxCost,
		BufferItems: 64,
	})
	if err == nil {
		store.l1 = cache
	}
	return store
}

func (s *imageTaskStore) Save(ctx context.Context, task *service.ImageTaskRecord, ttl time.Duration) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	if len(data) > service.MaxImageTaskRecordBytes {
		return fmt.Errorf("image task record exceeds %d bytes", service.MaxImageTaskRecordBytes)
	}
	if err := s.rdb.Set(ctx, imageTaskKey(task.ID), data, ttl).Err(); err != nil {
		return err
	}
	s.cache(task, len(data))
	return nil
}

func (s *imageTaskStore) Get(ctx context.Context, id string) (*service.ImageTaskRecord, error) {
	id = strings.TrimSpace(id)
	if s.l1 != nil {
		if value, ok := s.l1.Get(id); ok {
			if task, ok := value.(*service.ImageTaskRecord); ok {
				return cloneImageTaskRecord(task), nil
			}
		}
	}
	data, err := s.rdb.Get(ctx, imageTaskKey(id)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrImageTaskNotFound
		}
		return nil, err
	}
	task, err := decodeImageTask(data)
	if err != nil {
		return nil, err
	}
	s.cache(task, len(data))
	return task, nil
}

func decodeImageTask(data []byte) (*service.ImageTaskRecord, error) {
	var task service.ImageTaskRecord
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

func cloneImageTaskRecord(task *service.ImageTaskRecord) *service.ImageTaskRecord {
	if task == nil {
		return nil
	}
	cloned := *task
	cloned.Result = append(json.RawMessage(nil), task.Result...)
	cloned.Error = append(json.RawMessage(nil), task.Error...)
	if task.CompletedAt != nil {
		completedAt := *task.CompletedAt
		cloned.CompletedAt = &completedAt
	}
	return &cloned
}

func (s *imageTaskStore) cache(task *service.ImageTaskRecord, cost int) {
	if s == nil || s.l1 == nil || task == nil || cost <= 0 {
		return
	}
	ttl := s.processingCacheTTL
	if ttl <= 0 {
		ttl = imageTaskProcessingCacheTTL
	}
	if task.Status == service.ImageTaskStatusCompleted || task.Status == service.ImageTaskStatusFailed {
		ttl = s.terminalCacheTTL
		if ttl <= 0 {
			ttl = imageTaskTerminalCacheTTL
		}
	}
	s.l1.SetWithTTL(task.ID, cloneImageTaskRecord(task), int64(cost), ttl)
}

func imageTaskKey(id string) string {
	return imageTaskKeyPrefix + strings.TrimSpace(id)
}
