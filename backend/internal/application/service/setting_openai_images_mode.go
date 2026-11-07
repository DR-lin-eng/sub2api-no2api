package service

import (
	"context"
	"strings"
	"time"
)

type cachedOpenAIImagesMode struct {
	legacy    bool
	expiresAt time.Time
}

// New installations and existing rows without this key use the App protocol.
// Keep the last known mode on DB errors; never switch protocols during a turn.
func (s *SettingService) OpenAIImagesLegacyEnabled(ctx context.Context) bool {
	if s == nil {
		return false
	}
	if cached := s.openAIImagesModeCache.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
		return cached.legacy
	}
	if s.settingRepo == nil {
		return false
	}
	s.openAIImagesModeMu.Lock()
	defer s.openAIImagesModeMu.Unlock()
	cached := s.openAIImagesModeCache.Load()
	if cached != nil && time.Now().Before(cached.expiresAt) {
		return cached.legacy
	}
	if ctx == nil {
		ctx = context.Background()
	}
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	values, err := s.settingRepo.GetMultiple(readCtx, []string{SettingKeyOpenAIImagesLegacyEnabled})
	legacy, ttl := false, 60*time.Second
	if err == nil {
		legacy = strings.EqualFold(values[SettingKeyOpenAIImagesLegacyEnabled], "true")
	} else {
		ttl = 5 * time.Second
		if cached != nil {
			legacy = cached.legacy
		}
	}
	s.openAIImagesModeCache.Store(&cachedOpenAIImagesMode{legacy: legacy, expiresAt: time.Now().Add(ttl)})
	return legacy
}

func (s *SettingService) refreshOpenAIImagesMode(legacy bool) {
	s.openAIImagesModeMu.Lock()
	defer s.openAIImagesModeMu.Unlock()
	s.openAIImagesModeCache.Store(&cachedOpenAIImagesMode{legacy: legacy, expiresAt: time.Now().Add(60 * time.Second)})
}
