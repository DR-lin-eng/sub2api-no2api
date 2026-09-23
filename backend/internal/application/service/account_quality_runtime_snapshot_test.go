package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type qualityRuntimeSettingRepo struct {
	SettingRepository
	values map[string]string
}

func (r qualityRuntimeSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		result[key] = r.values[key]
	}
	return result, nil
}

func TestAccountQualityRuntimeSnapshotIsPrivateAndCredentialFree(t *testing.T) {
	simulation, err := json.Marshal(CodexSimulationSettings{FullSimulationEnabled: true, ContinuationMode: "shadow"})
	require.NoError(t, err)
	svc := &AccountQualityMonitoringService{settingRepo: qualityRuntimeSettingRepo{values: map[string]string{
		SettingKeyCodexSimulationSettings:              string(simulation),
		SettingKeyOpenAIOAuthGatewayRateLimitEnabled:   "true",
		SettingKeyOpenAIOAuthGatewayRateLimitRPM:       "120",
		SettingKeyOpenAIOAuthGatewayRateLimitBurst:     "8",
		SettingKeyOpenAIRequestIntegrityObserveEnabled: "true",
	}}}
	proxyID := int64(19)
	account := Account{
		ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &proxyID,
		Concurrency: 3, UpdatedAt: time.Unix(100, 0),
		Credentials: map[string]any{"access_token": "must-not-appear", "user_agent": "configured"},
		Extra:       map[string]any{"codex_fingerprint_mode": "session", "enable_tls_fingerprint": true, "tls_fingerprint_profile_id": float64(4)},
	}
	snapshot := qualityRuntimeSnapshot(account, "gpt-test", svc.loadQualityRuntimeConfig(context.Background()))
	require.True(t, snapshot.AccountRateLimitEnabled)
	require.Equal(t, int64(7), snapshot.AccountRateLimitBucketID)
	require.Equal(t, 120, snapshot.AccountRateLimitRPM)
	require.Equal(t, 8, snapshot.AccountRateLimitBurst)
	require.Equal(t, "session", snapshot.CodexFingerprintMode)
	require.Equal(t, "account", snapshot.UserAgentSource)
	require.NotEmpty(t, snapshot.ConfigurationDigest)

	encoded, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "must-not-appear")

	public := publicAccountQualityDetails(AccountQualityProbeDetails{Runtime: snapshot})
	require.Nil(t, public.Runtime)
}
