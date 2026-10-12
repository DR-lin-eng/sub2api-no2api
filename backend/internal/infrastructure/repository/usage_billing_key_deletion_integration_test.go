//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Exercise the real deletion command (including credential tombstoning), not
// just an UPDATE of deleted_at. In-flight billing belongs to the original user
// and subscription even if the same credential has already been recreated.
func TestUsageBillingKeyDeletion_DirectSettlement(t *testing.T) {
	for _, subscriptionBilling := range []bool{false, true} {
		name := "balance"
		if subscriptionBilling {
			name = "subscription"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			cmd, replacementID := newDeletedKeyBillingFixture(t, subscriptionBilling)
			repo := NewUsageBillingRepository(testEntClient(t), integrationDB)
			result, err := repo.Apply(ctx, cmd)
			require.NoError(t, err)
			require.True(t, result.Applied)
			require.False(t, result.APIKeyQuotaExhausted)
			result, err = repo.Apply(ctx, cmd)
			require.NoError(t, err)
			require.False(t, result.Applied, "replaying the original request must not charge twice")
			assertDeletedKeyBillingSettled(t, cmd, replacementID, 1)
		})
	}
}

func TestUsageBillingKeyDeletion_DurableQueueSettlement(t *testing.T) {
	for _, subscriptionBilling := range []bool{false, true} {
		name := "balance"
		if subscriptionBilling {
			name = "subscription"
		}
		t.Run(name, func(t *testing.T) {
			resetDurableBillingQueueTables(t)
			ctx := context.Background()
			cmd, replacementID := newDeletedKeyBillingFixture(t, subscriptionBilling)
			repo := newDurableBillingQueueIntegrationRepo()
			second := *cmd
			second.RequestID = uuid.NewString()
			commands := []*service.UsageBillingCommand{cmd, &second}
			inputs := make([]usageBillingBatchInput, 0, len(commands))
			for _, command := range commands {
				command.Normalize()
				payload, err := json.Marshal(command)
				require.NoError(t, err)
				inputs = append(inputs, usageBillingBatchInput{
					RequestID: command.RequestID, APIKeyID: command.APIKeyID,
					RequestFingerprint: command.RequestFingerprint, Payload: payload,
				})
			}
			payload, err := json.Marshal(inputs)
			require.NoError(t, err)
			statuses, err := repo.insertEnqueueBatch(ctx, payload)
			require.NoError(t, err)
			for _, command := range commands {
				require.Equal(t, usageBillingEnqueueInserted, statuses[usageBillingRequestKey(command.RequestID, command.APIKeyID)].status)
				repo.reconcilePendingOverlay(command)
			}

			// The fast batch path rolls back on a deleted key counter. Its
			// single-job fallback must settle both jobs without losing effects.
			for range 2 {
				processed, err := repo.processUsageBillingCycle(ctx, 0, false)
				require.NoError(t, err)
				require.Equal(t, 1, processed)
			}
			assertDeletedKeyBillingSettled(t, cmd, replacementID, 2)
			var jobs, deadLetters int
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_jobs").Scan(&jobs))
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dead_letters").Scan(&deadLetters))
			require.Zero(t, jobs)
			require.Zero(t, deadLetters, "key deletion must not cause unpaid dead letters")
			require.Zero(t, mustRedisFloat(t, integrationRedis, usageBillingPendingBalanceKey(cmd.UserID)))
			require.Zero(t, mustRedisFloat(t, integrationRedis, usageBillingPendingAPIKeyUsageKey(cmd.APIKeyID)))
			require.Zero(t, mustRedisFloat(t, integrationRedis, usageBillingPendingAPIKeyRateLimitKey(cmd.APIKeyID)))
			if subscriptionBilling {
				require.Zero(t, mustRedisFloat(t, integrationRedis, usageBillingPendingSubscriptionKey(cmd.UserID, cmd.GroupID)))
			}
			statuses, err = repo.insertEnqueueBatch(ctx, payload)
			require.NoError(t, err)
			for _, command := range commands {
				require.Equal(t, usageBillingEnqueueApplied, statuses[usageBillingRequestKey(command.RequestID, command.APIKeyID)].status)
			}
			processed, err := repo.processUsageBillingCycle(ctx, 0, false)
			require.NoError(t, err)
			require.Zero(t, processed)
			assertDeletedKeyBillingSettled(t, cmd, replacementID, 2)
		})
	}
}

