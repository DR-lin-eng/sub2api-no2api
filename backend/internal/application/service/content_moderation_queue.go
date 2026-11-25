package service

import (
	"context"
	"log/slog"
	"time"
)

func (s *ContentModerationService) enqueueAsync(input ContentModerationCheckInput, cfg *ContentModerationConfig, content ContentModerationInput, hashText string) {
	if s.queueDisabled() || s.stopped.Load() {
		return
	}
	task := contentModerationTask{
		input:      input,
		content:    content,
		inputHash:  hashText,
		enqueuedAt: time.Now(),
	}
	accepted, queueSize := s.enqueueTask(cfg, task)
	if accepted {
		s.asyncEnqueued.Add(1)
		return
	}
	slog.Warn("content_moderation.async_queue_full", "user_id", input.UserID, "endpoint", input.Endpoint, "queue_size", queueSize)
	s.asyncDropped.Add(1)
}

func (s *ContentModerationService) enqueueRecord(input ContentModerationCheckInput, cfg *ContentModerationConfig, log *ContentModerationLog, inputHash string, recordHash bool, applySideEffects bool) {
	if log == nil || s.queueDisabled() || s.stopped.Load() {
		return
	}
	accepted, queueSize := s.enqueueTask(cfg, contentModerationTask{
		input:            input,
		inputHash:        inputHash,
		log:              log,
		config:           cloneContentModerationConfig(cfg),
		recordHash:       recordHash,
		applySideEffects: applySideEffects,
		enqueuedAt:       time.Now(),
	})
	if accepted {
		s.asyncEnqueued.Add(1)
		return
	}
	slog.Warn("content_moderation.record_queue_full",
		"user_id", input.UserID,
		"endpoint", input.Endpoint,
		"action", log.Action,
		"queue_size", queueSize)
	s.asyncDropped.Add(1)
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
	if s == nil {
		return nil, false
	}
	if idleWait <= 0 {
		idleWait = time.Second
	}
	for {
		timer := time.NewTimer(idleWait)
		s.asyncQueueMu.RLock()
		queue := s.asyncQueue
		wake := s.asyncWake
		s.asyncQueueMu.RUnlock()
		if queue == nil {
			select {
			case <-wake:
				timer.Stop()
				continue
			case <-ctx.Done():
				timer.Stop()
				return nil, false
			case <-timer.C:
				return nil, false
			}
		}
		select {
		case task, ok := <-queue:
			timer.Stop()
			return task, ok
		case <-wake:
			timer.Stop()
			continue
		case <-ctx.Done():
			timer.Stop()
			return nil, false
		case <-timer.C:
			return nil, false
		}
	}
}

const contentModerationInitialQueueCapacity = 64

func (s *ContentModerationService) asyncQueueOf() chan *contentModerationTask {
	if s == nil {
		return nil
	}
	s.asyncQueueMu.RLock()
	queue := s.asyncQueue
	s.asyncQueueMu.RUnlock()
	return queue
}

func queueDropThreshold(cfg *ContentModerationConfig) int {
	threshold := defaultContentModerationQueueSize
	if cfg != nil && cfg.QueueSize > 0 {
		threshold = cfg.QueueSize
	}
	if threshold > maxContentModerationQueueSize {
		threshold = maxContentModerationQueueSize
	}
	if threshold < 1 {
		threshold = 1
	}
	return threshold
}

func (s *ContentModerationService) wakeWorkersLocked() {
	if s.asyncWake == nil {
		s.asyncWake = make(chan struct{})
		return
	}
	close(s.asyncWake)
	s.asyncWake = make(chan struct{})
}

func (s *ContentModerationService) growAsyncQueueLocked(threshold int) {
	current := s.asyncQueue
	if current == nil {
		return
	}
	next := cap(current) * 2
	if next < contentModerationInitialQueueCapacity {
		next = contentModerationInitialQueueCapacity
	}
	if next > threshold {
		next = threshold
	}
	if next <= cap(current) {
		return
	}
	grown := make(chan *contentModerationTask, next)
	for {
		select {
		case task := <-current:
			grown <- task
		default:
			s.asyncQueue = grown
			s.wakeWorkersLocked()
			return
		}
	}
}

func (s *ContentModerationService) queueDisabled() bool {
	if s == nil {
		return true
	}
	s.asyncQueueMu.RLock()
	defer s.asyncQueueMu.RUnlock()
	return s.asyncQueue == nil && !s.asyncQueueLazy
}

func (s *ContentModerationService) enqueueTask(cfg *ContentModerationConfig, task contentModerationTask) (bool, int) {
	if s == nil {
		return false, 0
	}
	threshold := queueDropThreshold(cfg)
	s.asyncQueueMu.Lock()
	defer s.asyncQueueMu.Unlock()
	if s.asyncQueue == nil {
		if !s.asyncQueueLazy {
			return false, threshold
		}
		initial := contentModerationInitialQueueCapacity
		if initial > threshold {
			initial = threshold
		}
		s.asyncQueue = make(chan *contentModerationTask, initial)
		s.wakeWorkersLocked()
	}
	if len(s.asyncQueue) >= threshold {
		return false, threshold
	}
	if len(s.asyncQueue) >= cap(s.asyncQueue) {
		s.growAsyncQueueLocked(threshold)
	}
	select {
	case s.asyncQueue <- &task:
		return true, threshold
	default:
		return false, threshold
	}
}
