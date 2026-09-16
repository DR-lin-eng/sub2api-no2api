package service

import (
	"context"
	"log"
	"sync"
	"time"
)

// ProxyExpiryService 周期扫描到期代理并把绑定账号改投备用/直连。
type ProxyExpiryService struct {
	proxyRepo      ProxyRepository
	autoAssignRepo ProxyAutoAssignmentRepository
	settingService *SettingService
	interval       time.Duration
	stopCh         chan struct{}
	stopOnce       sync.Once
	wg             sync.WaitGroup
}

func (s *ProxyExpiryService) SetAutoAssignment(settingService *SettingService, repo ProxyAutoAssignmentRepository) {
	if s == nil {
		return
	}
	s.settingService = settingService
	s.autoAssignRepo = repo
}

func NewProxyExpiryService(proxyRepo ProxyRepository, interval time.Duration) *ProxyExpiryService {
	return &ProxyExpiryService{proxyRepo: proxyRepo, interval: interval, stopCh: make(chan struct{})}
}

func (s *ProxyExpiryService) Start() {
	if s == nil || s.proxyRepo == nil || s.interval <= 0 {
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

func (s *ProxyExpiryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.wg.Wait()
}

func (s *ProxyExpiryService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	shouldTrackExpiry := false
	var expiredBefore int64
	if s.settingService != nil && s.autoAssignRepo != nil {
		settings, err := s.settingService.GetProxyAutoAssignmentSettings(ctx)
		if err == nil && settings.Enabled {
			if expiredBefore, err = s.proxyRepo.CountExpired(ctx); err == nil {
				shouldTrackExpiry = true
			}
		}
	}
	changed, err := s.proxyRepo.SweepExpiredProxies(ctx, time.Now())
	if err != nil {
		log.Printf("[ProxyExpiry] sweep expired proxies failed: %v", err)
		return
	}
	if changed > 0 {
		log.Printf("[ProxyExpiry] re-routed %d accounts off expired proxies", changed)
	}
	if shouldTrackExpiry {
		expiredAfter, countErr := s.proxyRepo.CountExpired(ctx)
		if countErr != nil {
			log.Printf("[ProxyExpiry] count expired proxies after sweep failed: %v", countErr)
			return
		}
		if expiredAfter > expiredBefore {
			reassigned, rebalanceErr := s.autoAssignRepo.RebalanceActiveProxyAccounts(ctx, time.Now())
			if rebalanceErr != nil {
				log.Printf("[ProxyExpiry] auto-assignment rebalance failed: %v", rebalanceErr)
				return
			}
			if reassigned > 0 {
				log.Printf("[ProxyExpiry] auto-assignment moved %d accounts after proxy expiry", reassigned)
			}
		}
	}
}
