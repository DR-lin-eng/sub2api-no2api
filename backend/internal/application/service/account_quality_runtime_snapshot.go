package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/shared/geminicli"
	openaiidentity "github.com/Wei-Shaw/sub2api/internal/shared/openai"
)

// AccountQualityRuntimeSnapshot records the non-secret execution profile that
// can explain differences between quality runs. It is stored privately in the
// artifact JSON and removed from the public quality projection.
type AccountQualityRuntimeSnapshot struct {
	RequestedModel                 string `json:"requested_model,omitempty"`
	MappedModel                    string `json:"mapped_model,omitempty"`
	UpstreamModel                  string `json:"upstream_model,omitempty"`
	ProbeTransport                 string `json:"probe_transport"`
	CodexFingerprintMode           string `json:"codex_fingerprint_mode,omitempty"`
	CodexFullSimulationActive      bool   `json:"codex_full_simulation_active"`
	CodexCLevelSimulationActive    bool   `json:"codex_c_level_simulation_active"`
	CodexExperimentalTransport     bool   `json:"codex_experimental_transport_active"`
	CodexContinuationMode          string `json:"codex_continuation_mode"`
	UserAgentSource                string `json:"user_agent_source"`
	TLSFingerprintConfigured       bool   `json:"tls_fingerprint_configured"`
	TLSFingerprintProfileID        int64  `json:"tls_fingerprint_profile_id,omitempty"`
	EgressMode                     string `json:"egress_mode"`
	ProxyID                        *int64 `json:"proxy_id,omitempty"`
	IPv6PoolID                     int64  `json:"ipv6_pool_id,omitempty"`
	AccountConcurrency             int    `json:"account_concurrency"`
	AccountWebSocketMode           string `json:"account_websocket_mode"`
	AccountRateLimitEnabled        bool   `json:"account_rate_limit_enabled"`
	AccountRateLimitBucketID       int64  `json:"account_rate_limit_bucket_account_id,omitempty"`
	AccountRateLimitRPM            int    `json:"account_rate_limit_rpm,omitempty"`
	AccountRateLimitBurst          int    `json:"account_rate_limit_burst,omitempty"`
	RequestIntegrityObserveEnabled bool   `json:"request_integrity_observe_enabled"`
	AccountRevisionUnixMicro       int64  `json:"account_revision_unix_micro,omitempty"`
	ConfigurationDigest            string `json:"configuration_digest"`
}

type accountQualityRuntimeConfig struct {
	simulation                     CodexSimulationSettings
	globalUserAgent                bool
	accountRateLimitEnabled        bool
	accountRateLimitRPM            int
	accountRateLimitBurst          int
	requestIntegrityObserveEnabled bool
}

func (s *AccountQualityMonitoringService) loadQualityRuntimeConfig(ctx context.Context) accountQualityRuntimeConfig {
	config := accountQualityRuntimeConfig{
		simulation:            CodexSimulationSettings{ContinuationMode: string(codexContinuationOff)},
		accountRateLimitRPM:   60,
		accountRateLimitBurst: 5,
	}
	if s == nil || s.settingRepo == nil {
		return config
	}
	keys := []string{
		SettingKeyCodexSimulationSettings,
		SettingKeyOpenAICodexUserAgent,
		SettingKeyOpenAIOAuthGatewayRateLimitEnabled,
		SettingKeyOpenAIOAuthGatewayRateLimitRPM,
		SettingKeyOpenAIOAuthGatewayRateLimitBurst,
		SettingKeyOpenAIRequestIntegrityObserveEnabled,
	}
	values, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return config
	}
	_ = json.Unmarshal([]byte(values[SettingKeyCodexSimulationSettings]), &config.simulation)
	config.globalUserAgent = strings.TrimSpace(values[SettingKeyOpenAICodexUserAgent]) != ""
	config.accountRateLimitEnabled = strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAIOAuthGatewayRateLimitEnabled]), "true")
	config.accountRateLimitRPM = qualitySnapshotInt(values[SettingKeyOpenAIOAuthGatewayRateLimitRPM], 60)
	config.accountRateLimitBurst = qualitySnapshotInt(values[SettingKeyOpenAIOAuthGatewayRateLimitBurst], 5)
	config.requestIntegrityObserveEnabled = strings.EqualFold(strings.TrimSpace(values[SettingKeyOpenAIRequestIntegrityObserveEnabled]), "true")
	return config
}

