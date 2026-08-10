//go:build unit

package service

import (
	"context"
	"testing"
)

var (
	modelPricingResolverBenchmarkSink        *ResolvedPricing
	modelPricingResolverChannelBenchmarkSink *ChannelModelPricing
)

func BenchmarkModelPricingResolverNoChannelPricing(b *testing.B) {
	const groupID = int64(100)

	channelService := &ChannelService{}
	channelService.cache.Store(populateChannelCache(
		[]Channel{{
			ID:       1,
			Status:   StatusActive,
			GroupIDs: []int64{groupID},
		}},
		map[int64]string{groupID: PlatformAnthropic},
	))
	resolver := NewModelPricingResolver(channelService, newTestBillingServiceForResolver())
	ctx := context.Background()
	gid := groupID
	input := PricingInput{Model: "claude-sonnet-4", GroupID: &gid}

	b.Run("single_cache_lookup", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			modelPricingResolverBenchmarkSink = resolver.Resolve(ctx, input)
		}
	})

	b.Run("legacy_duplicate_cache_lookup", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			modelPricingResolverBenchmarkSink = resolver.Resolve(ctx, input)
			modelPricingResolverChannelBenchmarkSink = channelService.GetChannelModelPricing(ctx, groupID, input.Model)
		}
	})
}
