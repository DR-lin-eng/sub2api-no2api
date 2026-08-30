package service

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/google/uuid"
)

const (
	supportChatRetentionCleanupInterval = 10 * time.Minute
	supportChatRetentionRunTimeout      = 2 * time.Minute
	supportChatRetentionLockTTL         = 5 * time.Minute
	supportChatRetentionBatchSize       = 500
	supportChatRetentionMaxBatches      = 100
	supportChatRetentionLockKey         = "support_chat:retention_cleanup"
)

type supportChatRetentionCleaner interface {
	CleanupExpiredMessages(context.Context, time.Time, int) (chat.RetentionCleanupResult, error)
}

type supportChatRetentionPolicy interface {
	GetSupportChatRetentionDays(context.Context) (int, error)
}

// SupportChatRetentionService periodically removes expired ordinary support
// messages. A shared leader lock elects one worker across instances, while the
// repository still uses bounded SKIP LOCKED batches for rolling-upgrade safety.
type SupportChatRetentionService struct {
	cleaner supportChatRetentionCleaner
	policy  supportChatRetentionPolicy

	lockCache LeaderLockCache
	db        *sql.DB
	instance  string

	interval   time.Duration
	runTimeout time.Duration
	batchSize  int
	maxBatches int

	startOnce sync.Once
	stopOnce  sync.Once
	started   atomic.Bool
	stopCh    chan struct{}
	doneCh    chan struct{}
	runCtx    context.Context
	runCancel context.CancelFunc
}

func NewSupportChatRetentionService(
	chatService *chat.Service,
	settingService *SettingService,
) *SupportChatRetentionService {
	runCtx, runCancel := context.WithCancel(context.Background())
	return &SupportChatRetentionService{
		cleaner:    chatService,
		policy:     settingService,
		instance:   uuid.NewString(),
		interval:   supportChatRetentionCleanupInterval,
		runTimeout: supportChatRetentionRunTimeout,
		batchSize:  supportChatRetentionBatchSize,
		maxBatches: supportChatRetentionMaxBatches,
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
		runCtx:     runCtx,
		runCancel:  runCancel,
	}
}

func (s *SupportChatRetentionService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

func (s *SupportChatRetentionService) Start() {
	if s == nil || s.cleaner == nil || s.policy == nil || s.interval <= 0 {
		return
	}
	s.startOnce.Do(func() {
		if s.runCtx == nil || s.runCancel == nil {
			s.runCtx, s.runCancel = context.WithCancel(context.Background())
		}
		if s.stopCh == nil {
			s.stopCh = make(chan struct{})
		}
		if s.doneCh == nil {
			s.doneCh = make(chan struct{})
		}
		s.started.Store(true)
		go s.runLoop()
	})
}

func (s *SupportChatRetentionService) Stop() {
	if s == nil {
		return
	}
	if !s.started.Load() {
		return
	}
	s.stopOnce.Do(func() {
		if s.runCancel != nil {
			s.runCancel()
		}
		close(s.stopCh)
	})
	select {
	case <-s.doneCh:
	case <-time.After(3 * time.Second):
		logger.LegacyPrintf("service.support_chat_retention", "[SupportChatRetention] stop timed out")
	}
}

func (s *SupportChatRetentionService) runLoop() {
	defer close(s.doneCh)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.runScheduled()
	for {
		select {
		case <-ticker.C:
			s.runScheduled()
		case <-s.stopCh:
			return
		}
	}
}

func (s *SupportChatRetentionService) runScheduled() {
	parent := s.runCtx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, s.runTimeout)
	defer cancel()
	result, err := s.RunOnce(ctx, time.Now().UTC())
	if err != nil {
		logger.LegacyPrintf("service.support_chat_retention", "[SupportChatRetention] cleanup failed: %v", err)
		return
	}
	if result.MessagesDeleted > 0 || result.AssetsDeleted > 0 {
		logger.LegacyPrintf(
			"service.support_chat_retention",
			"[SupportChatRetention] cleanup complete messages=%d assets=%d",
			result.MessagesDeleted,
			result.AssetsDeleted,
		)
	}
}

// RunOnce applies the currently persisted policy. It is exported for bounded
// maintenance tests and intentionally treats zero days as a no-op.
func (s *SupportChatRetentionService) RunOnce(
	ctx context.Context,
	now time.Time,
) (chat.RetentionCleanupResult, error) {
	result := chat.RetentionCleanupResult{}
	if s == nil || s.cleaner == nil || s.policy == nil {
		return result, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	days, err := s.policy.GetSupportChatRetentionDays(ctx)
	if err != nil {
		return result, fmt.Errorf("load support chat retention policy: %w", err)
	}
	if days == 0 {
		return result, nil
	}
	if days < 0 || days > SupportChatRetentionDaysMax {
		return result, fmt.Errorf("support chat retention days out of range: %d", days)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	release, acquired := tryAcquireSingletonLeaderLock(
		ctx,
		s.lockCache,
		s.db,
		supportChatRetentionLockKey,
		s.instance,
		supportChatRetentionLockTTL,
	)
	if !acquired {
		return result, nil
	}
	defer release()

	for batch := 0; batch < s.maxBatches; batch++ {
		if batch > 0 {
			days, err = s.policy.GetSupportChatRetentionDays(ctx)
			if err != nil {
				return result, fmt.Errorf("reload support chat retention policy: %w", err)
			}
			if days == 0 {
				break
			}
			if days < 0 || days > SupportChatRetentionDaysMax {
				return result, fmt.Errorf("support chat retention days out of range: %d", days)
			}
		}
		cutoff := now.UTC().Add(-time.Duration(days) * 24 * time.Hour)
		batchResult, err := s.cleaner.CleanupExpiredMessages(ctx, cutoff, s.batchSize)
		result.MessagesDeleted += batchResult.MessagesDeleted
		result.AssetsDeleted += batchResult.AssetsDeleted
		if err != nil {
			return result, err
		}
		if batchResult.MessagesDeleted < s.batchSize && batchResult.AssetsDeleted < s.batchSize {
			break
		}
	}
	return result, nil
}
