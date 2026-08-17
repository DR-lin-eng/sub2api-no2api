package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

const (
	CloudflareIngressModeZoneAccessRules = "zone_access_rules"
	CloudflareIngressModeWAFCustomRules  = "waf_custom_rules"

	CloudflareIngressDefaultRequestTimeoutSeconds    = 5
	CloudflareIngressDefaultQueueCapacity            = 1024
	CloudflareIngressDefaultMaxActiveRules           = 1000
	CloudflareIngressDefaultReconcileIntervalSeconds = 300
	CloudflareIngressDefaultWAFSyncIntervalSeconds   = 15
	CloudflareIngressDefaultAnalyticsIntervalSeconds = 300

	CloudflareIngressMinRequestTimeoutSeconds    = 1
	CloudflareIngressMaxRequestTimeoutSeconds    = 30
	CloudflareIngressMinQueueCapacity            = 16
	CloudflareIngressMaxQueueCapacity            = 100_000
	CloudflareIngressMinMaxActiveRules           = 1
	CloudflareIngressMaxMaxActiveRules           = 50_000
	CloudflareIngressMinReconcileIntervalSeconds = 30
	CloudflareIngressMaxReconcileIntervalSeconds = 3600
	CloudflareIngressMinWAFSyncIntervalSeconds   = 5
	CloudflareIngressMaxWAFSyncIntervalSeconds   = 300
	CloudflareIngressMinAnalyticsIntervalSeconds = 60
	CloudflareIngressMaxAnalyticsIntervalSeconds = 3600
	CloudflareIngressMaxWAFRuleIDs               = 100
	CloudflareIngressMaxWAFHostnames             = 50

	cloudflareIngressSettingsVersion  = 3
	cloudflareIngressTokenMaxBytes    = 4096
	cloudflareIngressHostnameMaxBytes = 2048
)

var (
	ErrCloudflareIngressUnavailable = infraerrors.ServiceUnavailable(
		"CLOUDFLARE_INGRESS_UNAVAILABLE",
		"Cloudflare ingress settings runtime is unavailable",
	)
	ErrCloudflareIngressCredentialsBusy = infraerrors.Conflict(
		"CLOUDFLARE_INGRESS_CREDENTIALS_BUSY",
		"disable Cloudflare edge blocking and wait for queued and active rules to reach zero before changing credentials",
	)
)

// CloudflareIngressSettings is the complete in-process configuration. APIToken
// is never serialized into an HTTP response; the admin API uses the separate
// view and update types below.
type CloudflareIngressSettings struct {
	Enabled                  bool
	Mode                     string
	ZoneID                   string
	APIToken                 string
	WAFHostname              string
	WAFHostnames             []string
	WAFRuleIDs               []string
	WAFSyncIntervalSeconds   int
	AnalyticsIntervalSeconds int
	RequestTimeoutSeconds    int
	QueueCapacity            int
	MaxActiveRules           int
	ReconcileIntervalSeconds int
}

type CloudflareIngressSettingsView struct {
	Enabled                  bool     `json:"enabled"`
	Mode                     string   `json:"mode"`
	ZoneID                   string   `json:"zone_id"`
	APITokenConfigured       bool     `json:"api_token_configured"`
	WAFHostname              string   `json:"waf_hostname"`
	WAFHostnames             []string `json:"waf_hostnames"`
	WAFRuleIDs               []string `json:"waf_rule_ids"`
	WAFSyncIntervalSeconds   int      `json:"waf_sync_interval_seconds"`
	AnalyticsIntervalSeconds int      `json:"analytics_interval_seconds"`
	RequestTimeoutSeconds    int      `json:"request_timeout_seconds"`
	QueueCapacity            int      `json:"queue_capacity"`
	MaxActiveRules           int      `json:"max_active_rules"`
	ReconcileIntervalSeconds int      `json:"reconcile_interval_seconds"`
}

type UpdateCloudflareIngressSettingsInput struct {
	Enabled                  bool     `json:"enabled"`
	Mode                     string   `json:"mode"`
	ZoneID                   string   `json:"zone_id"`
	APIToken                 string   `json:"api_token"`
	WAFHostname              string   `json:"waf_hostname"`
	WAFHostnames             []string `json:"waf_hostnames"`
	WAFRuleIDs               []string `json:"waf_rule_ids"`
	WAFSyncIntervalSeconds   int      `json:"waf_sync_interval_seconds"`
	AnalyticsIntervalSeconds int      `json:"analytics_interval_seconds"`
	RequestTimeoutSeconds    int      `json:"request_timeout_seconds"`
	QueueCapacity            int      `json:"queue_capacity"`
	MaxActiveRules           int      `json:"max_active_rules"`
	ReconcileIntervalSeconds int      `json:"reconcile_interval_seconds"`
}

