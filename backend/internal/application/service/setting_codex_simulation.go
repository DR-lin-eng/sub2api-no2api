package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/codexsimulation"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
)

const (
	codexSimulationSettingsRefreshInterval = 5 * time.Second
	codexSimulationSettingsReloadTimeout   = 3 * time.Second
	codexSimulationDefaultStateTTLSeconds  = 7 * 24 * 60 * 60
)

// CodexSimulationSettings is the complete runtime snapshot used by one Codex
// request. IdentitySecret is persisted but must never be returned by an HTTP
// handler; transport DTOs expose only IdentitySecretConfigured.
type CodexSimulationSettings struct {
	FullSimulationEnabled   bool   `json:"full_simulation_enabled"`
	CLevelSimulationEnabled bool   `json:"c_level_simulation_enabled"`
	ContinuationMode        string `json:"continuation_mode"`
	StateTTLSeconds         int    `json:"state_ttl_seconds"`
	IdentitySecret          string `json:"identity_secret"`
}

func (s CodexSimulationSettings) IdentitySecretConfigured() bool {
	return len([]byte(strings.TrimSpace(s.IdentitySecret))) >= 32
}

func (s CodexSimulationSettings) enabled() bool {
	return s.FullSimulationEnabled || s.continuationMode() != codexContinuationOff
}

func (s CodexSimulationSettings) configured() bool {
	return s.enabled() && s.IdentitySecretConfigured()
}

func (s CodexSimulationSettings) continuationMode() codexContinuationMode {
	switch strings.ToLower(strings.TrimSpace(s.ContinuationMode)) {
	case string(codexContinuationShadow):
		return codexContinuationShadow
	case string(codexContinuationEnforce):
		return codexContinuationEnforce
	default:
		return codexContinuationOff
	}
}

func (s CodexSimulationSettings) stateTTL() time.Duration {
	if s.StateTTLSeconds <= 0 {
		return codexSimulationDefaultStateTTL
	}
	return time.Duration(s.StateTTLSeconds) * time.Second
}

// cLevelTransportSimulationEnabled is intentionally permissive for isolated
// unit fixtures that do not construct a SettingService. Production wiring
// always supplies the setting service, so the administrator gate is
// authoritative there.
func cLevelTransportSimulationEnabled(settingService *SettingService) bool {
	return settingService == nil || codexsimulation.CLevelEnabled()
}

func (s *SettingService) defaultCodexSimulationSettings() CodexSimulationSettings {
	settings := CodexSimulationSettings{
		ContinuationMode: codexContinuationOff.String(),
		StateTTLSeconds:  codexSimulationDefaultStateTTLSeconds,
	}
	if s == nil || s.cfg == nil {
		return settings
	}

	cfg := s.cfg.Gateway.CodexSimulation
	settings.FullSimulationEnabled = cfg.FullSimulationEnabled
	settings.CLevelSimulationEnabled = cfg.CLevelSimulationEnabled
	settings.IdentitySecret = strings.TrimSpace(cfg.IdentitySecret)
	settings.ContinuationMode = normalizeCodexContinuationMode(cfg.ContinuationMode)
	if cfg.StateTTLSeconds > 0 {
		settings.StateTTLSeconds = cfg.StateTTLSeconds
	}
	return settings
}

func (m codexContinuationMode) String() string {
	return string(m)
}

func normalizeCodexContinuationMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(codexContinuationShadow):
		return string(codexContinuationShadow)
	case string(codexContinuationEnforce):
		return string(codexContinuationEnforce)
	default:
		return string(codexContinuationOff)
	}
}

func validateCodexSimulationSettings(settings CodexSimulationSettings) (CodexSimulationSettings, error) {
	mode := strings.ToLower(strings.TrimSpace(settings.ContinuationMode))
	if mode == "" {
		mode = string(codexContinuationOff)
	}
	switch mode {
	case string(codexContinuationOff), string(codexContinuationShadow), string(codexContinuationEnforce):
	default:
		return CodexSimulationSettings{}, fmt.Errorf("continuation_mode must be one of off|shadow|enforce")
	}
	if settings.StateTTLSeconds <= 0 {
		return CodexSimulationSettings{}, fmt.Errorf("state_ttl_seconds must be positive")
	}
	// time.Duration is an int64 nanosecond count. Reject values that would wrap.
	if !validCodexSimulationStateTTLSeconds(settings.StateTTLSeconds) {
		return CodexSimulationSettings{}, fmt.Errorf("state_ttl_seconds is too large")
	}

	settings.ContinuationMode = mode
	settings.IdentitySecret = strings.TrimSpace(settings.IdentitySecret)
	if (settings.FullSimulationEnabled || mode != string(codexContinuationOff)) && len([]byte(settings.IdentitySecret)) < 32 {
		return CodexSimulationSettings{}, fmt.Errorf("identity secret must be at least 32 bytes when Codex simulation is enabled")
	}
	return settings, nil
}

