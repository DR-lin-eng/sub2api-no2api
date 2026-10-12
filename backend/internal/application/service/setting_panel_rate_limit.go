package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// PanelRateLimitSettings 面板 API 限流配置。
// 认证后的面板接口按「用户 ID」维度限流（与客户端 IP 无关，反向代理/共享出口
// 不会被误伤）；无需认证的公开接口按安全客户端 IP 维度限流（包含内网/回环直连地址）。
type PanelRateLimitSettings struct {
	// Enabled 总开关
	Enabled bool `json:"enabled"`
	// UserRPM 每用户每分钟请求数上限（认证面板接口全量计数；0 = 不限制）
	UserRPM int `json:"user_rpm"`
	// HeavyRPM 每用户每分钟重查询上限（usage/dashboard 等聚合统计接口；0 = 不限制）
	HeavyRPM int `json:"heavy_rpm"`
	// ExemptAdmin 管理员账号是否豁免按用户限流
	ExemptAdmin bool `json:"exempt_admin"`
	// PublicIPRPM 无需认证的公开接口每 IP 每分钟上限（0 = 不限制）
	PublicIPRPM int `json:"public_ip_rpm"`
}

// 面板限流 RPM 的取值上限，防止配置异常大的值失去意义。
const panelRateLimitRPMMax = 100000

const (
	panelRateLimitCacheTTL  = 60 * time.Second
	panelRateLimitErrorTTL  = 2 * time.Second
	panelRateLimitDBTimeout = 5 * time.Second
	panelRateLimitLogEvery  = 30 * time.Second
)

// cachedPanelRateLimitSettings 进程内缓存条目（60s TTL）。
type cachedPanelRateLimitSettings struct {
	settings  PanelRateLimitSettings
	expiresAt int64 // unix nano
}

// DefaultPanelRateLimitSettings protects public and authenticated panel
// endpoints on installations that have no persisted policy yet. Operators may
// explicitly disable it through the admin setting when compatibility requires.
func DefaultPanelRateLimitSettings() *PanelRateLimitSettings {
	return &PanelRateLimitSettings{
		Enabled:     true,
		UserRPM:     240,
		HeavyRPM:    60,
		ExemptAdmin: true,
		PublicIPRPM: 300,
	}
}

// normalizePanelRateLimitSettings 修正非法取值（负数归零、超上限截断）。
func normalizePanelRateLimitSettings(s *PanelRateLimitSettings) {
	if s == nil {
		return
	}
	if s.UserRPM < 0 {
		s.UserRPM = 0
	}
	if s.HeavyRPM < 0 {
		s.HeavyRPM = 0
	}
	if s.PublicIPRPM < 0 {
		s.PublicIPRPM = 0
	}
	if s.UserRPM > panelRateLimitRPMMax {
		s.UserRPM = panelRateLimitRPMMax
	}
	if s.HeavyRPM > panelRateLimitRPMMax {
		s.HeavyRPM = panelRateLimitRPMMax
	}
	if s.PublicIPRPM > panelRateLimitRPMMax {
		s.PublicIPRPM = panelRateLimitRPMMax
	}
}

// GetPanelRateLimitSettings 获取面板 API 限流配置（直读 DB，供管理端读写路径使用）。
// 缺失/空/解析失败 -> 返回安全默认配置（Enabled=true）。
func (s *SettingService) GetPanelRateLimitSettings(ctx context.Context) (*PanelRateLimitSettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyPanelRateLimitSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultPanelRateLimitSettings(), nil
		}
		return nil, fmt.Errorf("get panel rate limit settings: %w", err)
	}
	if strings.TrimSpace(value) == "" {
		return DefaultPanelRateLimitSettings(), nil
	}

	settings := DefaultPanelRateLimitSettings()
	if err := json.Unmarshal([]byte(value), settings); err != nil {
		s.logPanelRateLimitRefreshFailure("failed to unmarshal panel rate limit settings, falling back to defaults", err)
		return DefaultPanelRateLimitSettings(), nil
	}
	normalizePanelRateLimitSettings(settings)
	return settings, nil
}