type persistedCloudflareIngressSettings struct {
	Version                  int      `json:"version"`
	Enabled                  bool     `json:"enabled"`
	Mode                     string   `json:"mode,omitempty"`
	ZoneID                   string   `json:"zone_id"`
	APITokenCiphertext       string   `json:"api_token_ciphertext,omitempty"`
	WAFHostname              string   `json:"waf_hostname,omitempty"`
	WAFHostnames             []string `json:"waf_hostnames,omitempty"`
	WAFRuleIDs               []string `json:"waf_rule_ids,omitempty"`
	WAFSyncIntervalSeconds   int      `json:"waf_sync_interval_seconds,omitempty"`
	AnalyticsIntervalSeconds int      `json:"analytics_interval_seconds,omitempty"`
	RequestTimeoutSeconds    int      `json:"request_timeout_seconds"`
	QueueCapacity            int      `json:"queue_capacity"`
	MaxActiveRules           int      `json:"max_active_rules"`
	ReconcileIntervalSeconds int      `json:"reconcile_interval_seconds"`
}

// CloudflareIngressSettingsController is implemented by the infrastructure
// worker. The settings service uses it to validate credentials and apply a new
// immutable runtime snapshot without restarting the process.
type CloudflareIngressSettingsController interface {
	ValidateCloudflareIngressSettings(ctx context.Context, settings CloudflareIngressSettings) error
	ApplyCloudflareIngressSettings(ctx context.Context, settings CloudflareIngressSettings) error
}

type CloudflareIngressSettingService struct {
	settingRepo             SettingRepository
	encryptor               SecretEncryptor
	edge                    InvalidAuthEdgeBlocker
	localInvalidAuthEnabled bool
}

func NewCloudflareIngressSettingService(
	settingRepo SettingRepository,
	encryptor SecretEncryptor,
	edge InvalidAuthEdgeBlocker,
	cfg *config.Config,
) *CloudflareIngressSettingService {
	return &CloudflareIngressSettingService{
		settingRepo:             settingRepo,
		encryptor:               encryptor,
		edge:                    edge,
		localInvalidAuthEnabled: cfg != nil && cfg.APIKeyAuth.InvalidAbuse.Enabled,
	}
}

func DefaultCloudflareIngressSettings() CloudflareIngressSettings {
	return CloudflareIngressSettings{
		Mode:                     CloudflareIngressModeZoneAccessRules,
		WAFSyncIntervalSeconds:   CloudflareIngressDefaultWAFSyncIntervalSeconds,
		AnalyticsIntervalSeconds: CloudflareIngressDefaultAnalyticsIntervalSeconds,
		RequestTimeoutSeconds:    CloudflareIngressDefaultRequestTimeoutSeconds,
		QueueCapacity:            CloudflareIngressDefaultQueueCapacity,
		MaxActiveRules:           CloudflareIngressDefaultMaxActiveRules,
		ReconcileIntervalSeconds: CloudflareIngressDefaultReconcileIntervalSeconds,
	}
}

func (s *CloudflareIngressSettingService) Get(ctx context.Context) (*CloudflareIngressSettingsView, error) {
	if s == nil || s.settingRepo == nil || s.encryptor == nil {
		return nil, ErrCloudflareIngressUnavailable
	}
	settings, _, err := loadCloudflareIngressSettings(ctx, s.settingRepo, s.encryptor)
	if err != nil {
		return nil, err
	}
	view := cloudflareIngressSettingsView(settings)
	return &view, nil
}

