//go:build unit

package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLegacyAdminKeyCanGrantVoucherPermissionWithoutChangingSecret(t *testing.T) {
	ctx := t.Context()
	secret := "legacy-fixture-key-without-rotation"
	repo := &adminAPIKeySettingStub{values: map[string]string{SettingKeyAdminAPIKey: secret}}
	svc := NewSettingService(repo, nil)
	auth, err := svc.AuthenticateAdminAPIKey(ctx, secret)
	require.NoError(t, err)
	require.Equal(t, []string{AdminAPIKeyScopeRead}, auth.Scopes)
	scopes := []string{AdminAPIKeyScopeRead, AdminAPIKeyScopeRedeemSecrets}
	updated, err := svc.UpdateAdminAPIKey(ctx, "legacy", AdminAPIKeyUpdateInput{Scopes: &scopes})
	require.NoError(t, err)
	require.Equal(t, scopes, updated.Scopes)
	require.Equal(t, secret, repo.values[SettingKeyAdminAPIKey])
	require.NotContains(t, repo.values[legacyAdminKeyOptionsSetting], secret)
	auth, err = svc.AuthenticateAdminAPIKey(ctx, secret)
	require.NoError(t, err)
	require.Equal(t, scopes, auth.Scopes)
	keys, err := svc.ListAdminAPIKeys(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, scopes, keys[0].Scopes)
	// A new raw legacy key is a new identity and defaults to restricted read.
	repo.values[SettingKeyAdminAPIKey] = "rotated-legacy-fixture"
	auth, err = svc.AuthenticateAdminAPIKey(ctx, "rotated-legacy-fixture")
	require.NoError(t, err)
	require.Equal(t, []string{AdminAPIKeyScopeRead}, auth.Scopes)
	_, err = svc.AuthenticateAdminAPIKey(ctx, secret)
	require.Error(t, err)
}
