package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type adminAPIKeySettingStub struct{ values map[string]string }

func (s *adminAPIKeySettingStub) Get(ctx context.Context, key string) (*Setting, error) {
	value, err := s.GetValue(ctx, key)
	if err != nil {
		return nil, err
	}
	return &Setting{Key: key, Value: value}, nil
}
func (s *adminAPIKeySettingStub) GetValue(_ context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}
func (s *adminAPIKeySettingStub) Set(_ context.Context, key, value string) error {
	s.values[key] = value
	return nil
}
func (s *adminAPIKeySettingStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := map[string]string{}
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}
func (s *adminAPIKeySettingStub) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		s.values[key] = value
	}
	return nil
}
func (s *adminAPIKeySettingStub) GetAll(context.Context) (map[string]string, error) {
	return s.values, nil
}
func (s *adminAPIKeySettingStub) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
}

func TestAdminAPIKeyScopesAndLifecycle(t *testing.T) {
	repo := &adminAPIKeySettingStub{values: map[string]string{}}
	svc := NewSettingService(repo, nil)
	ctx := context.Background()
	expires := time.Now().Add(time.Hour)
	created, secret, err := svc.CreateAdminAPIKey(ctx, AdminAPIKeyCreateInput{
		Name:      "read-only",
		Scopes:    []string{AdminAPIKeyScopeUsersRead, AdminAPIKeyScopeUsersRead},
		ExpiresAt: &expires,
	}, 42)
	require.NoError(t, err)
	require.NotEmpty(t, secret)
	require.Equal(t, []string{AdminAPIKeyScopeUsersRead}, created.Scopes)
	require.NotContains(t, repo.values[adminAPIKeysSetting], secret)

	authenticated, err := svc.AuthenticateAdminAPIKey(ctx, secret)
	require.NoError(t, err)
	require.Equal(t, created.ID, authenticated.ID)

	keys, err := svc.ListAdminAPIKeys(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.NotContains(t, keys[0].KeyPrefix, secret)

	require.NoError(t, svc.RevokeAdminAPIKey(ctx, created.ID))
	_, err = svc.AuthenticateAdminAPIKey(ctx, secret)
	require.Error(t, err)
}

func TestAdminAPIKeyExpiredAndInvalidScopes(t *testing.T) {
	repo := &adminAPIKeySettingStub{values: map[string]string{}}
	svc := NewSettingService(repo, nil)
	ctx := context.Background()
	_, _, err := svc.CreateAdminAPIKey(ctx, AdminAPIKeyCreateInput{Name: "bad", Scopes: []string{"admin.nope"}}, 1)
	require.Error(t, err)

	expires := time.Now().Add(-time.Minute)
	_, secret, err := svc.CreateAdminAPIKey(ctx, AdminAPIKeyCreateInput{Name: "expired", ExpiresAt: &expires}, 1)
	require.Error(t, err)
	require.Empty(t, secret)
}
