package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type cachedOpenAIOAuthForceRelaySettings struct {
	enabled   bool
	baseURL   string
	expiresAt int64
}

const (
	openAIOAuthForceRelayCacheTTL   = 60 * time.Second
	openAIOAuthForceRelayDBTimeout  = 5 * time.Second
	openAIOAuthForceRelayRefreshKey = "openai_oauth_force_relay"
)

// GetOpenAIOAuthForceRelaySettings returns the cached global relay policy.
// Invalid enabled configurations return an error so model traffic never
// silently falls back to the official endpoint after an administrator asked
// for a forced relay.
func (s *SettingService) GetOpenAIOAuthForceRelaySettings(ctx context.Context) (bool, string, error) {
	if s == nil || s.settingRepo == nil {
		return false, "", nil
	}
	if cached, ok := s.openAIOAuthForceRelayCache.Load().(*cachedOpenAIOAuthForceRelaySettings); ok && cached != nil && time.Now().UnixNano() < cached.expiresAt {
		return s.validateOpenAIOAuthForceRelayCache(cached)
	}
	value, loadErr, _ := s.openAIOAuthForceRelaySF.Do(openAIOAuthForceRelayRefreshKey, func() (any, error) {
		if cached, ok := s.openAIOAuthForceRelayCache.Load().(*cachedOpenAIOAuthForceRelaySettings); ok && cached != nil && time.Now().UnixNano() < cached.expiresAt {
			return cached, nil
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIOAuthForceRelayDBTimeout)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, []string{
			SettingKeyOpenAIOAuthForceRelayEnabled,
			SettingKeyOpenAIOAuthForceRelayBaseURL,
		})
		if err != nil {
			slog.Warn("failed to get OpenAI OAuth force relay settings", "error", err)
			return nil, err
		}
		entry := &cachedOpenAIOAuthForceRelaySettings{
			enabled:   strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAIOAuthForceRelayEnabled]), "true"),
			baseURL:   strings.TrimSpace(values[SettingKeyOpenAIOAuthForceRelayBaseURL]),
			expiresAt: time.Now().Add(openAIOAuthForceRelayCacheTTL).UnixNano(),
		}
		s.openAIOAuthForceRelayCache.Store(entry)
		return entry, nil
	})
	if loadErr != nil {
		return false, "", fmt.Errorf("load OpenAI OAuth force relay settings: %w", loadErr)
	}
	entry, ok := value.(*cachedOpenAIOAuthForceRelaySettings)
	if !ok || entry == nil {
		return false, "", nil
	}
	return s.validateOpenAIOAuthForceRelayCache(entry)
}

func (s *SettingService) validateOpenAIOAuthForceRelayCache(cached *cachedOpenAIOAuthForceRelaySettings) (bool, string, error) {
	if cached == nil || !cached.enabled {
		return false, "", nil
	}
	baseURL := strings.TrimSpace(cached.baseURL)
	if baseURL == "" {
		return false, "", fmt.Errorf("base URL is required when the global relay is enabled")
	}
	normalized, err := validateOpenAIOAuthCustomRelayBaseURL(baseURL, s.cfg)
	if err != nil {
		return false, "", fmt.Errorf("invalid base URL %q: %w", baseURL, err)
	}
	return true, normalized, nil
}

func (s *SettingService) refreshOpenAIOAuthForceRelaySettings(settings *SystemSettings) {
	if s == nil || settings == nil {
		return
	}
	s.openAIOAuthForceRelaySF.Forget(openAIOAuthForceRelayRefreshKey)
	s.openAIOAuthForceRelayCache.Store(&cachedOpenAIOAuthForceRelaySettings{
		enabled:   settings.OpenAIOAuthForceRelayEnabled,
		baseURL:   strings.TrimSpace(settings.OpenAIOAuthForceRelayBaseURL),
		expiresAt: time.Now().Add(openAIOAuthForceRelayCacheTTL).UnixNano(),
	})
}