// Update persists an encrypted token and immediately swaps the worker's
// runtime snapshot. An empty api_token retains the previously saved token.
func (s *CloudflareIngressSettingService) Update(
	ctx context.Context,
	input UpdateCloudflareIngressSettingsInput,
) (*CloudflareIngressSettingsView, error) {
	if s == nil || s.settingRepo == nil || s.encryptor == nil {
		return nil, ErrCloudflareIngressUnavailable
	}
	current, currentRecord, err := loadCloudflareIngressSettings(ctx, s.settingRepo, s.encryptor)
	if err != nil {
		return nil, err
	}

	hostnames := normalizeCloudflareHostnames(input.WAFHostnames)
	if input.WAFHostnames == nil && input.WAFHostname != "" {
		legacyHostname := normalizeCloudflareHostname(input.WAFHostname)
		if legacyHostname == current.WAFHostname && len(current.WAFHostnames) > 1 {
			hostnames = slices.Clone(current.WAFHostnames)
		} else {
			hostnames = normalizeCloudflareHostnames([]string{legacyHostname})
		}
	}
	candidate := CloudflareIngressSettings{
		Enabled:                  input.Enabled,
		Mode:                     normalizeCloudflareIngressMode(input.Mode),
		ZoneID:                   strings.ToLower(strings.TrimSpace(input.ZoneID)),
		APIToken:                 strings.TrimSpace(input.APIToken),
		WAFHostname:              firstCloudflareHostname(hostnames),
		WAFHostnames:             hostnames,
		WAFRuleIDs:               normalizeCloudflareWAFRuleIDs(input.WAFRuleIDs),
		WAFSyncIntervalSeconds:   input.WAFSyncIntervalSeconds,
		AnalyticsIntervalSeconds: input.AnalyticsIntervalSeconds,
		RequestTimeoutSeconds:    input.RequestTimeoutSeconds,
		QueueCapacity:            input.QueueCapacity,
		MaxActiveRules:           input.MaxActiveRules,
		ReconcileIntervalSeconds: input.ReconcileIntervalSeconds,
	}
	defaults := DefaultCloudflareIngressSettings()
	if candidate.WAFSyncIntervalSeconds == 0 {
		candidate.WAFSyncIntervalSeconds = defaults.WAFSyncIntervalSeconds
	}
	if candidate.AnalyticsIntervalSeconds == 0 {
		candidate.AnalyticsIntervalSeconds = defaults.AnalyticsIntervalSeconds
	}
	if candidate.APIToken == "" {
		candidate.APIToken = current.APIToken
	}
	if err := validateCloudflareIngressSettings(candidate, s.localInvalidAuthEnabled); err != nil {
		return nil, err
	}

	bindingChanged := cloudflareIngressBindingChanged(current, candidate)
	if bindingChanged && currentCredentialsBusy(current, s.edge) {
		return nil, ErrCloudflareIngressCredentialsBusy
	}
	controller, ok := s.edge.(CloudflareIngressSettingsController)
	if !ok || controller == nil {
		return nil, ErrCloudflareIngressUnavailable
	}
	if candidate.Enabled {
		if err := controller.ValidateCloudflareIngressSettings(ctx, candidate); err != nil {
			return nil, infraerrors.BadRequest("CLOUDFLARE_INGRESS_VALIDATION_FAILED", err.Error())
		}
	}

	record := persistedCloudflareIngressSettings{
		Version:                  cloudflareIngressSettingsVersion,
		Enabled:                  candidate.Enabled,
		Mode:                     candidate.Mode,
		ZoneID:                   candidate.ZoneID,
		APITokenCiphertext:       currentRecord.APITokenCiphertext,
		WAFHostname:              candidate.WAFHostname,
		WAFHostnames:             slices.Clone(candidate.WAFHostnames),
		WAFRuleIDs:               slices.Clone(candidate.WAFRuleIDs),
		WAFSyncIntervalSeconds:   candidate.WAFSyncIntervalSeconds,
		AnalyticsIntervalSeconds: candidate.AnalyticsIntervalSeconds,
		RequestTimeoutSeconds:    candidate.RequestTimeoutSeconds,
		QueueCapacity:            candidate.QueueCapacity,
		MaxActiveRules:           candidate.MaxActiveRules,
		ReconcileIntervalSeconds: candidate.ReconcileIntervalSeconds,
	}
	if strings.TrimSpace(input.APIToken) != "" {
		record.APITokenCiphertext, err = s.encryptor.Encrypt(candidate.APIToken)
		if err != nil {
			return nil, fmt.Errorf("encrypt Cloudflare ingress API token: %w", err)
		}
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("marshal Cloudflare ingress settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyCloudflareIngressSettings, string(encoded)); err != nil {
		return nil, fmt.Errorf("save Cloudflare ingress settings: %w", err)
	}
	if err := controller.ApplyCloudflareIngressSettings(ctx, candidate); err != nil {
		return nil, fmt.Errorf("apply Cloudflare ingress settings: %w", err)
	}

	view := cloudflareIngressSettingsView(candidate)
	return &view, nil
}

// LoadPersistedCloudflareIngressSettings is used by the infrastructure worker
// at startup and during multi-instance polling. The returned token is plaintext
// only in memory after decryption.
func LoadPersistedCloudflareIngressSettings(
	ctx context.Context,
	settingRepo SettingRepository,
	encryptor SecretEncryptor,
) (CloudflareIngressSettings, error) {
	settings, _, err := loadCloudflareIngressSettings(ctx, settingRepo, encryptor)
	return settings, err
}

func loadCloudflareIngressSettings(
	ctx context.Context,
	settingRepo SettingRepository,
	encryptor SecretEncryptor,
) (CloudflareIngressSettings, persistedCloudflareIngressSettings, error) {
	defaults := DefaultCloudflareIngressSettings()
	if settingRepo == nil {
		return defaults, persistedCloudflareIngressSettings{}, ErrCloudflareIngressUnavailable
	}
	raw, err := settingRepo.GetValue(ctx, SettingKeyCloudflareIngressSettings)
	if errors.Is(err, ErrSettingNotFound) {
		return defaults, persistedCloudflareIngressSettings{}, nil
	}
	if err != nil {
		return defaults, persistedCloudflareIngressSettings{}, fmt.Errorf("load Cloudflare ingress settings: %w", err)
	}

	var record persistedCloudflareIngressSettings
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return defaults, record, fmt.Errorf("parse Cloudflare ingress settings: %w", err)
	}
	hostnames := normalizeCloudflareHostnames(record.WAFHostnames)
	if len(hostnames) == 0 && record.WAFHostname != "" {
		hostnames = normalizeCloudflareHostnames([]string{record.WAFHostname})
	}
	settings := CloudflareIngressSettings{
		Enabled:                  record.Enabled,
		Mode:                     normalizeCloudflareIngressMode(record.Mode),
		ZoneID:                   strings.ToLower(strings.TrimSpace(record.ZoneID)),
		WAFHostname:              firstCloudflareHostname(hostnames),
		WAFHostnames:             hostnames,
		WAFRuleIDs:               normalizeCloudflareWAFRuleIDs(record.WAFRuleIDs),
		WAFSyncIntervalSeconds:   normalizeRange(record.WAFSyncIntervalSeconds, CloudflareIngressMinWAFSyncIntervalSeconds, CloudflareIngressMaxWAFSyncIntervalSeconds, defaults.WAFSyncIntervalSeconds),
		AnalyticsIntervalSeconds: normalizeRange(record.AnalyticsIntervalSeconds, CloudflareIngressMinAnalyticsIntervalSeconds, CloudflareIngressMaxAnalyticsIntervalSeconds, defaults.AnalyticsIntervalSeconds),
		RequestTimeoutSeconds:    normalizeRange(record.RequestTimeoutSeconds, CloudflareIngressMinRequestTimeoutSeconds, CloudflareIngressMaxRequestTimeoutSeconds, defaults.RequestTimeoutSeconds),
		QueueCapacity:            normalizeRange(record.QueueCapacity, CloudflareIngressMinQueueCapacity, CloudflareIngressMaxQueueCapacity, defaults.QueueCapacity),
		MaxActiveRules:           normalizeRange(record.MaxActiveRules, CloudflareIngressMinMaxActiveRules, CloudflareIngressMaxMaxActiveRules, defaults.MaxActiveRules),
		ReconcileIntervalSeconds: normalizeRange(record.ReconcileIntervalSeconds, CloudflareIngressMinReconcileIntervalSeconds, CloudflareIngressMaxReconcileIntervalSeconds, defaults.ReconcileIntervalSeconds),
	}
	if record.APITokenCiphertext != "" {
		if encryptor == nil {
			return defaults, record, ErrCloudflareIngressUnavailable
		}
		settings.APIToken, err = encryptor.Decrypt(record.APITokenCiphertext)
		if err != nil {
			return defaults, record, fmt.Errorf("decrypt Cloudflare ingress API token: %w", err)
		}
		settings.APIToken = strings.TrimSpace(settings.APIToken)
	}
	return settings, record, nil
}

