package service

import (
	"context"
	"log/slog"
	"time"
)

const initialContentModerationAsyncQueueCap = 64

func (s *ContentModerationService) asyncQueueLen() int {
	if s == nil {
		return 0
	}
	s.asyncMu.Lock()
	n := s.asyncSize
	s.asyncMu.Unlock()
	return n
}

func (s *ContentModerationService) pushAsyncTask(task *contentModerationTask, maxSize int) bool {
	if s == nil || task == nil || s.asyncNotify == nil || s.stopped.Load() {
		return false
	}
	if maxSize <= 0 {
		maxSize = defaultContentModerationQueueSize
	}
	if maxSize > maxContentModerationQueueSize {
		maxSize = maxContentModerationQueueSize
	}

	s.asyncMu.Lock()
	if s.stopped.Load() || s.asyncSize >= maxSize {
		s.asyncMu.Unlock()
		return false
	}
	if s.asyncSize == len(s.asyncBuf) {
		nextCap := initialContentModerationAsyncQueueCap
		if len(s.asyncBuf) > 0 {
			nextCap = len(s.asyncBuf) * 2
		}
		if nextCap > maxSize {
			nextCap = maxSize
		}
		if nextCap < s.asyncSize+1 {
			nextCap = s.asyncSize + 1
		}
		grown := make([]*contentModerationTask, nextCap)
		if s.asyncSize > 0 {
			first := len(s.asyncBuf) - s.asyncHead
			if first > s.asyncSize {
				first = s.asyncSize
			}
			copy(grown, s.asyncBuf[s.asyncHead:s.asyncHead+first])
			if s.asyncSize > first {
				copy(grown[first:], s.asyncBuf[:s.asyncSize-first])
			}
		}
		s.asyncBuf = grown
		s.asyncHead = 0
	}
	tail := (s.asyncHead + s.asyncSize) % len(s.asyncBuf)
	s.asyncBuf[tail] = task
	s.asyncSize++
	s.asyncMu.Unlock()

	select {
	case s.asyncNotify <- struct{}{}:
	default:
	}
	return true
}

func (s *ContentModerationService) popAsyncTask() (*contentModerationTask, bool) {
	if s == nil {
		return nil, false
	}
	s.asyncMu.Lock()
	if s.asyncSize == 0 {
		s.asyncMu.Unlock()
		return nil, false
	}
	task := s.asyncBuf[s.asyncHead]
	s.asyncBuf[s.asyncHead] = nil
	s.asyncHead = (s.asyncHead + 1) % len(s.asyncBuf)
	s.asyncSize--
	more := s.asyncSize > 0
	if s.asyncSize == 0 {
		s.asyncHead = 0
	}
	s.asyncMu.Unlock()

	if more && s.asyncNotify != nil {
		select {
		case s.asyncNotify <- struct{}{}:
		default:
		}
	}
	return task, true
}

func (s *ContentModerationService) enqueueAsync(input ContentModerationCheckInput, cfg *ContentModerationConfig, content ContentModerationInput, hashText string) {
	if s == nil || s.asyncNotify == nil || s.stopped.Load() {
		return
	}
	queueSize := defaultContentModerationQueueSize
	if cfg != nil && cfg.QueueSize > 0 {
		queueSize = cfg.QueueSize
	}
	task := &contentModerationTask{
		input:      input,
		content:    content,
		inputHash:  hashText,
		enqueuedAt: time.Now(),
	}
	if !s.pushAsyncTask(task, queueSize) {
		if !s.stopped.Load() {
			slog.Warn("content_moderation.async_queue_full", "user_id", input.UserID, "endpoint", input.Endpoint, "queue_size", queueSize)
		}
		s.asyncDropped.Add(1)
		return
	}
	s.asyncEnqueued.Add(1)
}

func (s *ContentModerationService) enqueueRecord(input ContentModerationCheckInput, cfg *ContentModerationConfig, log *ContentModerationLog, inputHash string, recordHash bool, applySideEffects bool) {
	if s == nil || s.asyncNotify == nil || log == nil || s.stopped.Load() {
		return
	}
	queueSize := defaultContentModerationQueueSize
	if cfg != nil && cfg.QueueSize > 0 {
		queueSize = cfg.QueueSize
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
	if !s.pushAsyncTask(task, queueSize) {
		if !s.stopped.Load() {
			slog.Warn("content_moderation.record_queue_full",
				"user_id", input.UserID,
				"endpoint", input.Endpoint,
				"action", log.Action,
				"queue_size", queueSize)
		}
		s.asyncDropped.Add(1)
		return
	}
	s.asyncEnqueued.Add(1)
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
	if s == nil || s.asyncNotify == nil {
		return nil, false
	}
	if task, ok := s.popAsyncTask(); ok {
		return task, true
	}
	if idleWait <= 0 {
		for {
			select {
			case <-s.asyncNotify:
				if task, ok := s.popAsyncTask(); ok {
					return task, true
				}
			case <-ctx.Done():
				return nil, false
			}
		}
	}
	timer := time.NewTimer(idleWait)
	defer timer.Stop()
	for {
		select {
		case <-s.asyncNotify:
			if task, ok := s.popAsyncTask(); ok {
				return task, true
			}
		case <-ctx.Done():
			return nil, false
		case <-timer.C:
			return nil, false
		}
	}
}
