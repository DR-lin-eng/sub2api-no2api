package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

const (
	defaultProxyHealthCheckIntervalMinutes = 15
	minProxyHealthCheckIntervalMinutes     = 1
	maxProxyHealthCheckIntervalMinutes     = 24 * 60
	defaultProxyHealthFailureThreshold     = 3
	minProxyHealthFailureThreshold         = 1
	maxProxyHealthFailureThreshold         = 10
)

var (
	ErrProxyAutoAssignmentUnavailable = infraerrors.ServiceUnavailable(
		"PROXY_AUTO_ASSIGNMENT_UNAVAILABLE", "proxy auto-assignment is unavailable",
	)
	ErrProxyAutoAssignmentNoActiveProxy = infraerrors.Conflict(
		"PROXY_AUTO_ASSIGNMENT_NO_ACTIVE_PROXY", "proxy auto-assignment requires at least one active proxy",
	)
)

// ProxyAutoAssignmentSettings controls forced account assignment and periodic
// proxy health checks. The feature is opt-in to preserve existing deployments.
type ProxyAutoAssignmentSettings struct {
	Enabled                    bool `json:"enabled"`
	HealthCheckEnabled         bool `json:"health_check_enabled"`
	HealthCheckIntervalMinutes int  `json:"health_check_interval_minutes"`
	FailureThreshold           int  `json:"failure_threshold"`
}

// ProxyAutoAssignmentRepository is the narrow transactional capability used by
// the proxy pool. Keeping it separate avoids widening read-only proxy adapters.
type ProxyAutoAssignmentRepository interface {
	SelectLeastLoadedActiveProxy(ctx context.Context, now time.Time) (*int64, error)
	RebalanceActiveProxyAccounts(ctx context.Context, now time.Time) (int64, error)
}

func DefaultProxyAutoAssignmentSettings() *ProxyAutoAssignmentSettings {
	return &ProxyAutoAssignmentSettings{
		Enabled:                    false,
		HealthCheckEnabled:         false,
		HealthCheckIntervalMinutes: defaultProxyHealthCheckIntervalMinutes,
		FailureThreshold:           defaultProxyHealthFailureThreshold,
	}
}

func normalizeProxyAutoAssignmentSettings(settings *ProxyAutoAssignmentSettings) {
	if settings.HealthCheckIntervalMinutes == 0 {
		settings.HealthCheckIntervalMinutes = defaultProxyHealthCheckIntervalMinutes
	}
	if settings.FailureThreshold == 0 {
		settings.FailureThreshold = defaultProxyHealthFailureThreshold
	}
}

func validateProxyAutoAssignmentSettings(settings *ProxyAutoAssignmentSettings) error {
	if settings == nil {
		return infraerrors.BadRequest("INVALID_PROXY_AUTO_ASSIGNMENT_SETTINGS", "settings cannot be nil")
	}
	normalizeProxyAutoAssignmentSettings(settings)
	if settings.HealthCheckIntervalMinutes < minProxyHealthCheckIntervalMinutes ||
		settings.HealthCheckIntervalMinutes > maxProxyHealthCheckIntervalMinutes {
		return infraerrors.BadRequest(
			"INVALID_PROXY_HEALTH_CHECK_INTERVAL",
			fmt.Sprintf("health_check_interval_minutes must be between %d and %d", minProxyHealthCheckIntervalMinutes, maxProxyHealthCheckIntervalMinutes),
		)
	}
	if settings.FailureThreshold < minProxyHealthFailureThreshold ||
		settings.FailureThreshold > maxProxyHealthFailureThreshold {
		return infraerrors.BadRequest(
			"INVALID_PROXY_HEALTH_FAILURE_THRESHOLD",
			fmt.Sprintf("failure_threshold must be between %d and %d", minProxyHealthFailureThreshold, maxProxyHealthFailureThreshold),
		)
	}
	return nil
}

func (s *SettingService) GetProxyAutoAssignmentSettings(ctx context.Context) (*ProxyAutoAssignmentSettings, error) {
	defaults := DefaultProxyAutoAssignmentSettings()
	if s == nil || s.settingRepo == nil {
		return defaults, nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyProxyAutoAssignmentSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return defaults, nil
		}
		return nil, fmt.Errorf("get proxy auto-assignment settings: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return defaults, nil
	}
	settings := *defaults
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil, fmt.Errorf("parse proxy auto-assignment settings: %w", err)
	}
	normalizeProxyAutoAssignmentSettings(&settings)
	return &settings, nil
}

