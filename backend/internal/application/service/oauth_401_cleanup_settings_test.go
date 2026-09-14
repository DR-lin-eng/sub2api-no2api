package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOAuth401CleanupSettingsDefaultOffAndRoundTrip(t *testing.T) {
	repo := &inspectionSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, nil)

	settings, err := svc.GetOAuth401CleanupSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.Enabled)

	require.NoError(t, svc.SetOAuth401CleanupSettings(context.Background(), &OAuth401CleanupSettings{Enabled: true}))
	settings, err = svc.GetOAuth401CleanupSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
}

func TestOAuth401CleanupSettingsMalformedValueFailsClosed(t *testing.T) {
	repo := &inspectionSettingRepoStub{values: map[string]string{SettingKeyOAuth401CleanupSettings: "{"}}
	svc := NewSettingService(repo, nil)

	settings, err := svc.GetOAuth401CleanupSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.Enabled)
}