func newDeletedKeyBillingFixture(t *testing.T, subscriptionBilling bool) (*service.UsageBillingCommand, int64) {
	t.Helper()
	ctx := context.Background()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{Email: uuid.NewString() + "@billing-audit.invalid", Balance: 100})
	group := mustCreateGroup(t, client, &service.Group{
		Name: "billing-key-deletion-" + uuid.NewString(), Platform: service.PlatformOpenAI,
		SubscriptionType: service.SubscriptionTypeStandard, RateMultiplier: 1,
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "billing-key-deletion-" + uuid.NewString(), Type: service.AccountTypeAPIKey,
		Platform: service.PlatformOpenAI, Extra: map[string]any{"quota_limit": 100.0},
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID, GroupID: &group.ID, Key: "sk-billing-audit-" + uuid.NewString(),
		Quota: 50, RateLimit5h: 50,
	})
	cmd := &service.UsageBillingCommand{
		RequestID: uuid.NewString(), APIKeyID: apiKey.ID, UserID: user.ID, GroupID: group.ID,
		AccountID: account.ID, AccountType: account.Type, Model: "gpt-4o-mini",
		InputTokens: 1000, OutputTokens: 500, BalanceCost: 1.25,
		APIKeyQuotaCost: 1.25, APIKeyRateLimitCost: 1.25, AccountQuotaCost: 1.25,
		QuotaPlatform: service.PlatformOpenAI, UserPlatformQuotaCost: 1.25,
	}
	if subscriptionBilling {
		_, err := integrationDB.ExecContext(ctx, "UPDATE groups SET subscription_type = $1 WHERE id = $2", service.SubscriptionTypeSubscription, group.ID)
		require.NoError(t, err)
		subscription := mustCreateSubscription(t, client, &service.UserSubscription{UserID: user.ID, GroupID: group.ID})
		cmd.SubscriptionID = &subscription.ID
		cmd.SubscriptionCost = cmd.BalanceCost
		cmd.BalanceCost = 0
		cmd.UserPlatformQuotaCost = 0 // Subscription requests are exempt from platform quotas.
	}
	keyRepo := NewAPIKeyRepository(client, integrationDB)
	require.NoError(t, keyRepo.DeleteWithAudit(ctx, apiKey.ID))
	_, err := keyRepo.GetByKey(ctx, apiKey.Key)
	require.ErrorIs(t, err, service.ErrAPIKeyNotFound)
	replacement := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID, GroupID: &group.ID, Key: apiKey.Key, Name: "replacement", Quota: 50, RateLimit5h: 50,
	})
	require.NotEqual(t, apiKey.ID, replacement.ID)
	return cmd, replacement.ID
}

func assertDeletedKeyBillingSettled(t *testing.T, cmd *service.UsageBillingCommand, replacementID int64, requests int) {
	t.Helper()
	ctx := context.Background()
	cost := 1.25 * float64(requests)
	var balance, accountUsage float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", cmd.UserID).Scan(&balance))
	if cmd.SubscriptionID == nil {
		require.InDelta(t, 100-cost, balance, 1e-8)
		var platformUsage float64
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT daily_usage_usd FROM user_platform_quotas WHERE user_id = $1 AND platform = $2", cmd.UserID, cmd.QuotaPlatform).Scan(&platformUsage))
		require.InDelta(t, cost, platformUsage, 1e-8)
	} else {
		require.InDelta(t, 100, balance, 1e-8)
		var daily, weekly, monthly float64
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd FROM user_subscriptions WHERE id = $1", *cmd.SubscriptionID).Scan(&daily, &weekly, &monthly))
		for _, usage := range []float64{daily, weekly, monthly} {
			require.InDelta(t, cost, usage, 1e-8)
		}
	}
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT (extra->>'quota_used')::numeric FROM accounts WHERE id = $1", cmd.AccountID).Scan(&accountUsage))
	require.InDelta(t, cost, accountUsage, 1e-8)
	for _, id := range []int64{cmd.APIKeyID, replacementID} {
		var quota, rateLimit float64
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_used, usage_5h FROM api_keys WHERE id = $1", id).Scan(&quota, &rateLimit))
		require.Zero(t, quota)
		require.Zero(t, rateLimit)
	}
	var deletedAt sql.NullTime
	var storedKey string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT key, deleted_at FROM api_keys WHERE id = $1", cmd.APIKeyID).Scan(&storedKey, &deletedAt))
	require.True(t, deletedAt.Valid)
	require.Contains(t, storedKey, "__deleted__")
	var dedup int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE api_key_id = $1", cmd.APIKeyID).Scan(&dedup))
	require.Equal(t, requests, dedup)
	t.Logf("settled=%d cost=%.2f balance=%.2f subscription=%t account_quota=%.2f dedup=%d", requests, cost, balance, cmd.SubscriptionID != nil, accountUsage, dedup)
}
