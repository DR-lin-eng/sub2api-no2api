package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImageCountBillingChannelContextTier(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		gid := int64(906)
		model := "gpt-image-1"
		resolver := newOpenAITokenImageChannelPricingResolverForTest(t, gid, model)
		cache, ok := resolver.channelService.cache.Load().(*channelCache)
		require.True(t, ok)
		limit, cheap, expensive := 1000, .1, .3
		cache.pricingByGroupModel[channelModelKey{groupID: gid, model: model}] = &ChannelModelPricing{
			BillingMode: BillingModePerRequest,
			Intervals:   []PricingInterval{{MinTokens: 0, MaxTokens: &limit, PerRequestPrice: &cheap}, {MinTokens: 1000, PerRequestPrice: &expensive}},
		}
		key := &APIKey{ID: 6, GroupID: &gid, Group: &Group{ID: gid, Platform: PlatformOpenAI, Hydrated: true, AllowImageGeneration: true, ImageCountBilling: enabled}}
		generic := &GatewayService{billingService: resolver.billingService, resolver: resolver}
		cost := generic.calculateRecordUsageCost(context.Background(), &ForwardResult{
			Model: model, ImageCount: 2, ImageEndpoint: true, ImageSize: "1K", Usage: ClaudeUsage{InputTokens: 5000, OutputTokens: 100},
		}, key, model, 1, 1, nil)
		require.InDelta(t, .6, cost.TotalCost, 1e-12)
		openai := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}
		resolved, err := openai.calculateOpenAIRecordUsageCostWithResolution(context.Background(), &OpenAIForwardResult{
			Model: model, ImageCount: 2, ImageEndpoint: true, ImageSize: "1K", Usage: OpenAIUsage{InputTokens: 5000, OutputTokens: 100},
		}, key, []string{model}, 1, 1, 1, 1, UsageTokens{InputTokens: 5000, OutputTokens: 100}, "", false, false)
		require.NoError(t, err)
		require.InDelta(t, .6, resolved.Cost.TotalCost, 1e-12)
	}
}

func TestImageCountBillingFallsBackFromTokenPricing(t *testing.T) {
	gid := int64(907)
	model := "gpt-image-1"
	resolver := newOpenAITokenImageChannelPricingResolverForTest(t, gid, model)
	key := &APIKey{ID: 7, GroupID: &gid, Group: &Group{ID: gid, Platform: PlatformOpenAI, Hydrated: true, AllowImageGeneration: true, ImageCountBilling: true}}
	svc := &GatewayService{billingService: resolver.billingService, resolver: resolver}
	cost := svc.calculateRecordUsageCost(context.Background(), &ForwardResult{Model: model, ImageCount: 2, ImageEndpoint: true, ImageSize: "1K"}, key, model, 1, 1, nil)
	want := resolver.billingService.CalculateImageCost(model, "1K", 2, nil, 1)
	require.Positive(t, want.TotalCost)
	require.InDelta(t, want.TotalCost, cost.TotalCost, 1e-12)
}

func TestImageCountBillingGroupCloneAndCache(t *testing.T) {
	group := &Group{ID: 908, Platform: PlatformOpenAI, Hydrated: true, AllowImageGeneration: true, ImageCountBilling: true}
	require.True(t, cloneGroupForDuplicate(group, "fixture-operation").ImageCountBilling)
	svc := &APIKeyService{}
	snapshot := svc.snapshotFromAPIKey(context.Background(), &APIKey{ID: 8, User: &User{ID: 8}, GroupID: &group.ID, Group: group})
	require.NotNil(t, snapshot.Group)
	require.True(t, snapshot.Group.ImageCountBilling)
	restored := svc.snapshotToAPIKey("fixture-key", snapshot)
	require.True(t, restored.Group.ImageCountBilling)
	require.Greater(t, snapshot.Version, 25)
}
