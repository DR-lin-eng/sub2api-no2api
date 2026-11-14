package service

import (
	"context"
	"strings"
	"time"
)

const openAIAuxiliarySettingsTTL = 60 * time.Second

type cachedOpenAIAuxiliarySettings struct {
	attestation bool
	lenovo      bool
	expiresAt   time.Time
}

func (s *SettingService) defaultOpenAIOAuthAttestationEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIOAuthAttestationEnabled
}

// OpenAIOAuthAttestationEnabled uses the persisted override, with the existing
// deployment configuration as the fallback for installations without that row.
func (s *SettingService) OpenAIOAuthAttestationEnabled(ctx context.Context) bool {
	return s.openAIAuxiliarySettings(ctx).attestation
}

func (s *SettingService) OpenAILenovoAttributionEnabled(ctx context.Context) bool {
	return s.openAIAuxiliarySettings(ctx).lenovo
}

func (s *SettingService) openAIAuxiliarySettings(ctx context.Context) cachedOpenAIAuxiliarySettings {
	fallback := cachedOpenAIAuxiliarySettings{}
	if s == nil {
		return fallback
	}
	if s.cfg != nil {
		fallback.attestation = s.cfg.Gateway.OpenAIOAuthAttestationEnabled
	}
	if cached := s.openAIAuxiliaryCache.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
		return *cached
	}
	// Saving takes the same mutex: a slow database read cannot overwrite the
	// switch that a successful administrator update just published locally.
	s.openAIAuxiliaryMu.Lock()
	defer s.openAIAuxiliaryMu.Unlock()
	cached := s.openAIAuxiliaryCache.Load()
	if cached != nil && time.Now().Before(cached.expiresAt) {
		return *cached
	}
	if s.settingRepo == nil {
		if cached != nil {
			return *cached
		}
		return fallback
	}
	if ctx == nil {
		ctx = context.Background()
	}
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	values, err := s.settingRepo.GetMultiple(readCtx, []string{
		SettingKeyOpenAIOAuthAttestationEnabled, SettingKeyOpenAILenovoAttributionEnabled,
	})
	value, ttl := fallback, openAIAuxiliarySettingsTTL
	if err == nil {
		if raw := strings.TrimSpace(values[SettingKeyOpenAIOAuthAttestationEnabled]); raw != "" {
			value.attestation = strings.EqualFold(raw, "true")
		}
		value.lenovo = strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAILenovoAttributionEnabled]), "true")
	} else {
		ttl = 5 * time.Second
		if cached != nil {
			value = *cached
		}
	}
	value.expiresAt = time.Now().Add(ttl)
	s.openAIAuxiliaryCache.Store(&value)
	return value
}

func (s *SettingService) refreshOpenAIAuxiliarySettings(settings *SystemSettings) {
	s.openAIAuxiliaryMu.Lock()
	defer s.openAIAuxiliaryMu.Unlock()
	s.openAIAuxiliaryCache.Store(&cachedOpenAIAuxiliarySettings{
		attestation: settings.OpenAIOAuthAttestationEnabled,
		lenovo:      settings.OpenAILenovoAttributionEnabled,
		expiresAt:   time.Now().Add(openAIAuxiliarySettingsTTL),
	})
}