func validateCloudflareIngressSettings(settings CloudflareIngressSettings, localInvalidAuthEnabled bool) error {
	if settings.Mode != CloudflareIngressModeZoneAccessRules && settings.Mode != CloudflareIngressModeWAFCustomRules {
		return infraerrors.BadRequest("CLOUDFLARE_INGRESS_MODE_INVALID", "unsupported Cloudflare ingress blocking mode")
	}
	if settings.Enabled && !localInvalidAuthEnabled {
		return infraerrors.BadRequest("CLOUDFLARE_INGRESS_LOCAL_LIMITER_DISABLED", "enable api_key_auth_cache.invalid_abuse before enabling Cloudflare edge blocking")
	}
	if settings.Enabled && strings.TrimSpace(settings.APIToken) == "" {
		return infraerrors.BadRequest("CLOUDFLARE_INGRESS_TOKEN_REQUIRED", "Cloudflare API token is required")
	}
	if settings.Enabled && strings.TrimSpace(settings.ZoneID) == "" {
		return infraerrors.BadRequest("CLOUDFLARE_INGRESS_ZONE_REQUIRED", "Cloudflare zone ID is required")
	}
	if settings.ZoneID != "" && !isCloudflareZoneID(settings.ZoneID) {
		return infraerrors.BadRequest("CLOUDFLARE_INGRESS_ZONE_INVALID", "Cloudflare zone ID must be a 32-character hexadecimal ID")
	}
	if len(settings.APIToken) > cloudflareIngressTokenMaxBytes {
		return infraerrors.BadRequest("CLOUDFLARE_INGRESS_TOKEN_TOO_LONG", "Cloudflare API token is too long")
	}
	if settings.RequestTimeoutSeconds < CloudflareIngressMinRequestTimeoutSeconds || settings.RequestTimeoutSeconds > CloudflareIngressMaxRequestTimeoutSeconds {
		return infraerrors.BadRequest("CLOUDFLARE_INGRESS_TIMEOUT_INVALID", "request timeout must be between 1 and 30 seconds")
	}
	if settings.QueueCapacity < CloudflareIngressMinQueueCapacity || settings.QueueCapacity > CloudflareIngressMaxQueueCapacity {
		return infraerrors.BadRequest("CLOUDFLARE_INGRESS_QUEUE_INVALID", "queue capacity must be between 16 and 100000")
	}
	if settings.MaxActiveRules < CloudflareIngressMinMaxActiveRules || settings.MaxActiveRules > CloudflareIngressMaxMaxActiveRules {
		return infraerrors.BadRequest("CLOUDFLARE_INGRESS_RULE_LIMIT_INVALID", "active rule limit must be between 1 and 50000")
	}
	if settings.ReconcileIntervalSeconds < CloudflareIngressMinReconcileIntervalSeconds || settings.ReconcileIntervalSeconds > CloudflareIngressMaxReconcileIntervalSeconds {
		return infraerrors.BadRequest("CLOUDFLARE_INGRESS_RECONCILE_INVALID", "reconcile interval must be between 30 and 3600 seconds")
	}
	if settings.WAFSyncIntervalSeconds < CloudflareIngressMinWAFSyncIntervalSeconds || settings.WAFSyncIntervalSeconds > CloudflareIngressMaxWAFSyncIntervalSeconds {
		return infraerrors.BadRequest("CLOUDFLARE_INGRESS_WAF_SYNC_INVALID", "WAF sync interval must be between 5 and 300 seconds")
	}
	if settings.AnalyticsIntervalSeconds < CloudflareIngressMinAnalyticsIntervalSeconds || settings.AnalyticsIntervalSeconds > CloudflareIngressMaxAnalyticsIntervalSeconds {
		return infraerrors.BadRequest("CLOUDFLARE_INGRESS_ANALYTICS_INTERVAL_INVALID", "analytics interval must be between 60 and 3600 seconds")
	}
	if len(settings.WAFHostnames) > CloudflareIngressMaxWAFHostnames {
		return infraerrors.BadRequest("CLOUDFLARE_INGRESS_WAF_HOSTNAME_INVALID", "too many WAF hostnames")
	}
	hostnameBytes := 0
	for _, hostname := range settings.WAFHostnames {
		if !isCloudflareHostname(hostname) {
			return infraerrors.BadRequest("CLOUDFLARE_INGRESS_WAF_HOSTNAME_INVALID", "WAF hostnames must be valid ASCII hostnames")
		}
		hostnameBytes += len(hostname) + 3
	}
	if hostnameBytes > cloudflareIngressHostnameMaxBytes {
		return infraerrors.BadRequest("CLOUDFLARE_INGRESS_WAF_HOSTNAME_INVALID", "WAF hostname expression is too long")
	}
	if len(settings.WAFRuleIDs) > CloudflareIngressMaxWAFRuleIDs {
		return infraerrors.BadRequest("CLOUDFLARE_INGRESS_WAF_RULES_INVALID", "too many WAF rule IDs")
	}
	for _, ruleID := range settings.WAFRuleIDs {
		if !isCloudflareZoneID(ruleID) {
			return infraerrors.BadRequest("CLOUDFLARE_INGRESS_WAF_RULES_INVALID", "WAF rule IDs must be 32-character hexadecimal IDs")
		}
	}
	if settings.Enabled && settings.Mode == CloudflareIngressModeWAFCustomRules {
		if len(settings.WAFHostnames) == 0 {
			return infraerrors.BadRequest("CLOUDFLARE_INGRESS_WAF_HOSTNAME_REQUIRED", "at least one WAF hostname is required")
		}
		if len(settings.WAFRuleIDs) == 0 {
			return infraerrors.BadRequest("CLOUDFLARE_INGRESS_WAF_RULES_REQUIRED", "at least one WAF rule ID is required")
		}
	}
	return nil
}

