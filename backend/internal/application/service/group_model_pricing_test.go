//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func testGroupPricing(input, output float64, models ...string) ChannelModelPricing {
	return ChannelModelPricing{
		Platform:    PlatformAnthropic,
		Models:      models,
		BillingMode: BillingModeToken,
		InputPrice:  &input,
		OutputPrice: &output,
	}
}

func TestGroupModelPricingIndexExactAndWildcard(t *testing.T) {
	exact := testGroupPricing(7e-6, 21e-6, "claude-sonnet-4.5")
	wildcard := testGroupPricing(5e-6, 15e-6, "claude-*")
	group := &Group{ModelPricing: []ChannelModelPricing{wildcard, exact}}
	group.compileModelPricingIndex()

	require.Same(t, &group.ModelPricing[1], group.modelPricingFor("CLAUDE-SONNET-4-5"))
	require.Same(t, &group.ModelPricing[0], group.modelPricingFor("claude-haiku-4"))
	require.Nil(t, group.modelPricingFor("gpt-5.4"))
}

func TestResolveGroupPricingOverridesChannelBaseAndKeepsHigherTier(t *testing.T) {
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:    PlatformAnthropic,
		Models:      []string{"claude-sonnet-4"},
		BillingMode: BillingModeToken,
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: testPtrInt(100000), InputPrice: testPtrFloat64(2e-6), OutputPrice: testPtrFloat64(8e-6)},
			{MinTokens: 100000, InputPrice: testPtrFloat64(4e-6), OutputPrice: testPtrFloat64(16e-6)},
		},
	}})
	group := &Group{ID: 100, LongContextPricingEnabled: true, ModelPricing: []ChannelModelPricing{
		testGroupPricing(9e-6, 27e-6, "claude-sonnet-4"),
	}}
	group.compileModelPricingIndex()

	resolved := r.Resolve(context.Background(), PricingInput{Model: "claude-sonnet-4", GroupID: groupIDPtr(), Group: group})
	require.Equal(t, PricingSourceGroup, resolved.Source)
	require.InDelta(t, 9e-6, resolved.BasePricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 27e-6, resolved.BasePricing.OutputPricePerToken, 1e-12)
	require.Len(t, resolved.Intervals, 1)
	require.Equal(t, 100000, resolved.Intervals[0].MinTokens)

	higher := r.GetIntervalPricing(resolved, 100001)
	require.InDelta(t, 4e-6, higher.InputPricePerToken, 1e-12)
	require.InDelta(t, 16e-6, higher.OutputPricePerToken, 1e-12)
}

func TestResolveGroupPricingDisabledLongContextFlattensChannelTiers(t *testing.T) {
	r := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:    PlatformAnthropic,
		Models:      []string{"claude-sonnet-4"},
		BillingMode: BillingModeToken,
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: testPtrInt(100000), InputPrice: testPtrFloat64(2e-6)},
			{MinTokens: 100000, InputPrice: testPtrFloat64(4e-6)},
		},
	}})
	group := &Group{ID: 100, LongContextPricingEnabled: false, ModelPricing: []ChannelModelPricing{
		testGroupPricing(9e-6, 27e-6, "claude-sonnet-4"),
	}}
	group.compileModelPricingIndex()

	resolved := r.Resolve(context.Background(), PricingInput{Model: "claude-sonnet-4", GroupID: groupIDPtr(), Group: group})
	require.Empty(t, resolved.Intervals)
	require.True(t, resolved.longContextPricingDisabled)
	require.InDelta(t, 9e-6, resolved.BasePricing.InputPricePerToken, 1e-12)
}

func TestCompileGroupModelPricingIndexDropsLegacyTokenIntervals(t *testing.T) {
	group := &Group{ModelPricing: []ChannelModelPricing{{
		Models:      []string{"claude-*"},
		BillingMode: BillingModeToken,
		InputPrice:  testPtrFloat64(1e-6),
		Intervals:   []PricingInterval{{MinTokens: 100000, InputPrice: testPtrFloat64(2e-6)}},
	}}}
	group.compileModelPricingIndex()

	require.Empty(t, group.ModelPricing[0].Intervals)
	require.Same(t, &group.ModelPricing[0], group.modelPricingFor("claude-sonnet-4"))
}

func TestCalculateCostUnifiedVideoUsesContinuousUnits(t *testing.T) {
	price := 0.07
	group := &Group{ID: 100, LongContextPricingEnabled: true, ModelPricing: []ChannelModelPricing{{
		Models:          []string{"grok-imagine-video"},
		BillingMode:     BillingModeVideo,
		PerRequestPrice: &price,
	}}}
	group.compileModelPricingIndex()
	bs := newTestBillingServiceForResolver()
	resolver := NewModelPricingResolver(nil, bs)

	cost, err := bs.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "grok-imagine-video", Group: group,
		UsageUnits: 15, RateMultiplier: 2, Resolver: resolver,
	})
	require.NoError(t, err)
	require.InDelta(t, 1.05, cost.TotalCost, 1e-12)
	require.InDelta(t, 2.10, cost.ActualCost, 1e-12)
	require.Equal(t, string(BillingModeVideo), cost.BillingMode)
}

func TestInclusiveLongContextThreshold(t *testing.T) {
	bs := &BillingService{}
	pricing := &ModelPricing{
		InputPricePerToken: 1e-6, LongContextInputThreshold: 200000,
		LongContextThresholdInclusive: true, LongContextInputMultiplier: 2, LongContextOutputMultiplier: 2,
	}
	require.True(t, bs.shouldApplySessionLongContextPricing(UsageTokens{InputTokens: 200000}, pricing))
	pricing.LongContextThresholdInclusive = false
	require.False(t, bs.shouldApplySessionLongContextPricing(UsageTokens{InputTokens: 200000}, pricing))
}
