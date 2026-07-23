//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"

	"github.com/stretchr/testify/require"
)

func TestSelectBillableModelPricingCompositeAliasUsesConcreteModel(t *testing.T) {
	svc := &GatewayService{billingService: NewBillingService(&config.Config{}, nil)}
	apiKey := &APIKey{Group: &Group{Platform: PlatformComposite}}
	ctx := context.Background()

	model, resolved := svc.selectBillableModelPricing(ctx, apiKey, "all/claude", "claude-opus-4-7")
	require.Equal(t, "claude-opus-4-7", model)
	require.True(t, billingPricingResolvable(resolved))

	model, resolved = svc.selectBillableModelPricing(ctx, apiKey, "team/best", "claude-sonnet-4")
	require.Equal(t, "claude-sonnet-4", model)
	require.True(t, billingPricingResolvable(resolved))
}

func TestSelectBillableModelPricingGeneralFallback(t *testing.T) {
	svc := &GatewayService{billingService: NewBillingService(&config.Config{}, nil)}
	apiKey := &APIKey{}
	ctx := context.Background()

	model, resolved := svc.selectBillableModelPricing(ctx, apiKey, "team/best", "", "claude-sonnet-4")
	require.Equal(t, "claude-sonnet-4", model)
	require.True(t, billingPricingResolvable(resolved))

	model, resolved = svc.selectBillableModelPricing(ctx, apiKey, "claude-sonnet-4", "", "claude-opus-4")
	require.Equal(t, "claude-sonnet-4", model)
	require.True(t, billingPricingResolvable(resolved))

	model, resolved = svc.selectBillableModelPricing(ctx, apiKey, "team/best", "", "another/alias", "")
	require.Equal(t, "team/best", model)
	require.False(t, billingPricingResolvable(resolved))

	model, resolved = svc.selectBillableModelPricing(ctx, apiKey, "", "", "claude-sonnet-4")
	require.Equal(t, "claude-sonnet-4", model)
	require.True(t, billingPricingResolvable(resolved))
}

func TestSelectBillableModelPricingKeepsExplicitCompositeChannelAlias(t *testing.T) {
	groupID := int64(10)
	price := 0.007
	cache := newEmptyChannelCache()
	cache.pricingByGroupModel[channelModelKey{groupID: groupID, platform: PlatformAnthropic, model: "team/best"}] = &ChannelModelPricing{
		BillingMode:     BillingModePerRequest,
		PerRequestPrice: &price,
	}
	cache.channelByGroupID[groupID] = &Channel{ID: 1, Status: StatusActive}
	cache.groupPlatform[groupID] = PlatformComposite
	cache.loadedAt = time.Now()
	channelService := &ChannelService{}
	channelService.cache.Store(cache)
	billingService := NewBillingService(&config.Config{}, nil)
	svc := &GatewayService{
		billingService: billingService,
		channelService: channelService,
		resolver:       NewModelPricingResolver(channelService, billingService),
	}
	ctx := WithResolvedTargetPlatform(context.Background(), PlatformAnthropic)
	apiKey := &APIKey{Group: &Group{ID: groupID, Platform: PlatformComposite}}

	model, resolved := svc.selectBillableModelPricing(ctx, apiKey, "team/best", "claude-opus-4-7")
	require.Equal(t, "team/best", model)
	require.NotNil(t, resolved)
	require.Equal(t, PricingSourceChannel, resolved.Source)
}