func cloudflareIngressSettingsView(settings CloudflareIngressSettings) CloudflareIngressSettingsView {
	return CloudflareIngressSettingsView{
		Enabled:                  settings.Enabled,
		Mode:                     settings.Mode,
		ZoneID:                   settings.ZoneID,
		APITokenConfigured:       settings.APIToken != "",
		WAFHostname:              settings.WAFHostname,
		WAFHostnames:             slices.Clone(settings.WAFHostnames),
		WAFRuleIDs:               slices.Clone(settings.WAFRuleIDs),
		WAFSyncIntervalSeconds:   settings.WAFSyncIntervalSeconds,
		AnalyticsIntervalSeconds: settings.AnalyticsIntervalSeconds,
		RequestTimeoutSeconds:    settings.RequestTimeoutSeconds,
		QueueCapacity:            settings.QueueCapacity,
		MaxActiveRules:           settings.MaxActiveRules,
		ReconcileIntervalSeconds: settings.ReconcileIntervalSeconds,
	}
}

func cloudflareIngressBindingChanged(current, candidate CloudflareIngressSettings) bool {
	return current.Mode != candidate.Mode ||
		current.ZoneID != candidate.ZoneID ||
		current.APIToken != candidate.APIToken ||
		!slices.Equal(current.WAFHostnames, candidate.WAFHostnames) ||
		!slices.Equal(current.WAFRuleIDs, candidate.WAFRuleIDs)
}