func validCodexSimulationStateTTLSeconds(value int) bool {
	return value > 0 && int64(value) <= int64(^uint64(0)>>1)/int64(time.Second)
}

func (s *SettingService) readCodexSimulationSettings(ctx context.Context) (CodexSimulationSettings, error) {
	fallback := s.defaultCodexSimulationSettings()
	if s == nil || s.settingRepo == nil {
		return fallback, nil
	}

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyCodexSimulationSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return fallback, nil
		}
		return CodexSimulationSettings{}, fmt.Errorf("get Codex simulation settings: %w", err)
	}

	var settings CodexSimulationSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return CodexSimulationSettings{}, fmt.Errorf("decode Codex simulation settings: %w", err)
	}
	settings, err = validateCodexSimulationSettings(settings)
	if err != nil {
		return CodexSimulationSettings{}, fmt.Errorf("validate Codex simulation settings: %w", err)
	}
	return settings, nil
}

// LoadCodexSimulationSettings refreshes the process-local snapshot. Missing
// rows intentionally retain the legacy YAML/environment values.
func (s *SettingService) LoadCodexSimulationSettings(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.codexSimulationSettingsLoadMu.Lock()
	defer s.codexSimulationSettingsLoadMu.Unlock()
	revision := s.codexSimulationSettingsRevision.Load()
	settings, err := s.readCodexSimulationSettings(ctx)
	if err != nil {
		return err
	}
	if revision != s.codexSimulationSettingsRevision.Load() {
		return nil
	}
	s.codexSimulationSettings.Store(&settings)
	codexsimulation.SetCLevelEnabled(settings.CLevelSimulationEnabled)
	return nil
}

// CodexSimulationSettingsSnapshot returns the current immutable runtime value.
// Request processing never performs a database read; cross-instance refreshes
// are handled by StartCodexSimulationSettingsSync.
func (s *SettingService) CodexSimulationSettingsSnapshot(_ context.Context) CodexSimulationSettings {
	if s == nil {
		return CodexSimulationSettings{
			ContinuationMode: string(codexContinuationOff),
			StateTTLSeconds:  codexSimulationDefaultStateTTLSeconds,
		}
	}

	if cached := s.codexSimulationSettings.Load(); cached != nil {
		return *cached
	}
	return s.defaultCodexSimulationSettings()
}

// StartCodexSimulationSettingsSync keeps non-writer instances current without
// putting database work on the OAuth request path.
func (s *SettingService) StartCodexSimulationSettingsSync(ctx context.Context) {
	s.startCodexSimulationSettingsSync(ctx, codexSimulationSettingsRefreshInterval)
}

func (s *SettingService) startCodexSimulationSettingsSync(ctx context.Context, interval time.Duration) {
	if s == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if interval <= 0 {
		interval = codexSimulationSettingsRefreshInterval
	}
	s.codexSimulationSettingsSyncOnce.Do(func() {
		go s.runCodexSimulationSettingsSync(ctx, interval)
	})
}

func (s *SettingService) runCodexSimulationSettingsSync(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reloadCtx, cancel := context.WithTimeout(ctx, codexSimulationSettingsReloadTimeout)
			err := s.LoadCodexSimulationSettings(reloadCtx)
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.LegacyPrintf("service.setting", "Warning: refresh Codex simulation settings failed: %v", err)
			}
		}
	}
}

// GetCodexSimulationSettings performs an exact control-plane read and updates
// the local runtime snapshot before returning it.
func (s *SettingService) GetCodexSimulationSettings(ctx context.Context) (*CodexSimulationSettings, error) {
	if err := s.LoadCodexSimulationSettings(ctx); err != nil {
		return nil, err
	}
	settings := s.CodexSimulationSettingsSnapshot(ctx)
	return &settings, nil
}

