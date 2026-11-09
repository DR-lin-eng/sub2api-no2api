package service

import (
	"context"
	"strings"
	"time"
)

const imageSchedulingSettingsTTL = 5 * time.Second

type cachedImageSchedulingSettings struct {
	enabled   bool
	expiresAt time.Time
}

func (s *SettingService) ImageIndependentSchedulingEnabled(ctx context.Context) bool {
	if s == nil || s.settingRepo == nil {
		return false
	}
	if cached := s.imageSchedulingCache.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
		return cached.enabled
	}
	s.imageSchedulingMu.Lock()
	defer s.imageSchedulingMu.Unlock()
	if cached := s.imageSchedulingCache.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
		return cached.enabled
	}
	if ctx == nil {
		ctx = context.Background()
	}
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	values, err := s.settingRepo.GetMultiple(readCtx, []string{SettingKeyImageIndependentSchedulingEnabled})
	enabled := err == nil && strings.EqualFold(strings.TrimSpace(values[SettingKeyImageIndependentSchedulingEnabled]), "true")
	s.imageSchedulingCache.Store(&cachedImageSchedulingSettings{enabled: enabled, expiresAt: time.Now().Add(imageSchedulingSettingsTTL)})
	return enabled
}

func (s *SettingService) refreshImageSchedulingSettings(enabled bool) {
	s.imageSchedulingMu.Lock()
	defer s.imageSchedulingMu.Unlock()
	s.imageSchedulingCache.Store(&cachedImageSchedulingSettings{enabled: enabled, expiresAt: time.Now().Add(imageSchedulingSettingsTTL)})
}
