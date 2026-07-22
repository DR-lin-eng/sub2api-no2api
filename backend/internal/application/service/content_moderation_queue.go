package service

import (
	"context"
	"log/slog"
	"time"
)

func (s *ContentModerationService) enqueueAsync(input ContentModerationCheckInput, cfg *ContentModerationConfig, content ContentModerationInput, hashText string) {
	if s == nil || s.asyncQueue == nil || s.stopped.Load() {
		return
	}
	queueSize := defaultContentModerationQueueSize
	if cfg != nil && cfg.QueueSize > 0 {
		queueSize = cfg.QueueSize
	}
	if len(s.asyncQueue) >= queueSize {
		slog.Warn("content_moderation.async_queue_full", "user_id", input.UserID, "endpoint", input.Endpoint, "queue_size", queueSize)
		s.asyncDropped.Add(1)
		return
	}
	task := &contentModerationTask{
		input:      input,
		content:    content,
		inputHash:  hashText,
		enqueuedAt: time.Now(),
	}
	select {
	case s.asyncQueue <- task:
		s.asyncEnqueued.Add(1)
	case <-s.lifecycleCtx.Done():
		s.asyncDropped.Add(1)
	default:
		slog.Warn("content_moderation.async_queue_full", "user_id", input.UserID, "endpoint", input.Endpoint)
		s.asyncDropped.Add(1)
	}
}

func (s *ContentModerationService) enqueueRecord(input ContentModerationCheckInput, cfg *ContentModerationConfig, log *ContentModerationLog, inputHash string, recordHash bool, applySideEffects bool) {
	if s == nil || s.asyncQueue == nil || log == nil || s.stopped.Load() {
		return
	}
	queueSize := defaultContentModerationQueueSize
	if cfg != nil && cfg.QueueSize > 0 {
		queueSize = cfg.QueueSize
	}
	if len(s.asyncQueue) >= queueSize {
		slog.Warn("content_moderation.record_queue_full",
			"user_id", input.UserID,
			"endpoint", input.Endpoint,
			"action", log.Action,
			"queue_size", queueSize)
		s.asyncDropped.Add(1)
		return
	}
	task := &contentModerationTask{
		input:            input,
		inputHash:        inputHash,
		log:              log,
		config:           cloneContentModerationConfig(cfg),
		recordHash:       recordHash,
		applySideEffects: applySideEffects,
		enqueuedAt:       time.Now(),
	}
	select {
	case s.asyncQueue <- task:
		s.asyncEnqueued.Add(1)
	case <-s.lifecycleCtx.Done():
		s.asyncDropped.Add(1)
	default:
		slog.Warn("content_moderation.record_queue_full",
			"user_id", input.UserID,
			"endpoint", input.Endpoint,
			"action", log.Action)
		s.asyncDropped.Add(1)
	}
}

func (s *ContentModerationService) worker(workerCtx context.Context, id int) {
	defer s.workerWG.Done()
	for {
		task, ok := s.dequeueAsyncTask(workerCtx, 0)
		if !ok {
			if workerCtx.Err() != nil {
				return
			}
			continue
		}
		if task == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(workerCtx, maxContentModerationTimeoutMS*time.Millisecond+10*time.Second)
		runtimeSnapshot, err := s.loadRuntimeSnapshot(ctx)
		if err != nil || runtimeSnapshot == nil || runtimeSnapshot.config == nil {
			cancel()
			s.asyncErrors.Add(1)
			continue
		}
		cfg := runtimeSnapshot.config
		func() {
			defer cancel()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("content_moderation.worker_panic", "worker_id", id, "recover", r)
				}
			}()
			if task.log != nil {
				s.asyncActive.Add(1)
				defer s.asyncActive.Add(-1)
				queueDelay := int(time.Since(task.enqueuedAt).Milliseconds())
				task.log.QueueDelayMS = &queueDelay
				taskCfg := task.config
				if taskCfg == nil {
					taskCfg = cfg
				}
				s.persistContentModerationLog(ctx, taskCfg, task.log, task.inputHash, task.recordHash, task.applySideEffects)
				s.asyncProcessed.Add(1)
				return
			}
			if !cfg.Enabled || cfg.Mode == ContentModerationModeOff || len(cfg.apiKeys()) == 0 {
				return
			}
			if !cfg.includesGroup(task.input.GroupID) {
				return
			}
			if !cfg.includesModel(task.input.Model) {
				return
			}
			s.asyncActive.Add(1)
			defer s.asyncActive.Add(-1)
			queueDelay := int(time.Since(task.enqueuedAt).Milliseconds())
			_ = s.checkSync(ctx, task.input, cfg, task.content, task.inputHash, &queueDelay, false)
			s.asyncProcessed.Add(1)
		}()
	}
}

func (s *ContentModerationService) dequeueAsyncTask(ctx context.Context, idleWait time.Duration) (*contentModerationTask, bool) {
	if s == nil || s.asyncQueue == nil {
		return nil, false
	}
	if idleWait <= 0 {
		select {
		case task, ok := <-s.asyncQueue:
			return task, ok
		case <-ctx.Done():
			return nil, false
		}
	}
	timer := time.NewTimer(idleWait)
	defer timer.Stop()
	select {
	case task, ok := <-s.asyncQueue:
		return task, ok
	case <-ctx.Done():
		return nil, false
	case <-timer.C:
		return nil, false
	}
}
