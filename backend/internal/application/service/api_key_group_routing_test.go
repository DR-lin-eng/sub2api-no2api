package service

import (
	"context"
	"math"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
	"github.com/stretchr/testify/require"
)

type apiKeyGroupRoutingAccountRepo struct {
	AccountRepository
	byGroup    map[int64][]Account
	groupCalls []int64
}

type apiKeyGroupRoutingGroupRepo struct {
	GroupRepository
	groups map[int64]*Group
}

func (r apiKeyGroupRoutingGroupRepo) GetByID(_ context.Context, id int64) (*Group, error) {
	group, ok := r.groups[id]
	if !ok {
		return nil, ErrGroupNotFound
	}
	return group, nil
}

func (r *apiKeyGroupRoutingAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, groupID int64, platform string) ([]Account, error) {
	r.groupCalls = append(r.groupCalls, groupID)
	accounts := r.byGroup[groupID]
	out := make([]Account, 0, len(accounts))
	for i := range accounts {
		if accounts[i].Platform == platform {
			out = append(out, accounts[i])
		}
	}
	return out, nil
}

func (r *apiKeyGroupRoutingAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]Account, error) {
	var out []Account
	for _, accounts := range r.byGroup {
		for i := range accounts {
			if accounts[i].Platform == platform {
				out = append(out, accounts[i])
			}
		}
	}
	return out, nil
}

func (r *apiKeyGroupRoutingAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}

func TestNormalizeAPIKeyGroupBindingInputs(t *testing.T) {
	primaryID := int64(10)
	ceiling := 1.25
	inputs := []APIKeyGroupBindingInput{
		{GroupID: primaryID, MaxRateMultiplier: &ceiling},
		{GroupID: 20},
	}

	bindings, err := normalizeAPIKeyGroupBindingInputs(&primaryID, &inputs)
	require.NoError(t, err)
	require.Equal(t, []int64{10, 20}, []int64{bindings[0].GroupID, bindings[1].GroupID})
	require.Equal(t, 1.25, *bindings[0].MaxRateMultiplier)
	require.Nil(t, bindings[1].MaxRateMultiplier)
}

func TestNormalizeAPIKeyGroupBindingInputsRejectsInvalidBindings(t *testing.T) {
	tests := []struct {
		name    string
		groupID *int64
		inputs  []APIKeyGroupBindingInput
	}{
		{name: "duplicate", inputs: []APIKeyGroupBindingInput{{GroupID: 1}, {GroupID: 1}}},
		{name: "non-positive group", inputs: []APIKeyGroupBindingInput{{GroupID: 0}}},
		{name: "zero ceiling", inputs: []APIKeyGroupBindingInput{{GroupID: 1, MaxRateMultiplier: routingTestFloat64Pointer(0)}}},
		{name: "nan ceiling", inputs: []APIKeyGroupBindingInput{{GroupID: 1, MaxRateMultiplier: routingTestFloat64Pointer(math.NaN())}}},
		{name: "infinite ceiling", inputs: []APIKeyGroupBindingInput{{GroupID: 1, MaxRateMultiplier: routingTestFloat64Pointer(math.Inf(1))}}},
		{name: "primary mismatch", groupID: routingTestInt64Pointer(2), inputs: []APIKeyGroupBindingInput{{GroupID: 1}}},
		{name: "too many", inputs: makeAPIKeyGroupBindingInputs(maxAPIKeyGroupBindings + 1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeAPIKeyGroupBindingInputs(tt.groupID, &tt.inputs)
			require.ErrorIs(t, err, ErrAPIKeyGroupBindingsInvalid)
		})
	}
}

func TestValidateAPIKeyGroupBindingsRejectsMixedPlatforms(t *testing.T) {
	openAIGroup := activeRoutingGroup(1)
	geminiGroup := activeRoutingGroup(2)
	geminiGroup.Platform = PlatformGemini
	svc := &APIKeyService{groupRepo: apiKeyGroupRoutingGroupRepo{groups: map[int64]*Group{
		openAIGroup.ID: openAIGroup,
		geminiGroup.ID: geminiGroup,
	}}}

	_, err := svc.validateAndHydrateAPIKeyGroupBindings(context.Background(), &User{ID: 7}, []APIKeyGroupBinding{
		{APIKeyGroupBinding: domain.APIKeyGroupBinding{GroupID: openAIGroup.ID}},
		{APIKeyGroupBinding: domain.APIKeyGroupBinding{GroupID: geminiGroup.ID}},
	})

	require.ErrorIs(t, err, ErrAPIKeyGroupBindingsInvalid)
}

func TestAPIKeyCloneForRequestDoesNotAliasRoutingFields(t *testing.T) {
	groupID := int64(1)
	ceiling := 2.0
	original := &APIKey{
		GroupID: &groupID,
		GroupBindings: []APIKeyGroupBinding{{APIKeyGroupBinding: domain.APIKeyGroupBinding{
			GroupID:           groupID,
			MaxRateMultiplier: &ceiling,
		}}},
	}

	clone := original.CloneForRequest()
	*clone.GroupID = 2
	clone.GroupBindings[0].GroupID = 3
	*clone.GroupBindings[0].MaxRateMultiplier = 4

	require.Equal(t, int64(1), *original.GroupID)
	require.Equal(t, int64(1), original.GroupBindings[0].GroupID)
	require.Equal(t, 2.0, *original.GroupBindings[0].MaxRateMultiplier)
}

