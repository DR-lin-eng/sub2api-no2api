package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
)

var imageCountBillingBenchmarkCost *CostBreakdown

func BenchmarkImageCountBillingPricing(b *testing.B) {
	for _, tc := range []struct {
		name               string
		enabled, dedicated bool
	}{{"legacy", false, true}, {"mixed", true, false}, {"dedicated", true, true}} {
		b.Run(tc.name, func(b *testing.B) {
			gid := int64(905)
			model := "gpt-image-1"
			input, output, perImage := 3e-6, 15e-6, .3
			cache := newEmptyChannelCache()
			cache.pricingByGroupModel[channelModelKey{groupID: gid, model: model}] = &ChannelModelPricing{BillingMode: BillingModeToken, InputPrice: &input, OutputPrice: &output}
			cache.channelByGroupID[gid] = &Channel{ID: gid, Status: StatusActive}
			cache.groupPlatform[gid] = ""
			cache.loadedAt = time.Now()
			cs := &ChannelService{}
			cs.cache.Store(cache)
			billing := NewBillingService(&config.Config{}, nil)
			resolver := NewModelPricingResolver(cs, billing)
			group := &Group{ID: gid, Platform: PlatformOpenAI, Hydrated: true, AllowImageGeneration: true, ImagePrice1K: &perImage}
			raw, _ := json.Marshal(map[string]bool{"ImageCountBilling": tc.enabled})
			if err := json.Unmarshal(raw, group); err != nil {
				b.Fatal(err)
			}
			result := &ForwardResult{Model: model, ImageCount: 2, ImageSize: "1K", Usage: ClaudeUsage{InputTokens: 1000, OutputTokens: 100}}
			raw, _ = json.Marshal(map[string]bool{"ImageEndpoint": tc.dedicated})
			if err := json.Unmarshal(raw, result); err != nil {
				b.Fatal(err)
			}
			key := &APIKey{ID: 5, GroupID: &gid, Group: group}
			svc := &GatewayService{billingService: billing, resolver: resolver}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				imageCountBillingBenchmarkCost = svc.calculateRecordUsageCost(context.Background(), result, key, model, 1, 1, nil)
			}
		})
	}
}