func qualityRuntimeSnapshot(account Account, model string, config accountQualityRuntimeConfig) *AccountQualityRuntimeSnapshot {
	requestedModel := strings.TrimSpace(model)
	if requestedModel == "" {
		switch account.Platform {
		case PlatformOpenAI:
			requestedModel = openaiidentity.DefaultTestModel
		case PlatformGemini:
			requestedModel = geminicli.DefaultTestModel
		}
	}
	mappedModel := strings.TrimSpace(account.GetMappedModel(requestedModel))
	upstreamModel := mappedModel
	if account.IsOpenAIOAuth() {
		upstreamModel = normalizeOpenAIModelForUpstream(&account, mappedModel)
	}
	codexAccount := account.IsOpenAIOAuth()
	fullSimulationActive := codexAccount && account.GetCodexFingerprintMode() == codexFingerprintFull && config.simulation.FullSimulationEnabled
	cLevelActive := codexAccount && config.simulation.CLevelSimulationEnabled
	accountLimitActive := codexAccount && config.accountRateLimitEnabled
	snapshot := &AccountQualityRuntimeSnapshot{
		RequestedModel:                 requestedModel,
		MappedModel:                    mappedModel,
		UpstreamModel:                  upstreamModel,
		ProbeTransport:                 "http_sse",
		CodexFingerprintMode:           string(account.GetCodexFingerprintMode()),
		CodexFullSimulationActive:      fullSimulationActive,
		CodexCLevelSimulationActive:    cLevelActive,
		CodexExperimentalTransport:     cLevelActive && config.simulation.ExperimentalTransportEnabled,
		CodexContinuationMode:          string(codexContinuationOff),
		UserAgentSource:                "default",
		TLSFingerprintConfigured:       account.IsTLSFingerprintEnabled(),
		TLSFingerprintProfileID:        account.GetTLSFingerprintProfileID(),
		EgressMode:                     string(account.EgressRoute().Mode),
		ProxyID:                        cloneAccountValuePointer(account.ProxyID),
		IPv6PoolID:                     account.EgressRoute().PoolID,
		AccountConcurrency:             account.Concurrency,
		AccountWebSocketMode:           account.ResolveOpenAIResponsesWebSocketV2Mode("off"),
		AccountRevisionUnixMicro:       account.UpdatedAt.UnixMicro(),
		AccountRateLimitEnabled:        accountLimitActive,
		RequestIntegrityObserveEnabled: codexAccount && config.requestIntegrityObserveEnabled,
	}
	if codexAccount {
		snapshot.CodexContinuationMode = string(config.simulation.continuationMode())
	}
	if accountLimitActive {
		snapshot.AccountRateLimitBucketID = openAIOAuthGatewayRateLimitAccountID(&account)
		snapshot.AccountRateLimitRPM = config.accountRateLimitRPM
		snapshot.AccountRateLimitBurst = config.accountRateLimitBurst
	}
	if strings.TrimSpace(account.GetOpenAIUserAgent()) != "" {
		snapshot.UserAgentSource = "account"
	} else if config.globalUserAgent {
		snapshot.UserAgentSource = "global"
	}
	encoded, _ := json.Marshal(snapshot)
	digest := sha256.Sum256(encoded)
	snapshot.ConfigurationDigest = hex.EncodeToString(digest[:])
	return snapshot
}

func qualitySnapshotInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return fallback
	}
	return value
}