func (s *SettingService) SetProxyAutoAssignmentSettings(ctx context.Context, settings *ProxyAutoAssignmentSettings) error {
	if s == nil || s.settingRepo == nil {
		return ErrProxyAutoAssignmentUnavailable
	}
	if err := validateProxyAutoAssignmentSettings(settings); err != nil {
		return err
	}
	payload, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal proxy auto-assignment settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyProxyAutoAssignmentSettings, string(payload)); err != nil {
		return fmt.Errorf("set proxy auto-assignment settings: %w", err)
	}
	return nil
}

func (s *adminServiceImpl) proxyAssignmentRepository() (ProxyAutoAssignmentRepository, error) {
	repo, ok := s.proxyRepo.(ProxyAutoAssignmentRepository)
	if !ok || repo == nil {
		return nil, ErrProxyAutoAssignmentUnavailable
	}
	return repo, nil
}

func (s *adminServiceImpl) GetProxyAutoAssignmentSettings(ctx context.Context) (*ProxyAutoAssignmentSettings, error) {
	if s.settingService == nil {
		return DefaultProxyAutoAssignmentSettings(), nil
	}
	return s.settingService.GetProxyAutoAssignmentSettings(ctx)
}

func (s *adminServiceImpl) UpdateProxyAutoAssignmentSettings(ctx context.Context, requested *ProxyAutoAssignmentSettings) (*ProxyAutoAssignmentSettings, error) {
	if s.settingService == nil {
		return nil, ErrProxyAutoAssignmentUnavailable
	}
	if err := validateProxyAutoAssignmentSettings(requested); err != nil {
		return nil, err
	}
	previous, err := s.settingService.GetProxyAutoAssignmentSettings(ctx)
	if err != nil {
		return nil, err
	}
	if requested.Enabled {
		repo, err := s.proxyAssignmentRepository()
		if err != nil {
			return nil, err
		}
		if _, err := repo.SelectLeastLoadedActiveProxy(ctx, time.Now()); err != nil {
			return nil, err
		}
	}
	if err := s.settingService.SetProxyAutoAssignmentSettings(ctx, requested); err != nil {
		return nil, err
	}
	if requested.Enabled {
		if _, err := s.RebalanceProxyAssignments(ctx); err != nil {
			_ = s.settingService.SetProxyAutoAssignmentSettings(context.Background(), previous)
			return nil, err
		}
	}
	return requested, nil
}

func (s *adminServiceImpl) RebalanceProxyAssignments(ctx context.Context) (int64, error) {
	settings, err := s.GetProxyAutoAssignmentSettings(ctx)
	if err != nil {
		return 0, err
	}
	if !settings.Enabled {
		return 0, infraerrors.Conflict("PROXY_AUTO_ASSIGNMENT_DISABLED", "proxy auto-assignment is disabled")
	}
	repo, err := s.proxyAssignmentRepository()
	if err != nil {
		return 0, err
	}
	return repo.RebalanceActiveProxyAccounts(ctx, time.Now())
}

func (s *adminServiceImpl) rebalanceProxyAssignmentsIfEnabled(ctx context.Context) (int64, error) {
	settings, err := s.GetProxyAutoAssignmentSettings(ctx)
	if err != nil {
		return 0, err
	}
	if !settings.Enabled {
		return 0, nil
	}
	repo, err := s.proxyAssignmentRepository()
	if err != nil {
		return 0, err
	}
	return repo.RebalanceActiveProxyAccounts(ctx, time.Now())
}

// forcedProxyAssignment returns the authoritative proxy id while the global
// switch is enabled. Callers must fail closed when no active proxy is available.
func (s *adminServiceImpl) forcedProxyAssignment(ctx context.Context) (*int64, bool, error) {
	settings, err := s.GetProxyAutoAssignmentSettings(ctx)
	if err != nil {
		return nil, false, err
	}
	if !settings.Enabled {
		return nil, false, nil
	}
	repo, err := s.proxyAssignmentRepository()
	if err != nil {
		return nil, true, err
	}
	id, err := repo.SelectLeastLoadedActiveProxy(ctx, time.Now())
	if err != nil {
		return nil, true, err
	}
	return id, true, nil
}