// SetCodexSimulationSettings persists an explicit runtime override. An
// existing DB/YAML secret is preserved; enabling without one generates a
// cryptographically random server-side secret.
func (s *SettingService) SetCodexSimulationSettings(ctx context.Context, requested *CodexSimulationSettings) (*CodexSimulationSettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("setting service is unavailable")
	}
	if requested == nil {
		return nil, fmt.Errorf("settings cannot be nil")
	}
	s.codexSimulationSettingsMu.Lock()
	defer s.codexSimulationSettingsMu.Unlock()

	current, err := s.readCodexSimulationSettings(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(current.IdentitySecret) == "" {
		current.IdentitySecret = s.defaultCodexSimulationSettings().IdentitySecret
	}
	settings := *requested
	providedSecret := strings.TrimSpace(settings.IdentitySecret)
	if providedSecret == "" {
		settings.IdentitySecret = current.IdentitySecret
	}
	mode := strings.ToLower(strings.TrimSpace(settings.ContinuationMode))
	if mode == "" {
		mode = string(codexContinuationOff)
	}
	if (settings.FullSimulationEnabled || mode != string(codexContinuationOff)) && len([]byte(strings.TrimSpace(settings.IdentitySecret))) < 32 {
		if providedSecret != "" {
			return nil, fmt.Errorf("identity secret must be at least 32 bytes when Codex simulation is enabled")
		}
		settings.IdentitySecret, err = generateCodexSimulationIdentitySecret()
		if err != nil {
			return nil, err
		}
	}
	settings, err = validateCodexSimulationSettings(settings)
	if err != nil {
		return nil, err
	}
	return s.persistCodexSimulationSettings(ctx, settings)
}

// ForceDisableCodexSimulationSettings is the control-plane emergency path. It
// deliberately does not depend on decoding the existing DB row, so an invalid
// row or a dirty admin form cannot prevent restoring the legacy OAuth path.
func (s *SettingService) ForceDisableCodexSimulationSettings(ctx context.Context) (*CodexSimulationSettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("setting service is unavailable")
	}
	s.codexSimulationSettingsMu.Lock()
	defer s.codexSimulationSettingsMu.Unlock()

	settings := s.defaultCodexSimulationSettings()
	if cached := s.codexSimulationSettings.Load(); cached != nil {
		settings = *cached
	}
	// Best-effort salvage keeps a valid persisted secret/TTL without making the
	// emergency write dependent on a valid existing settings document.
	if raw, err := s.settingRepo.GetValue(ctx, SettingKeyCodexSimulationSettings); err == nil {
		var persisted CodexSimulationSettings
		if json.Unmarshal([]byte(raw), &persisted) == nil {
			if strings.TrimSpace(persisted.IdentitySecret) != "" {
				settings.IdentitySecret = strings.TrimSpace(persisted.IdentitySecret)
			}
			if validCodexSimulationStateTTLSeconds(persisted.StateTTLSeconds) {
				settings.StateTTLSeconds = persisted.StateTTLSeconds
			}
		}
	}
	if !validCodexSimulationStateTTLSeconds(settings.StateTTLSeconds) {
		settings.StateTTLSeconds = codexSimulationDefaultStateTTLSeconds
	}
	settings.FullSimulationEnabled = false
	settings.CLevelSimulationEnabled = false
	settings.ContinuationMode = string(codexContinuationOff)

	validated, err := validateCodexSimulationSettings(settings)
	if err != nil {
		return nil, err
	}
	return s.persistCodexSimulationSettings(ctx, validated)
}

func (s *SettingService) persistCodexSimulationSettings(ctx context.Context, settings CodexSimulationSettings) (*CodexSimulationSettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("setting service is unavailable")
	}

	raw, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("encode Codex simulation settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyCodexSimulationSettings, string(raw)); err != nil {
		return nil, fmt.Errorf("set Codex simulation settings: %w", err)
	}
	s.codexSimulationSettingsRevision.Add(1)
	s.codexSimulationSettings.Store(&settings)
	codexsimulation.SetCLevelEnabled(settings.CLevelSimulationEnabled)
	return &settings, nil
}

func generateCodexSimulationIdentitySecret() (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generate Codex simulation identity secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(secret), nil
}
