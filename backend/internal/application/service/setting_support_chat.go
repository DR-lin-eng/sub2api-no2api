package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

const (
	supportChatCacheTTL   = 60 * time.Second
	supportChatErrorTTL   = 5 * time.Second
	supportChatDBTimeout  = 5 * time.Second
	supportChatRefreshKey = "support_chat_enabled"

	// Ten years keeps the setting bounded for duration arithmetic while still
	// covering long compliance windows. Zero means retain indefinitely.
	SupportChatRetentionDaysMax = 3650
)

type cachedSupportChatEnabled struct {
	value     bool
	expiresAt int64
}

// IsSupportChatEnabled returns the cached opt-in switch. Missing settings and
// storage failures are deliberately disabled so an upgrade cannot expose a
// new chat surface while its control plane is unavailable.
func (s *SettingService) IsSupportChatEnabled(ctx context.Context) bool {
	if s == nil || s.settingRepo == nil {
		return false
	}
	if cached, ok := s.supportChatCache.Load().(*cachedSupportChatEnabled); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.value
		}
	}

	if ctx == nil {
		ctx = context.Background()
	}
	result, _, _ := s.supportChatSF.Do(supportChatRefreshKey, func() (any, error) {
		s.supportChatCacheMu.Lock()
		defer s.supportChatCacheMu.Unlock()

		if cached, ok := s.supportChatCache.Load().(*cachedSupportChatEnabled); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.value, nil
			}
		}

		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), supportChatDBTimeout)
		defer cancel()
		value, err := s.settingRepo.GetValue(dbCtx, SettingKeySupportChatEnabled)
		if err != nil {
			ttl := supportChatErrorTTL
			if errors.Is(err, ErrSettingNotFound) {
				ttl = supportChatCacheTTL
			} else {
				slog.Warn("failed to read support chat feature setting", "error", err)
			}
			s.supportChatCache.Store(&cachedSupportChatEnabled{
				value:     false,
				expiresAt: time.Now().Add(ttl).UnixNano(),
			})
			return false, nil
		}

		enabled := strings.EqualFold(strings.TrimSpace(value), "true")
		s.supportChatCache.Store(&cachedSupportChatEnabled{
			value:     enabled,
			expiresAt: time.Now().Add(supportChatCacheTTL).UnixNano(),
		})
		return enabled, nil
	})
	if enabled, ok := result.(bool); ok {
		return enabled
	}
	return false
}

func normalizeSupportChatRetentionDays(days int) int {
	if days < 0 {
		return 0
	}
	if days > SupportChatRetentionDaysMax {
		return SupportChatRetentionDaysMax
	}
	return days
}

func parseSupportChatRetentionDays(raw string) int {
	days, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0
	}
	return normalizeSupportChatRetentionDays(days)
}

// GetSupportChatRetentionDays returns the effective destructive policy. Cleanup
// requires an explicit enabled switch; missing settings preserve upgrade
// compatibility by retaining everything.
func (s *SettingService) GetSupportChatRetentionDays(ctx context.Context) (int, error) {
	if s == nil || s.settingRepo == nil {
		return 0, errors.New("support chat retention setting repository is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), supportChatDBTimeout)
	defer cancel()
	enabledRaw, err := s.settingRepo.GetValue(dbCtx, SettingKeySupportChatRetentionEnabled)
	if errors.Is(err, ErrSettingNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	switch strings.ToLower(strings.TrimSpace(enabledRaw)) {
	case "false":
		return 0, nil
	case "true":
		// Continue and validate the destructive retention period.
	default:
		return 0, fmt.Errorf("invalid support chat retention enabled value %q", enabledRaw)
	}

	raw, err := s.settingRepo.GetValue(dbCtx, SettingKeySupportChatRetentionDays)
	if errors.Is(err, ErrSettingNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	days, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || days < 0 || days > SupportChatRetentionDaysMax {
		return 0, fmt.Errorf("invalid support chat retention days %q", raw)
	}
	return days, nil
}
