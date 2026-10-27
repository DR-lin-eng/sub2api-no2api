package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// JSON lets the same fixture execute against the pre-switch source as well.
func TestImageCountBillingTransaction(t *testing.T) {
	for _, route := range []string{"gateway", "openai"} {
		for _, tc := range []struct {
			name               string
			enabled, dedicated bool
			want               float64
		}{
			{"legacy", false, true, .0045},
			{"dedicated", true, true, .6},
			{"mixed", true, false, .0045},
		} {
			t.Run(route+"/"+tc.name, func(t *testing.T) {
				ctx := context.Background()
				gid := int64(905)
				model := "gpt-image-1"
				resolver := newOpenAITokenImageChannelPricingResolverForTest(t, gid, model)
				price := .3
				group := &Group{ID: gid, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, AllowImageGeneration: true, ImagePrice1K: &price}
				payload, err := json.Marshal(map[string]bool{"ImageCountBilling": tc.enabled})
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(payload, group))
				key := &APIKey{ID: 5, GroupID: &gid, Group: group}
				var cost *CostBreakdown
				payload, err = json.Marshal(map[string]bool{"ImageEndpoint": tc.dedicated})
				require.NoError(t, err)
				if route == "gateway" {
					result := &ForwardResult{Model: model, ImageCount: 2, ImageSize: "1K", Usage: ClaudeUsage{InputTokens: 1000, OutputTokens: 100}}
					require.NoError(t, json.Unmarshal(payload, result))
					svc := &GatewayService{billingService: resolver.billingService, resolver: resolver}
					cost = svc.calculateRecordUsageCost(ctx, result, key, model, 1, 1, nil)
				} else {
					result := &OpenAIForwardResult{Model: model, ImageCount: 2, ImageSize: "1K"}
					require.NoError(t, json.Unmarshal(payload, result))
					svc := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}
					resolved, err := svc.calculateOpenAIRecordUsageCostWithResolution(ctx, result, key, []string{model}, 1, 1, 1, 1, UsageTokens{InputTokens: 1000, OutputTokens: 100}, "", false, false)
					require.NoError(t, err)
					cost = resolved.Cost
				}
				require.NotNil(t, cost)
				t.Logf("route=%s enabled=%v dedicated=%v images=2 cost=%.6f mode=%s", route, tc.enabled, tc.dedicated, cost.TotalCost, cost.BillingMode)
				require.InDelta(t, tc.want, cost.TotalCost, 1e-12)
			})
		}
	}
}
