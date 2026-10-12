//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUsageBillingRepositoryApply_CodexVoiceMinuteBilling(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)
	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("voice-%d@example.com", time.Now().UnixNano()), PasswordHash: "hash", Balance: 1})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-voice-" + uuid.NewString(), Name: "voice", Quota: 1})
	account := mustCreateAccount(t, client, &service.Account{Name: "voice-" + uuid.NewString(), Type: service.AccountTypeOAuth, Platform: service.PlatformOpenAI})
	cmd := &service.UsageBillingCommand{
		RequestID: "voice-" + uuid.NewString(), APIKeyID: key.ID, UserID: user.ID, AccountID: account.ID,
		AccountType: service.AccountTypeOAuth, Model: service.CodexVoiceModel,
		BalanceCost: .05, APIKeyQuotaCost: .05, APIKeyRateLimitCost: .05,
	}
	first, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.True(t, first.Applied)
	retry, err := repo.Apply(ctx, cmd)
	require.NoError(t, err)
	require.False(t, retry.Applied)
	var balance, quota, rate float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id=$1", user.ID).Scan(&balance))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_used, usage_5h FROM api_keys WHERE id=$1", key.ID).Scan(&quota, &rate))
	require.Equal(t, .95, balance)
	require.Equal(t, .05, quota)
	require.Equal(t, .05, rate)
}
