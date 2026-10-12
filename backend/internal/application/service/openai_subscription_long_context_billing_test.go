//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecordUsageSubscriptionLongContextBillingMatrix(t *testing.T) {
	for _, kind := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
		for _, grouped := range []string{"none", "enabled", "disabled"} {
			for _, enabled := range []bool{false, true} {
				for _, tier := range []string{"", "priority"} {
					name := fmt.Sprintf("%s/group=%s/account=%t/tier=%s", kind, grouped, enabled, tier)
					t.Run(name, func(t *testing.T) {
						logs := &openAIRecordUsageLogRepoStub{inserted: true}
						users := &openAIRecordUsageUserRepoStub{}
						svc := newOpenAIRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{}, nil)
						key := &APIKey{ID: 1}
						if grouped != "none" {
							id := int64(7)
							key.GroupID = &id
							key.Group = &Group{ID: id, Platform: PlatformOpenAI, RateMultiplier: 1.1,
								LongContextPricingEnabled: grouped == "enabled", Hydrated: true}
							svc.resolver = NewModelPricingResolver(nil, svc.billingService)
						}
						account := &Account{ID: 3, Platform: PlatformOpenAI, Type: kind,
							Extra: map[string]any{openAILongContextBillingEnabledKey: enabled}}
						err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
							Result: &OpenAIForwardResult{RequestID: name, Model: "gpt-6.1-sol", UpstreamModel: "gpt-6.1-sol",
								ServiceTier: &tier, Duration: time.Second,
								Usage: OpenAIUsage{InputTokens: 300000, OutputTokens: 2000, CacheReadInputTokens: 80000, CacheCreationInputTokens: 20000}},
							APIKey: key, User: &User{ID: 2}, Account: account,
						})
						require.NoError(t, err)
						long := kind == AccountTypeAPIKey && enabled && grouped != "disabled"
						inputFactor, outputFactor, tierFactor := 1.0, 1.0, 1.0
						if long {
							inputFactor, outputFactor = 2, 1.5
						}
						if tier == "priority" {
							tierFactor = 2
						}
						require.Equal(t, long, logs.lastLog.LongContextBillingApplied)
						want := (200000*2e-6*inputFactor + 80000*0.1e-6*inputFactor + 20000*2.5e-6*inputFactor + 2000*10e-6*outputFactor) * tierFactor
						require.InDelta(t, want, logs.lastLog.TotalCost, 1e-12)
						require.InDelta(t, want*1.1, logs.lastLog.ActualCost, 1e-12)
						require.Equal(t, 1, users.deductCalls)
					})
				}
			}
		}
	}
}

func TestSubscriptionLongContextExemptionPreservesCustomIntervals(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)
	resolved := &ResolvedPricing{Mode: BillingModeToken, Source: PricingSourceGroup,
		BasePricing: &ModelPricing{InputPricePerToken: 1e-6, OutputPricePerToken: 2e-6},
		Intervals:   []PricingInterval{{MinTokens: 272000, InputPrice: testPtrFloat64(7e-6), OutputPrice: testPtrFloat64(14e-6)}}}
	for _, exempt := range []bool{false, true} {
		cost, err := bs.CalculateCostUnified(CostInput{
			Ctx: context.Background(), Model: "gpt-6.1-sol", Tokens: UsageTokens{InputTokens: 300000, OutputTokens: 2000},
			RateMultiplier: 1, Resolver: resolver, Resolved: resolved, DisableModelLongContextPricing: exempt,
		})
		require.NoError(t, err)
		require.InDelta(t, 2.128, cost.TotalCost, 1e-12)
		require.False(t, cost.LongContextBillingApplied)
	}
}
