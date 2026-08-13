package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestSettingServiceOpenAIVisibleOutputTTFTDefaultsEnabled(t *testing.T) {
	resetGatewayForwardingSettingsCacheForTest(t)
	repo := &gatewayTTLSettingRepo{data: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	require.True(t, svc.parseSettings(repo.data).OpenAIVisibleOutputTTFTEnabled)
	require.True(t, svc.IsOpenAIVisibleOutputTTFTEnabled(context.Background()))
}

func TestSettingServiceOpenAIVisibleOutputTTFTPersistsAndRefreshesCache(t *testing.T) {
	resetGatewayForwardingSettingsCacheForTest(t)
	repo := &gatewayTTLSettingRepo{data: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	settings := svc.parseSettings(repo.data)
	settings.OpenAIVisibleOutputTTFTEnabled = false

	require.NoError(t, svc.UpdateSettings(context.Background(), settings))
	require.Equal(t, "false", repo.data[SettingKeyOpenAIVisibleOutputTTFTEnabled])
	require.False(t, svc.IsOpenAIVisibleOutputTTFTEnabled(context.Background()))
}
