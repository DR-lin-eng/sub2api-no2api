//go:build unit

package service

import (
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAuxiliarySettingsReplicaRefreshAndStoreFailure(t *testing.T) {
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeyOpenAIOAuthAttestationEnabled: "true", SettingKeyOpenAILenovoAttributionEnabled: "true",
	}}
	svc := NewSettingService(repo, &config.Config{})
	require.True(t, svc.OpenAIOAuthAttestationEnabled(t.Context()))
	require.True(t, svc.OpenAILenovoAttributionEnabled(t.Context()))
	require.Equal(t, 1, repo.callCount(), "HTTP/WS share one settings cache")
	repo.setValues(map[string]string{SettingKeyOpenAIOAuthAttestationEnabled: "false", SettingKeyOpenAILenovoAttributionEnabled: "false"})
	require.True(t, svc.OpenAILenovoAttributionEnabled(t.Context()))
	old := *svc.openAIAuxiliaryCache.Load()
	old.expiresAt = time.Now().Add(-time.Second)
	svc.openAIAuxiliaryCache.Store(&old)
	require.False(t, svc.OpenAILenovoAttributionEnabled(t.Context()), "other instance's persisted update must become visible")
	require.False(t, svc.OpenAIOAuthAttestationEnabled(t.Context()))
	require.Equal(t, 2, repo.callCount())
	repo.err = errors.New("store offline")
	old = *svc.openAIAuxiliaryCache.Load()
	old.expiresAt = time.Now().Add(-time.Second)
	svc.openAIAuxiliaryCache.Store(&old)
	require.False(t, svc.OpenAILenovoAttributionEnabled(t.Context()), "store failure must not revive an old enabled state")
	require.Equal(t, 3, repo.callCount())
}

func TestOpenAIAuxiliarySettingsMissingOverrideUsesConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIOAuthAttestationEnabled = true
	svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, cfg)
	require.True(t, svc.OpenAIOAuthAttestationEnabled(t.Context()))
	require.False(t, svc.OpenAILenovoAttributionEnabled(t.Context()))
}
