package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestLiveLeaseReplacesRegularSlotsAndCountsTowardLimits(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	regular := NewConcurrencyCache(client, 15, 900)
	live, ok := regular.(service.LiveConcurrencyCache)
	require.True(t, ok)
	apiKeys, ok := regular.(service.APIKeyConcurrencyCache)
	require.True(t, ok)
	priority, ok := regular.(service.PriorityAdmissionCache)
	require.True(t, ok)
	ctx := context.Background()

	accountAcquired, err := regular.AcquireAccountSlot(ctx, 10, 1, "regular-account")
	require.NoError(t, err)
	require.True(t, accountAcquired)
	userAcquired, err := regular.AcquireUserSlot(ctx, 20, 1, "regular-user")
	require.NoError(t, err)
	require.True(t, userAcquired)
	apiKeyAcquired, err := apiKeys.AcquireAPIKeySlot(ctx, 30, 1, "regular-api-key")
	require.NoError(t, err)
	require.True(t, apiKeyAcquired)

	acquired, err := live.AcquireLiveLease(ctx, 10, 1, 20, 1, 30, 1, "live-lease", service.LiveConcurrencyReplacements{
		Account: true,
		User:    true,
		APIKey:  true,
	})
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, regular.ReleaseAccountSlot(ctx, 10, "regular-account"))
	require.NoError(t, regular.ReleaseUserSlot(ctx, 20, "regular-user"))
	require.NoError(t, apiKeys.ReleaseAPIKeySlot(ctx, 30, "regular-api-key"))

	accountCount, err := regular.GetAccountConcurrency(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, accountCount)
	userCount, err := regular.GetUserConcurrency(ctx, 20)
	require.NoError(t, err)
	require.Equal(t, 1, userCount)
	apiKeyCounts, err := apiKeys.GetAPIKeyConcurrencyBatch(ctx, []int64{30})
	require.NoError(t, err)
	require.Equal(t, 1, apiKeyCounts[30])
	accountAcquired, err = regular.AcquireAccountSlot(ctx, 10, 1, "ordinary-blocked")
	require.NoError(t, err)
	require.False(t, accountAcquired)
	userAcquired, err = regular.AcquireUserSlot(ctx, 20, 1, "ordinary-user-blocked")
	require.NoError(t, err)
	require.False(t, userAcquired)
	apiKeyAcquired, err = apiKeys.AcquireAPIKeySlot(ctx, 30, 1, "ordinary-api-key-blocked")
	require.NoError(t, err)
	require.False(t, apiKeyAcquired)

	accountStatus, err := priority.AcquirePriorityAccountSlot(ctx, service.PriorityAccountAdmissionRequest{
		AccountID:      10,
		MaxConcurrency: 1,
		Tier:           service.RequestSchedulingTierNormal,
		RequestID:      "priority-account-blocked",
	})
	require.NoError(t, err)
	require.Equal(t, service.PriorityAccountAdmissionRejected, accountStatus)
	userStatus, err := priority.AcquirePriorityUserSlot(ctx, service.PriorityUserAdmissionRequest{
		UserID:         20,
		MaxConcurrency: 1,
		Tier:           service.RequestSchedulingTierNormal,
		RequestID:      "priority-user-blocked",
	})
	require.NoError(t, err)
	require.Equal(t, service.PriorityAccountAdmissionRejected, userStatus)

	refreshed, err := live.RefreshLiveLease(ctx, 10, 20, 30, "live-lease")
	require.NoError(t, err)
	require.True(t, refreshed)
	require.NoError(t, live.ReleaseLiveLease(ctx, 10, 20, 30, "live-lease"))
	accountAcquired, err = regular.AcquireAccountSlot(ctx, 10, 1, "ordinary-allowed")
	require.NoError(t, err)
	require.True(t, accountAcquired)
}

func TestLiveLeaseExpiresWithoutRefresh(t *testing.T) {
	redisServer := miniredis.RunT(t)
	baseTime := time.Unix(1_800_000_000, 0).UTC()
	redisServer.SetTime(baseTime)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	regular := NewConcurrencyCache(client, 15, 900)
	live, ok := regular.(service.LiveConcurrencyCache)
	require.True(t, ok)
	apiKeys, ok := regular.(service.APIKeyConcurrencyCache)
	require.True(t, ok)
	ctx := context.Background()

	ordinaryAccount, err := regular.AcquireAccountSlot(ctx, 10, 2, "ordinary-account")
	require.NoError(t, err)
	require.True(t, ordinaryAccount)
	ordinaryUser, err := regular.AcquireUserSlot(ctx, 20, 2, "ordinary-user")
	require.NoError(t, err)
	require.True(t, ordinaryUser)
	ordinaryAPIKey, err := apiKeys.AcquireAPIKeySlot(ctx, 30, 2, "ordinary-api-key")
	require.NoError(t, err)
	require.True(t, ordinaryAPIKey)

	acquired, err := live.AcquireLiveLease(ctx, 10, 2, 20, 2, 30, 2, "expired-live", service.LiveConcurrencyReplacements{})
	require.NoError(t, err)
	require.True(t, acquired)

	redisServer.SetTime(baseTime.Add(61 * time.Second))
	accountCount, err := regular.GetAccountConcurrency(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, accountCount, "the ordinary account member must outlive the Live lease")
	userCount, err := regular.GetUserConcurrency(ctx, 20)
	require.NoError(t, err)
	require.Equal(t, 1, userCount, "the ordinary user member must outlive the Live lease")
	apiKeyCounts, err := apiKeys.GetAPIKeyConcurrencyBatch(ctx, []int64{30})
	require.NoError(t, err)
	require.Equal(t, 1, apiKeyCounts[30], "the ordinary API-key member must outlive the Live lease")
	refreshed, err := live.RefreshLiveLease(ctx, 10, 20, 30, "expired-live")
	require.NoError(t, err)
	require.False(t, refreshed)
}

func TestLimitedAPIKeySlotUsesOrdinarySingleKey(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	cache := NewConcurrencyCache(client, 15, 900)
	apiKeys, ok := cache.(service.APIKeyConcurrencyCache)
	require.True(t, ok)
	ctx := context.Background()

	first, err := apiKeys.AcquireAPIKeySlot(ctx, 77, 1, "first")
	require.NoError(t, err)
	require.True(t, first)
	second, err := apiKeys.AcquireAPIKeySlot(ctx, 77, 1, "second")
	require.NoError(t, err)
	require.False(t, second)
	require.NoError(t, apiKeys.ReleaseAPIKeySlot(ctx, 77, "first"))
	second, err = apiKeys.AcquireAPIKeySlot(ctx, 77, 1, "second")
	require.NoError(t, err)
	require.True(t, second)
}
