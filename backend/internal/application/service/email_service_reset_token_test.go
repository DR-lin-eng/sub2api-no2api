//go:build unit

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type resetTokenCacheStub struct {
	EmailCache
	data          *PasswordResetTokenData
	consumedToken string
	consumedHash  string
	consumed      bool
}

func (s *resetTokenCacheStub) GetPasswordResetToken(context.Context, string) (*PasswordResetTokenData, error) {
	return s.data, nil
}

func (s *resetTokenCacheStub) SetPasswordResetToken(_ context.Context, _ string, data *PasswordResetTokenData, _ time.Duration) error {
	s.data = data
	return nil
}
func (s *resetTokenCacheStub) ConsumePasswordResetToken(_ context.Context, _ string, token, hash string) (bool, error) {
	s.consumedToken, s.consumedHash = token, hash
	if s.consumed || s.data == nil || (s.data.Token != token && s.data.Token != hash) {
		return false, nil
	}
	s.consumed = true
	return true, nil
}
func TestEmailServiceResetTokenUpgradeCompatibility(t *testing.T) {
	token := strings.Repeat("a", 64)
	for _, stored := range []string{token, hashPasswordResetToken(token)} {
		cache := &resetTokenCacheStub{data: &PasswordResetTokenData{Token: stored}}
		svc := NewEmailService(nil, cache)
		require.NoError(t, svc.VerifyPasswordResetToken(context.Background(), "user@example.com", token))
		require.ErrorIs(t, svc.VerifyPasswordResetToken(context.Background(), "user@example.com", strings.Repeat("b", 64)), ErrInvalidResetToken)
		require.ErrorIs(t, svc.VerifyPasswordResetToken(context.Background(), "user@example.com", hashPasswordResetToken(token)), ErrInvalidResetToken)
		require.NoError(t, svc.ConsumePasswordResetToken(context.Background(), "user@example.com", token))
		require.Equal(t, token, cache.consumedToken)
		require.Equal(t, hashPasswordResetToken(token), cache.consumedHash)
		require.ErrorIs(t, svc.ConsumePasswordResetToken(context.Background(), "user@example.com", token), ErrInvalidResetToken)
	}
}

func TestEmailServiceResetTokenHashWriteRolloutGate(t *testing.T) {
	ctx := context.Background()
	cache := &resetTokenCacheStub{}
	settings := &settingRepoStub{values: map[string]string{}}
	svc := NewEmailService(settings, cache)
	legacy, err := svc.preparePasswordResetToken(ctx, "user@example.com")
	require.NoError(t, err)
	require.Len(t, legacy, 64)
	require.Equal(t, legacy, cache.data.Token, "default storage remains readable by old nodes")
	repeated, err := svc.preparePasswordResetToken(ctx, "user@example.com")
	require.NoError(t, err)
	require.Equal(t, legacy, repeated, "default resend preserves the original link")
	settings.values[passwordResetTokenHashStorageSettingKey] = "true"
	hashed, err := svc.preparePasswordResetToken(ctx, "user@example.com")
	require.NoError(t, err)
	require.NotEqual(t, legacy, hashed)
	require.Equal(t, hashPasswordResetToken(hashed), cache.data.Token)
	require.NotContains(t, cache.data.Token, hashed)
	require.NoError(t, svc.VerifyPasswordResetToken(ctx, "user@example.com", hashed))
	settings.values[passwordResetTokenHashStorageSettingKey] = "false"
	newLegacy, err := svc.preparePasswordResetToken(ctx, "user@example.com")
	require.NoError(t, err)
	require.NotEqual(t, hashed, newLegacy, "a digest must never be re-sent as a plaintext link")
	require.Equal(t, newLegacy, cache.data.Token)
}