// SetPanelRateLimitSettings 保存面板 API 限流配置，并立即刷新进程内缓存，
// 使当前节点的下一个请求即生效（多节点部署最迟 60s 内生效）。
func (s *SettingService) SetPanelRateLimitSettings(ctx context.Context, settings *PanelRateLimitSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	if settings.UserRPM < 0 || settings.HeavyRPM < 0 || settings.PublicIPRPM < 0 {
		return fmt.Errorf("rate limit values cannot be negative")
	}
	if settings.UserRPM > panelRateLimitRPMMax || settings.HeavyRPM > panelRateLimitRPMMax || settings.PublicIPRPM > panelRateLimitRPMMax {
		return fmt.Errorf("rate limit values must be at most %d", panelRateLimitRPMMax)
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal panel rate limit settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyPanelRateLimitSettings, string(data)); err != nil {
		return err
	}

	s.panelRateLimitRevision.Add(1)
	s.panelRateLimitRefreshRetryAt.Store(0)
	s.storePanelRateLimitCache(*settings, panelRateLimitCacheTTL)
	return nil
}

// GetPanelRateLimitSettingsCached serves stale settings immediately and
// refreshes expired entries in the background. The first request observes the
// secure default while the persisted value is loaded.
func (s *SettingService) GetPanelRateLimitSettingsCached(ctx context.Context) PanelRateLimitSettings {
	if s == nil || s.settingRepo == nil {
		return *DefaultPanelRateLimitSettings()
	}
	now := time.Now().UnixNano()
	cached, _ := s.panelRateLimitCache.Load().(*cachedPanelRateLimitSettings)
	if cached != nil && now < cached.expiresAt {
		return cached.settings
	}

	fallback := *DefaultPanelRateLimitSettings()
	if cached != nil {
		fallback = cached.settings
	}
	s.refreshPanelRateLimitSettingsAsync(ctx, now)
	return fallback
}

func (s *SettingService) refreshPanelRateLimitSettingsAsync(_ context.Context, now int64) {
	if now < s.panelRateLimitRefreshRetryAt.Load() || !s.panelRateLimitRefreshInFlight.CompareAndSwap(false, true) {
		return
	}
	revision := s.panelRateLimitRevision.Load()
	go func() {
		defer s.panelRateLimitRefreshInFlight.Store(false)
		dbCtx, cancel := context.WithTimeout(context.Background(), panelRateLimitDBTimeout)
		defer cancel()

		settings, err := s.GetPanelRateLimitSettings(dbCtx)
		if err != nil {
			now := time.Now()
			s.panelRateLimitRefreshRetryAt.Store(now.Add(panelRateLimitErrorTTL).UnixNano())
			s.logPanelRateLimitRefreshFailure("failed to refresh panel rate limit settings", err)
			return
		}
		s.panelRateLimitRefreshRetryAt.Store(0)
		if revision != s.panelRateLimitRevision.Load() {
			return
		}
		s.storePanelRateLimitCache(*settings, panelRateLimitCacheTTL)
	}()
}

func (s *SettingService) logPanelRateLimitRefreshFailure(message string, err error) {
	if s == nil {
		return
	}
	now := time.Now().UnixNano()
	for {
		last := s.panelRateLimitLastLogAt.Load()
		if last != 0 && now-last < panelRateLimitLogEvery.Nanoseconds() {
			return
		}
		if s.panelRateLimitLastLogAt.CompareAndSwap(last, now) {
			slog.Warn(message, "error", err, "key", SettingKeyPanelRateLimitSettings)
			return
		}
	}
}

func (s *SettingService) storePanelRateLimitCache(settings PanelRateLimitSettings, ttl time.Duration) {
	s.panelRateLimitCache.Store(&cachedPanelRateLimitSettings{
		settings:  settings,
		expiresAt: time.Now().Add(ttl).UnixNano(),
	})
}
