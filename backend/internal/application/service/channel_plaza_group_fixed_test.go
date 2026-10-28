//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlazaGroupFixedPricingMatchesBillingPrecedence(t *testing.T) {
	for _, mode := range []BillingMode{BillingModeImage, BillingModePerRequest, BillingModeVideo} {
		for _, pattern := range []string{"fixture-image", "fixture-*"} {
			group := Group{ID: 1, Name: "fixture", Platform: PlatformOpenAI, ImagePrice1K: testPtrFloat64(.9), ModelPricing: []ChannelModelPricing{{Models: []string{pattern}, BillingMode: mode, PerRequestPrice: testPtrFloat64(.12), Intervals: []PricingInterval{{TierLabel: "1K", PerRequestPrice: testPtrFloat64(.14)}}}}}
			channel := Channel{ID: 1, Name: "fixture", Status: StatusActive, GroupIDs: []int64{1}, ModelPricing: []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"fixture-image"}, BillingMode: BillingModeImage, PerRequestPrice: testPtrFloat64(.5)}}}
			plaza, err := newPlazaChannelService([]Channel{channel}, []Group{group}, nil).ListPlazaGroups(context.Background(), PlazaListOptions{})
			require.NoError(t, err)
			require.Len(t, plaza, 1)
			require.Len(t, plaza[0].Models, 1)
			card := plaza[0].Models[0].Pricing
			require.Equal(t, mode, card.BillingMode)
			require.InDelta(t, .12, *card.PerRequestPrice, 1e-12)
			require.Len(t, card.Intervals, 1)
			require.InDelta(t, .14, *card.Intervals[0].PerRequestPrice, 1e-12)
			require.InDelta(t, .5, *channel.ModelPricing[0].PerRequestPrice, 1e-12)
			group.compileModelPricingIndex()
			resolved := NewModelPricingResolver(nil, newTestBillingService()).Resolve(context.Background(), PricingInput{Model: "fixture-image", Group: &group})
			require.Equal(t, mode, resolved.Mode)
			require.Equal(t, PricingSourceGroup, resolved.Source)
		}
	}
}