func currentCredentialsBusy(current CloudflareIngressSettings, edge InvalidAuthEdgeBlocker) bool {
	if current.ZoneID == "" && current.APIToken == "" {
		return false
	}
	if current.Enabled {
		return true
	}
	if edge == nil {
		return false
	}
	health := edge.Health()
	return health.ActiveRules > 0 || health.QueueDepth > 0 ||
		(health.WAF != nil && (health.WAF.SyncedEntries > 0 || health.WAF.OverflowEntries > 0))
}

func normalizeRange(value, minimum, maximum, fallback int) int {
	if value < minimum || value > maximum {
		return fallback
	}
	return value
}

func isCloudflareZoneID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 32 {
		return false
	}
	for index := range len(value) {
		character := value[index]
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') && (character < 'A' || character > 'F') {
			return false
		}
	}
	return true
}

func normalizeCloudflareIngressMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return CloudflareIngressModeZoneAccessRules
	}
	return value
}

func normalizeCloudflareHostname(value string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
}

func normalizeCloudflareHostnames(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = normalizeCloudflareHostname(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func firstCloudflareHostname(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func normalizeCloudflareWAFRuleIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func isCloudflareHostname(value string) bool {
	value = normalizeCloudflareHostname(value)
	if value == "" || len(value) > 253 {
		return false
	}
	if _, err := netip.ParseAddr(value); err == nil {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for index := range len(label) {
			character := label[index]
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return strings.Contains(value, ".")
}
