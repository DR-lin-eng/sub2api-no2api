package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type aiStudioSchedulingAccountRepo struct {
	AccountRepository
	accounts   []Account
	byGroup    map[int64][]Account
	groupCalls *[]int64
}

func (r aiStudioSchedulingAccountRepo) ListSchedulableByPlatforms(context.Context, []string) ([]Account, error) {
	return append([]Account(nil), r.accounts...), nil
}

func (r aiStudioSchedulingAccountRepo) ListSchedulableByGroupIDAndPlatforms(_ context.Context, groupID int64, _ []string) ([]Account, error) {
	if r.groupCalls != nil {
		*r.groupCalls = append(*r.groupCalls, groupID)
	}
	return append([]Account(nil), r.byGroup[groupID]...), nil
}

func TestSelectAccountForAIStudioEndpointsWithExclusions(t *testing.T) {
	accounts := []Account{
		{
			ID:          1,
			Type:        AccountTypeAPIKey,
			Priority:    0,
			Credentials: map[string]any{"api_key": "first"},
		},
		{
			ID:          2,
			Type:        AccountTypeAPIKey,
			Priority:    1,
			Credentials: map[string]any{"api_key": "second"},
		},
	}
	svc := &GeminiMessagesCompatService{
		accountRepo: aiStudioSchedulingAccountRepo{accounts: accounts},
		cfg:         &config.Config{RunMode: config.RunModeSimple},
	}

	selected, err := svc.SelectAccountForAIStudioEndpointsWithExclusions(
		context.Background(),
		nil,
		map[int64]struct{}{1: {}},
	)
	require.NoError(t, err)
	require.Equal(t, int64(2), selected.ID)
}

func TestSelectAccountForAIStudioEndpointsFallsBackAcrossAPIKeyGroups(t *testing.T) {
	firstID := int64(31)
	secondID := int64(32)
	first := &Group{ID: firstID, Platform: PlatformGemini, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, Hydrated: true}
	second := &Group{ID: secondID, Platform: PlatformGemini, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, Hydrated: true}
	groupID := firstID
	apiKey := &APIKey{
		GroupID: &groupID,
		Group:   first,
		GroupBindings: []APIKeyGroupBinding{
			{APIKeyGroupBinding: domain.APIKeyGroupBinding{GroupID: firstID}, Group: first, EffectiveRateMultiplier: 1},
			{APIKeyGroupBinding: domain.APIKeyGroupBinding{GroupID: secondID}, Group: second, EffectiveRateMultiplier: 1},
		},
	}
	var calls []int64
	svc := &GeminiMessagesCompatService{
		accountRepo: aiStudioSchedulingAccountRepo{
			byGroup: map[int64][]Account{
				secondID: {{ID: 2, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "second"}}},
			},
			groupCalls: &calls,
		},
		cfg: &config.Config{},
	}

	selected, err := svc.SelectAccountForAIStudioEndpoints(WithAPIKeyGroupRouting(context.Background(), apiKey), apiKey.GroupID)

	require.NoError(t, err)
	require.Equal(t, int64(2), selected.ID)
	require.Equal(t, []int64{firstID, secondID}, calls)
	require.Equal(t, secondID, *apiKey.GroupID)
	require.Same(t, second, apiKey.Group)
}
