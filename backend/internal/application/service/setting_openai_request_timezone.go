package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
)

type cachedOpenAIRequestTimezoneSettings struct {
	enabled   bool
	timezone  string
	expiresAt time.Time
}

// ValidateOpenAIRequestTimezone permits an empty override to use the deployment
// default. Local is process-dependent and is not a portable IANA timezone.
func ValidateOpenAIRequestTimezone(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if len(name) > 128 || name == "Local" {
		return fmt.Errorf("openai_request_timezone must be an IANA timezone such as Asia/Singapore")
	}
	if _, err := time.LoadLocation(name); err != nil {
		return fmt.Errorf("openai_request_timezone must be a valid IANA timezone: %w", err)
	}
	return nil
}

func (s *SettingService) defaultOpenAIRequestTimezone() string {
	if s != nil && s.cfg != nil {
		if name := strings.TrimSpace(s.cfg.Gateway.OpenAIRequestTimezone); name != "" {
			return name
		}
	}
	return config.DefaultOpenAIRequestTimezone
}

func (s *SettingService) resolveOpenAIRequestTimezoneSettings(values map[string]string) (bool, string) {
	enabled := true
	if value := strings.TrimSpace(values[SettingKeyOpenAIRequestTimezoneEnabled]); value != "" {
		enabled = strings.EqualFold(value, "true")
	}
	name := strings.TrimSpace(values[SettingKeyOpenAIRequestTimezone])
	if name == "" || ValidateOpenAIRequestTimezone(name) != nil {
		name = s.defaultOpenAIRequestTimezone()
	}
	return enabled, name
}

// GetOpenAIRequestTimezoneSettings shares one bounded cache across HTTP and WS
// turns. Saving updates this instance immediately; other instances reload within
// 60 seconds. A temporary database failure retains the last known setting.
func (s *SettingService) GetOpenAIRequestTimezoneSettings(ctx context.Context) (bool, string) {
	if s == nil || s.settingRepo == nil {
		return true, s.defaultOpenAIRequestTimezone()
	}
	if cached := s.openAIRequestTimezoneCache.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
		return cached.enabled, cached.timezone
	}
	s.openAIRequestTimezoneMu.Lock()
	defer s.openAIRequestTimezoneMu.Unlock()
	cached := s.openAIRequestTimezoneCache.Load()
	if cached != nil && time.Now().Before(cached.expiresAt) {
		return cached.enabled, cached.timezone
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	values, err := s.settingRepo.GetMultiple(dbCtx, []string{SettingKeyOpenAIRequestTimezoneEnabled, SettingKeyOpenAIRequestTimezone})
	enabled, name := true, s.defaultOpenAIRequestTimezone()
	ttl := 60 * time.Second
	if err == nil {
		enabled, name = s.resolveOpenAIRequestTimezoneSettings(values)
	} else {
		ttl = 5 * time.Second
		if cached != nil {
			enabled, name = cached.enabled, cached.timezone
		}
	}
	s.openAIRequestTimezoneCache.Store(&cachedOpenAIRequestTimezoneSettings{enabled: enabled, timezone: name, expiresAt: time.Now().Add(ttl)})
	return enabled, name
}

func (s *SettingService) refreshOpenAIRequestTimezoneSettings(settings *SystemSettings) {
	name := strings.TrimSpace(settings.OpenAIRequestTimezone)
	if name == "" {
		name = s.defaultOpenAIRequestTimezone()
	}
	s.openAIRequestTimezoneMu.Lock()
	defer s.openAIRequestTimezoneMu.Unlock()
	s.openAIRequestTimezoneCache.Store(&cachedOpenAIRequestTimezoneSettings{
		enabled: settings.OpenAIRequestTimezoneEnabled, timezone: name, expiresAt: time.Now().Add(60 * time.Second),
	})
}