func TestAPIKeyGroupRoutingSkipsProtectedGroupAndTracksSelectedBillingGroup(t *testing.T) {
	first := activeRoutingGroup(1)
	first.RateMultiplier = 2
	second := activeRoutingGroup(2)
	ceiling := 1.5
	apiKey := routingAPIKey(first, second, &ceiling)

	ctx := WithAPIKeyGroupRouting(context.Background(), apiKey)
	ctx = context.WithValue(ctx, ctxkey.Group, apiKey.Group)
	ctx, _ = WithGatewayTokenRequestPricing(ctx)

	require.Equal(t, int64(2), *apiKey.GroupID)
	require.Same(t, second, tokenRequestBillingGroupFromContext(ctx))
	candidates, routed := apiKeyGroupRoutingCandidates(ctx, apiKey.GroupID)
	require.True(t, routed)
	require.Equal(t, []int64{2}, bindingGroupIDs(candidates))
}

func TestGatewayServiceFallsBackAcrossAPIKeyGroupsInOrder(t *testing.T) {
	first := activeRoutingGroup(11)
	second := activeRoutingGroup(12)
	apiKey := routingAPIKey(first, second, nil)
	groupIDAlias := apiKey.GroupID
	repo := &apiKeyGroupRoutingAccountRepo{byGroup: map[int64][]Account{
		second.ID: {routingAccount(1201, second.ID)},
	}}
	svc := &GatewayService{
		accountRepo:        repo,
		cfg:                &config.Config{},
		concurrencyService: NewConcurrencyService(nil),
	}

	ctx := WithAPIKeyGroupRouting(context.Background(), apiKey)
	account, err := svc.SelectAccountForModel(ctx, apiKey.GroupID, "", "")

	require.NoError(t, err)
	require.NotNil(t, account)
	require.Equal(t, int64(1201), account.ID)
	require.Equal(t, []int64{first.ID, second.ID}, uniqueConsecutiveGroupIDs(repo.groupCalls))
	require.Equal(t, second.ID, *apiKey.GroupID)
	require.Equal(t, second.ID, *groupIDAlias)
	require.Same(t, second, apiKey.Group)
}

func TestOpenAIGatewayServiceFallsBackAcrossAPIKeyGroupsInOrder(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)

	first := activeRoutingGroup(21)
	second := activeRoutingGroup(22)
	apiKey := routingAPIKey(first, second, nil)
	repo := &apiKeyGroupRoutingAccountRepo{byGroup: map[int64][]Account{
		second.ID: {routingAccount(2201, second.ID)},
	}}
	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              &schedulerTestGatewayCache{},
		cfg:                newSchedulerTestSubscriptionPriorityConfig(),
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	ctx := WithAPIKeyGroupRouting(context.Background(), apiKey)
	selection, _, err := svc.SelectAccountWithScheduler(
		ctx,
		apiKey.GroupID,
		"",
		"group-routing-test",
		"gpt-5.1",
		nil,
		OpenAIUpstreamTransportAny,
		false,
	)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(2201), selection.Account.ID)
	require.Equal(t, []int64{first.ID, second.ID}, uniqueConsecutiveGroupIDs(repo.groupCalls))
	require.Equal(t, second.ID, *apiKey.GroupID)
	require.Same(t, second, apiKey.Group)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func activeRoutingGroup(id int64) *Group {
	return &Group{
		ID:               id,
		Name:             "group",
		Platform:         PlatformOpenAI,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeStandard,
		RateMultiplier:   1,
		Hydrated:         true,
	}
}

func routingAPIKey(first, second *Group, firstCeiling *float64) *APIKey {
	groupID := first.ID
	return &APIKey{
		GroupID: &groupID,
		Group:   first,
		GroupBindings: []APIKeyGroupBinding{
			{
				APIKeyGroupBinding:      domain.APIKeyGroupBinding{GroupID: first.ID, MaxRateMultiplier: firstCeiling},
				Group:                   first,
				EffectiveRateMultiplier: first.RateMultiplier,
			},
			{
				APIKeyGroupBinding:      domain.APIKeyGroupBinding{GroupID: second.ID},
				Group:                   second,
				EffectiveRateMultiplier: second.RateMultiplier,
			},
		},
	}
}

func routingAccount(id, groupID int64) Account {
	return Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		GroupIDs:    []int64{groupID},
	}
}

func bindingGroupIDs(bindings []APIKeyGroupBinding) []int64 {
	out := make([]int64, 0, len(bindings))
	for i := range bindings {
		out = append(out, bindings[i].GroupID)
	}
	return out
}

func makeAPIKeyGroupBindingInputs(count int) []APIKeyGroupBindingInput {
	out := make([]APIKeyGroupBindingInput, 0, count)
	for i := 1; i <= count; i++ {
		out = append(out, APIKeyGroupBindingInput{GroupID: int64(i)})
	}
	return out
}

func uniqueConsecutiveGroupIDs(values []int64) []int64 {
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func routingTestInt64Pointer(value int64) *int64       { return &value }
func routingTestFloat64Pointer(value float64) *float64 { return &value }
