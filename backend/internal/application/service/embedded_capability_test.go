//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbeddedCapabilityCannotAuthenticateBackToSub2API(t *testing.T) {
	user := &User{ID: 42, Email: "user@example.test", Role: RoleUser, Status: StatusActive, TokenVersion: 7}
	svc := newAuthService(&userRepoStub{user: user}, map[string]string{
		SettingKeyCustomMenuItems: `[{"id":"billing","url":"https://billing.example.test/embed","visibility":"user","forward_access_token":true}]`,
	}, nil, nil)

	target, err := svc.settingService.ResolveEmbeddedCapabilityTarget(
		context.Background(),
		"billing",
		"https://billing.example.test",
		RoleUser,
	)
	require.NoError(t, err)
	issued, err := svc.IssueEmbeddedCapability(user, target)
	require.NoError(t, err)
	require.NotEmpty(t, issued.Token)

	_, err = svc.ValidateToken(issued.Token)
	require.ErrorIs(t, err, ErrInvalidToken, "the normal JWT signing domain must reject embedded capabilities")

	claims, err := svc.VerifyEmbeddedCapability(
		context.Background(),
		issued.Token,
		"https://billing.example.test",
	)
	require.NoError(t, err)
	require.Equal(t, int64(42), claims.UserID)
	require.Equal(t, "billing", claims.MenuID)
	require.Equal(t, []string{embeddedCapabilityPermission}, claims.Permissions)

	_, err = svc.VerifyEmbeddedCapability(context.Background(), issued.Token, "https://attacker.example.test")
	require.ErrorIs(t, err, ErrEmbeddedCapabilityInvalid)
}

func TestEmbeddedCapabilityRequiresCurrentExplicitMenuOptIn(t *testing.T) {
	user := &User{ID: 7, Role: RoleUser, Status: StatusActive, TokenVersion: 2}
	settings := &settingRepoStub{values: map[string]string{
		SettingKeyCustomMenuItems: `[{"id":"help","url":"https://help.example.test/embed","visibility":"user","forward_access_token":true}]`,
	}}
	svc := newAuthService(&userRepoStub{user: user}, nil, nil, nil)
	svc.settingService = NewSettingService(settings, svc.cfg)

	target, err := svc.settingService.ResolveEmbeddedCapabilityTarget(
		context.Background(), "help", "https://help.example.test", RoleUser,
	)
	require.NoError(t, err)
	issued, err := svc.IssueEmbeddedCapability(user, target)
	require.NoError(t, err)

	settings.values[SettingKeyCustomMenuItems] = `[{"id":"help","url":"https://help.example.test/embed","visibility":"user","forward_access_token":false}]`
	_, err = svc.VerifyEmbeddedCapability(context.Background(), issued.Token, "https://help.example.test")
	require.ErrorIs(t, err, ErrEmbeddedCapabilityInvalid)

	_, err = svc.settingService.ResolveEmbeddedCapabilityTarget(
		context.Background(), "help", "https://help.example.test/path", RoleUser,
	)
	require.True(t, errors.Is(err, ErrEmbeddedCapabilityDenied))
}

func TestEmbeddedCapabilityHonorsAdminMenuVisibility(t *testing.T) {
	settings := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyCustomMenuItems: `[{"id":"ops","url":"https://ops.example.test","visibility":"admin","forward_access_token":true}]`,
	}}, nil)

	_, err := settings.ResolveEmbeddedCapabilityTarget(
		context.Background(), "ops", "https://ops.example.test", RoleUser,
	)
	require.ErrorIs(t, err, ErrEmbeddedCapabilityDenied)

	target, err := settings.ResolveEmbeddedCapabilityTarget(
		context.Background(), "ops", "https://ops.example.test", RoleAdmin,
	)
	require.NoError(t, err)
	require.Equal(t, "https://ops.example.test", target.Origin)
}

func TestEmbeddedCapabilityRequiresTLSOutsideLoopback(t *testing.T) {
	settings := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyCustomMenuItems: `[
			{"id":"remote","url":"http://remote.example.test/embed","visibility":"user","forward_access_token":true},
			{"id":"local","url":"http://127.0.0.1:18086/embed","visibility":"user","forward_access_token":true}
		]`,
	}}, nil)

	_, err := settings.ResolveEmbeddedCapabilityTarget(
		context.Background(), "remote", "http://remote.example.test", RoleUser,
	)
	require.ErrorIs(t, err, ErrEmbeddedCapabilityDenied)

	target, err := settings.ResolveEmbeddedCapabilityTarget(
		context.Background(), "local", "http://127.0.0.1:18086", RoleUser,
	)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:18086", target.Origin)
}
