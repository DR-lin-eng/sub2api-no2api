package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/servertiming"
)

func NewContentModerationService(
	settingRepo SettingRepository,
	repo ContentModerationRepository,
	hashCache ContentModerationHashCache,
	groupRepo GroupRepository,
	userRepo UserRepository,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
	emailService *EmailService,
) *ContentModerationService {
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	svc := &ContentModerationService{
		settingRepo:          settingRepo,
		repo:                 repo,
		hashCache:            hashCache,
		groupRepo:            groupRepo,
		userRepo:             userRepo,
		authCacheInvalidator: authCacheInvalidator,
		emailService:         emailService,
		httpClient:           servertiming.InstrumentClient(nil),
		asyncQueue:           make(chan *contentModerationTask, maxContentModerationQueueSize),
		lifecycleCtx:         lifecycleCtx,
		lifecycleCancel:      lifecycleCancel,
		workerCancels:        make(map[int]context.CancelFunc),
		keyHealth:            make(map[string]*contentModerationKeyHealth),
	}
	if settingRepo != nil && repo != nil {
		workerCount := defaultContentModerationWorkerCount
		loadCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if cfg, err := svc.loadConfig(loadCtx); err == nil && cfg != nil {
			workerCount = cfg.WorkerCount
		}
		cancel()
		svc.resizeWorkers(workerCount)
		svc.workerWG.Add(1)
		go svc.cleanupWorker(lifecycleCtx)
	}
	return svc
}

func (s *ContentModerationService) resizeWorkers(count int) {
	if s == nil || s.stopped.Load() || s.asyncQueue == nil {
		return
	}
	if count <= 0 {
		count = defaultContentModerationWorkerCount
	}
	if count > maxContentModerationWorkerCount {
		count = maxContentModerationWorkerCount
	}

	s.workerMu.Lock()
	defer s.workerMu.Unlock()
	if s.stopped.Load() {
		return
	}
	for id, cancel := range s.workerCancels {
		if id >= count {
			cancel()
			delete(s.workerCancels, id)
		}
	}
	for id := 0; id < count; id++ {
		if _, exists := s.workerCancels[id]; exists {
			continue
		}
		workerCtx, cancel := context.WithCancel(s.lifecycleCtx)
		s.workerCancels[id] = cancel
		s.workerWG.Add(1)
		go s.worker(workerCtx, id)
	}
}

// Stop releases all moderation workers and the periodic cleanup loop.
func (s *ContentModerationService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		s.stopped.Store(true)
		if s.lifecycleCancel != nil {
			s.lifecycleCancel()
		}
		s.workerMu.Lock()
		for id, cancel := range s.workerCancels {
			cancel()
			delete(s.workerCancels, id)
		}
		s.workerMu.Unlock()
		s.workerWG.Wait()
		for {
			select {
			case <-s.asyncQueue:
			default:
				return
			}
		}
	})
}
