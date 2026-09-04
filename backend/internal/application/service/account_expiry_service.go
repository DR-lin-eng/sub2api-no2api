package service

import (
	"context"
	"log"
	"sync"
	"time"
)

// AccountExpiryService periodically pauses expired accounts when auto-pause is enabled.
type AccountExpiryService struct {
	accountRepo           AccountRepository
	quotaResetAutoEnabler *RateLimitService
	interval              time.Duration
	stopCh                chan struct{}
	stopOnce              sync.Once
	wg                    sync.WaitGroup
}

func (s *AccountExpiryService) SetOpenAIOAuthQuotaResetAutoEnabler(enabler *RateLimitService) {
	if s != nil {
		s.quotaResetAutoEnabler = enabler
	}
}

func NewAccountExpiryService(accountRepo AccountRepository, interval time.Duration) *AccountExpiryService {
	return &AccountExpiryService{
		accountRepo: accountRepo,
		interval:    interval,
		stopCh:      make(chan struct{}),
	}
}

func (s *AccountExpiryService) Start() {
	if s == nil || s.accountRepo == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *AccountExpiryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

func (s *AccountExpiryService) runOnce() {
	expiryCtx, cancelExpiry := context.WithTimeout(context.Background(), 5*time.Second)
	updated, err := s.accountRepo.AutoPauseExpiredAccounts(expiryCtx, time.Now())
	cancelExpiry()
	if err != nil {
		log.Printf("[AccountExpiry] Auto pause expired accounts failed: %v", err)
	} else if updated > 0 {
		log.Printf("[AccountExpiry] Auto paused %d expired accounts", updated)
	}
	if s.quotaResetAutoEnabler == nil {
		return
	}
	quotaCtx, cancelQuota := context.WithTimeout(context.Background(), 5*time.Second)
	enabled, err := s.quotaResetAutoEnabler.AutoEnableOpenAIAccountsAfterQuotaReset(quotaCtx, time.Now())
	cancelQuota()
	if err != nil {
		log.Printf("[AccountExpiry] Auto enable OpenAI OAuth accounts after quota reset failed: %v", err)
		return
	}
	if enabled > 0 {
		log.Printf("[AccountExpiry] Auto enabled %d OpenAI OAuth accounts after quota reset", enabled)
	}
}
