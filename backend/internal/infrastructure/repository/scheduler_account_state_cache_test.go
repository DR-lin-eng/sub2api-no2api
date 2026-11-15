//go:build unit

package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestSchedulerCacheAccountSchedulingStateUsesMetadataProjection(t *testing.T) {
	ctx := context.Background()
	cache := newSchedulerCacheUnit(t)
	future := time.Now().Add(time.Hour)
	active := service.Account{
		ID:          9301,
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"unused_secret": strings.Repeat("s", 8192)},
		Extra:       map[string]any{"unused_large_field": strings.Repeat("x", 8192)},
	}
	require.NoError(t, cache.SetAccount(ctx, &active))

	state, found, err := cache.GetCachedAccountSchedulingState(ctx, active.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, service.AccountSchedulingState{Exists: true, Schedulable: true}, state)

	active.RateLimitResetAt = &future
	require.NoError(t, cache.SetAccount(ctx, &active))
	state, found, err = cache.GetCachedAccountSchedulingState(ctx, active.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, state.Exists)
	require.False(t, state.Schedulable)

	active.Type = service.AccountTypeOAuth
	active.Extra = map[string]any{service.CodexPrewarmContinuationExtraKey: true}
	require.NoError(t, cache.SetAccount(ctx, &active))
	state, found, err = cache.GetCachedAccountSchedulingState(ctx, active.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, state.Schedulable, "opted OpenAI OAuth accounts bypass cached account-level 429 windows")

	active.RateLimitResetAt = nil
	active.TempUnschedulableUntil = &future
	active.TempUnschedulableReason = `{"status_code":429,"error_message":"limited"}`
	require.NoError(t, cache.SetAccount(ctx, &active))
	state, found, err = cache.GetCachedAccountSchedulingState(ctx, active.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, state.Schedulable, "cached 429 reason must remain available to the scheduler projection")

	active.TempUnschedulableReason = `{"status_code":401,"error_message":"unauthorized"}`
	require.NoError(t, cache.SetAccount(ctx, &active))
	state, found, err = cache.GetCachedAccountSchedulingState(ctx, active.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.False(t, state.Schedulable, "non-429 temporary blocks remain effective")

	active.Type = service.AccountTypeAPIKey
	active.TempUnschedulableUntil = nil
	active.TempUnschedulableReason = ""
	active.RateLimitResetAt = nil
	active.Extra = map[string]any{"quota_limit": 10.0, "quota_used": 10.0}
	require.NoError(t, cache.SetAccount(ctx, &active))
	state, found, err = cache.GetCachedAccountSchedulingState(ctx, active.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.False(t, state.Schedulable)

	active.Extra = nil
	active.Schedulable = false
	require.NoError(t, cache.SetAccount(ctx, &active))
	state, found, err = cache.GetCachedAccountSchedulingState(ctx, active.ID)
	require.NoError(t, err)
	require.True(t, found, "disabled accounts remain cached for queued-request revalidation")
	require.False(t, state.Schedulable)

	active.Schedulable = true
	require.NoError(t, cache.SetAccount(ctx, &active))
	state, found, err = cache.GetCachedAccountSchedulingState(ctx, active.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, state.Schedulable)

	require.NoError(t, cache.DeleteAccount(ctx, active.ID))
	state, found, err = cache.GetCachedAccountSchedulingState(ctx, active.ID)
	require.NoError(t, err)
	require.False(t, found)
	require.Equal(t, service.AccountSchedulingState{}, state)
}
