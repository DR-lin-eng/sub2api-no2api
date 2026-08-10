//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

const (
	anthropicCheapResponseModel   = "claude-sonnet-4"
	anthropicPricierResponseModel = "claude-opus-4.8"
	openAICheapResponseModel      = "gpt-5.4-nano"
	openAIPriceyResponseModel     = "gpt-5.5"
)

var responseModelBillingBenchmarkSink *CostBreakdown

func orderedResponseBillingModels(
	t *testing.T,
	billing *BillingService,
	tokens UsageTokens,
	a string,
	b string,
) (cheaper string, pricier string, cheaperCost *CostBreakdown, pricierCost *CostBreakdown) {
	t.Helper()
	costA, err := billing.CalculateCost(a, tokens, 1.1)
	require.NoError(t, err)
	costB, err := billing.CalculateCost(b, tokens, 1.1)
	require.NoError(t, err)
	require.NotEqual(t, costA.TotalCost, costB.TotalCost, "fixture prices for %s and %s must differ", a, b)
	require.True(t, billing.HasIdentifiedTokenPricing(a), "fixture model %s must be identifiable", a)
	require.True(t, billing.HasIdentifiedTokenPricing(b), "fixture model %s must be identifiable", b)
	if costA.TotalCost < costB.TotalCost {
		return a, b, costA, costB
	}
	return b, a, costB, costA
}

func TestGatewayServiceRecordUsage_ResponseModelBillingAdmission(t *testing.T) {
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50}
	tests := []struct {
		name          string
		responseModel func(cheaper, pricier string) string
		conflict      bool
		source        string
		wantCheaper   bool
	}{
		{
			name:          "cheaper_response_model_is_adopted",
			responseModel: func(cheaper, _ string) string { return cheaper },
			source:        BillingModelSourceResponse,
			wantCheaper:   true,
		},
		{
			name:          "pricier_response_model_is_rejected",
			responseModel: func(_, pricier string) string { return pricier },
			source:        BillingModelSourceResponse,
		},
		{
			name:          "conflicting_declarations_fall_back",
			responseModel: func(cheaper, _ string) string { return cheaper },
			conflict:      true,
			source:        BillingModelSourceResponse,
		},
		{
			name:          "unidentified_model_falls_back",
			responseModel: func(_, _ string) string { return "unpriced-response-model" },
			source:        BillingModelSourceResponse,
		},
		{
			name:          "default_mode_ignores_response_model",
			responseModel: func(cheaper, _ string) string { return cheaper },
			source:        BillingModelSourceChannelMapped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			userRepo := &openAIRecordUsageUserRepoStub{}
			svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{})
			cheaper, pricier, cheaperCost, pricierCost := orderedResponseBillingModels(
				t, svc.billingService, tokens, anthropicCheapResponseModel, anthropicPricierResponseModel,
			)
			baselineModel := pricier
			if tt.name == "pricier_response_model_is_rejected" {
				baselineModel = cheaper
			}

			err := svc.RecordUsage(context.Background(), &RecordUsageInput{
				Result: &ForwardResult{
					RequestID:                     "gateway_response_model_" + tt.name,
					Usage:                         ClaudeUsage{InputTokens: tokens.InputTokens, OutputTokens: tokens.OutputTokens},
					Model:                         baselineModel,
					UpstreamResponseModel:         tt.responseModel(cheaper, pricier),
					UpstreamResponseModelConflict: tt.conflict,
					Duration:                      time.Second,
				},
				APIKey:  &APIKey{ID: 501, Quota: 100},
				User:    &User{ID: 601},
				Account: &Account{ID: 701},
				ChannelUsageFields: ChannelUsageFields{
					ChannelID:          9,
					OriginalModel:      baselineModel,
					ChannelMappedModel: baselineModel,
					BillingModelSource: tt.source,
				},
			})

			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog)
			want := pricierCost
			if baselineModel == cheaper || tt.wantCheaper {
				want = cheaperCost
			}
			require.InDelta(t, want.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
			require.InDelta(t, want.ActualCost, userRepo.lastAmount, 1e-12)
			require.Greater(t, usageRepo.lastLog.ActualCost, 0.0)
		})
	}
}

func TestOpenAIGatewayServiceRecordUsage_ResponseModelBillingAdmission(t *testing.T) {
	tokens := UsageTokens{InputTokens: 20, OutputTokens: 10}
	tests := []struct {
		name          string
		responseModel func(cheaper, pricier string) string
		conflict      bool
		source        string
		wantCheaper   bool
	}{
		{
			name:          "cheaper_response_model_is_adopted",
			responseModel: func(cheaper, _ string) string { return cheaper },
			source:        BillingModelSourceResponse,
			wantCheaper:   true,
		},
		{
			name:          "pricier_response_model_is_rejected",
			responseModel: func(_, pricier string) string { return pricier },
			source:        BillingModelSourceResponse,
		},
		{
			name:          "conflicting_declarations_fall_back",
			responseModel: func(cheaper, _ string) string { return cheaper },
			conflict:      true,
			source:        BillingModelSourceResponse,
		},
		{
			name:          "unidentified_model_falls_back",
			responseModel: func(_, _ string) string { return "unpriced-response-model" },
			source:        BillingModelSourceResponse,
		},
		{
			name:          "default_mode_ignores_response_model",
			responseModel: func(cheaper, _ string) string { return cheaper },
			source:        BillingModelSourceChannelMapped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			userRepo := &openAIRecordUsageUserRepoStub{}
			svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
			cheaper, pricier, cheaperCost, pricierCost := orderedResponseBillingModels(
				t, svc.billingService, tokens, openAICheapResponseModel, openAIPriceyResponseModel,
			)
			baselineModel := pricier
			if tt.name == "pricier_response_model_is_rejected" {
				baselineModel = cheaper
			}

			err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{
					RequestID:                     "openai_response_model_" + tt.name,
					Model:                         baselineModel,
					UpstreamModel:                 baselineModel,
					UpstreamResponseModel:         tt.responseModel(cheaper, pricier),
					UpstreamResponseModelConflict: tt.conflict,
					Usage:                         OpenAIUsage{InputTokens: tokens.InputTokens, OutputTokens: tokens.OutputTokens},
					Duration:                      time.Second,
				},
				APIKey:  &APIKey{ID: 10},
				User:    &User{ID: 20},
				Account: &Account{ID: 30},
				ChannelUsageFields: ChannelUsageFields{
					ChannelID:          9,
					OriginalModel:      baselineModel,
					ChannelMappedModel: baselineModel,
					BillingModelSource: tt.source,
				},
			})

			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog)
			want := pricierCost
			if baselineModel == cheaper || tt.wantCheaper {
				want = cheaperCost
			}
			require.InDelta(t, want.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
			require.InDelta(t, want.ActualCost, userRepo.lastAmount, 1e-12)
			require.Greater(t, usageRepo.lastLog.ActualCost, 0.0)
		})
	}
}

func TestResponseModelBillingDeclaration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		source      string
		model       string
		conflict    bool
		mediaBilled bool
		want        string
	}{
		{name: "opted_in_and_clean", source: BillingModelSourceResponse, model: " claude-sonnet-4 ", want: "claude-sonnet-4"},
		{name: "default_source_ignored", source: BillingModelSourceChannelMapped, model: "claude-sonnet-4"},
		{name: "upstream_source_ignored", source: BillingModelSourceUpstream, model: "claude-sonnet-4"},
		{name: "conflict_rejected", source: BillingModelSourceResponse, model: "claude-sonnet-4", conflict: true},
		{name: "media_request_rejected", source: BillingModelSourceResponse, model: "claude-sonnet-4", mediaBilled: true},
		{name: "blank_declaration_rejected", source: BillingModelSourceResponse, model: "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, responseModelBillingDeclaration(tt.source, tt.model, tt.conflict, tt.mediaBilled))
		})
	}
}

func TestResponseModelBillingAdoptable(t *testing.T) {
	t.Parallel()
	cost := func(total float64) *CostBreakdown {
		return &CostBreakdown{TotalCost: total, ActualCost: total}
	}
	tests := []struct {
		name                  string
		baseline              *CostBreakdown
		response              *CostBreakdown
		baselineChannelPriced bool
		responseChannelPriced bool
		want                  bool
	}{
		{name: "cheaper", baseline: cost(1), response: cost(0.5), want: true},
		{name: "equal", baseline: cost(1), response: cost(1), want: true},
		{name: "float_noise", baseline: cost(1), response: cost(1 + 1e-13), want: true},
		{name: "pricier", baseline: cost(1), response: cost(1.0001)},
		{name: "billable_to_zero", baseline: cost(1), response: cost(0)},
		{name: "billable_to_negative", baseline: cost(1), response: cost(-1)},
		{name: "zero_stays_zero", baseline: cost(0), response: cost(0), want: true},
		{name: "channel_to_global", baseline: cost(1), response: cost(0.5), baselineChannelPriced: true},
		{name: "channel_to_channel", baseline: cost(1), response: cost(0.5), baselineChannelPriced: true, responseChannelPriced: true, want: true},
		{name: "global_to_channel", baseline: cost(1), response: cost(0.5), responseChannelPriced: true, want: true},
		{name: "nil_baseline", response: cost(0.5)},
		{name: "nil_response", baseline: cost(1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, responseModelBillingAdoptable(
				tt.baseline, tt.response, tt.baselineChannelPriced, tt.responseChannelPriced,
			))
		})
	}
}

func TestBillingServiceHasIdentifiedTokenPricing_RejectsGuessesAndImageOnlyEntries(t *testing.T) {
	t.Parallel()
	pricingService := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"token-model": {InputCostPerToken: 1e-6, OutputCostPerToken: 2e-6},
		"image-only":  {OutputCostPerImage: 0.02, TokenPricingAbsent: true},
	}}
	billing := NewBillingService(&config.Config{}, pricingService)

	require.True(t, billing.HasIdentifiedTokenPricing(" token-model "))
	require.True(t, billing.HasIdentifiedTokenPricing("CLAUDE-SONNET-4"))
	require.False(t, billing.HasIdentifiedTokenPricing("image-only"))
	require.False(t, billing.HasIdentifiedTokenPricing("totally-made-up-haiku-v9"))
	require.False(t, billing.HasIdentifiedTokenPricing("unknown-gpt-99"))
	require.False(t, billing.HasIdentifiedTokenPricing(""))
}

func TestGatewayServiceRecordUsage_ResponseModelSkippedForImageBilling(t *testing.T) {
	pricingService := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"pricey-image-model": {
			InputCostPerToken: 1e-6, OutputCostPerToken: 2e-6, OutputCostPerImage: 0.20,
		},
		"cheap-image-model": {
			InputCostPerToken: 1e-6, OutputCostPerToken: 2e-6, OutputCostPerImage: 0.01,
		},
	}}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{})
	svc.billingService = NewBillingService(svc.cfg, pricingService)
	expected := svc.billingService.CalculateImageCost("pricey-image-model", ImageBillingSize1K, 1, nil, 1.1)
	require.Greater(t, expected.ActualCost, 0.0)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID:             "gateway_response_model_image",
			Model:                 "pricey-image-model",
			UpstreamResponseModel: "cheap-image-model",
			ImageCount:            1,
			ImageSize:             ImageBillingSize1K,
			Duration:              time.Second,
		},
		APIKey:  &APIKey{ID: 501, Quota: 100},
		User:    &User{ID: 601},
		Account: &Account{ID: 701},
		ChannelUsageFields: ChannelUsageFields{
			OriginalModel:      "pricey-image-model",
			ChannelMappedModel: "pricey-image-model",
			BillingModelSource: BillingModelSourceResponse,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, expected.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, expected.ActualCost, userRepo.lastAmount, 1e-12)
}

func TestOpenAIGatewayServiceRecordUsage_ResponseModelSkippedForIndependentBilling(t *testing.T) {
	t.Run("image", func(t *testing.T) {
		pricingService := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
			"pricey-image-model": {
				InputCostPerToken: 1e-6, OutputCostPerToken: 2e-6, OutputCostPerImage: 0.20,
			},
			"cheap-image-model": {
				InputCostPerToken: 1e-6, OutputCostPerToken: 2e-6, OutputCostPerImage: 0.01,
			},
		}}
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		userRepo := &openAIRecordUsageUserRepoStub{}
		svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
		svc.billingService = NewBillingService(svc.cfg, pricingService)
		expected := svc.billingService.CalculateImageCost("pricey-image-model", ImageBillingSize1K, 1, nil, 1.1)

		err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
			Result: &OpenAIForwardResult{
				RequestID:             "openai_response_model_image",
				Model:                 "pricey-image-model",
				UpstreamModel:         "pricey-image-model",
				UpstreamResponseModel: "cheap-image-model",
				ImageCount:            1,
				ImageSize:             ImageBillingSize1K,
				Duration:              time.Second,
			},
			APIKey:  &APIKey{ID: 10},
			User:    &User{ID: 20},
			Account: &Account{ID: 30},
			ChannelUsageFields: ChannelUsageFields{
				OriginalModel:      "pricey-image-model",
				ChannelMappedModel: "pricey-image-model",
				BillingModelSource: BillingModelSourceResponse,
			},
		})

		require.NoError(t, err)
		require.InDelta(t, expected.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
		require.InDelta(t, expected.ActualCost, userRepo.lastAmount, 1e-12)
	})

	t.Run("video", func(t *testing.T) {
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		userRepo := &openAIRecordUsageUserRepoStub{}
		svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
		expected := svc.billingService.CalculateVideoCost(
			"grok-imagine-video", VideoBillingResolution720P, 1, 5, nil, 1.1,
		)
		responseCost, err := svc.billingService.CalculateCost(
			openAICheapResponseModel, UsageTokens{InputTokens: 20, OutputTokens: 10}, 1.1,
		)
		require.NoError(t, err)
		require.Less(t, responseCost.TotalCost, expected.TotalCost)

		err = svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
			Result: &OpenAIForwardResult{
				RequestID:             "openai_response_model_video",
				Model:                 "grok-imagine-video",
				UpstreamModel:         "grok-imagine-video",
				UpstreamResponseModel: openAICheapResponseModel,
				Usage:                 OpenAIUsage{InputTokens: 20, OutputTokens: 10},
				VideoCount:            1,
				VideoResolution:       VideoBillingResolution720P,
				VideoDurationSeconds:  5,
				Duration:              time.Second,
			},
			APIKey:  &APIKey{ID: 10},
			User:    &User{ID: 20},
			Account: &Account{ID: 30},
			ChannelUsageFields: ChannelUsageFields{
				OriginalModel:      "grok-imagine-video",
				ChannelMappedModel: "grok-imagine-video",
				BillingModelSource: BillingModelSourceResponse,
			},
		})

		require.NoError(t, err)
		require.InDelta(t, expected.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
		require.InDelta(t, expected.ActualCost, userRepo.lastAmount, 1e-12)
	})

	t.Run("web_search", func(t *testing.T) {
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		userRepo := &openAIRecordUsageUserRepoStub{}
		svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
		expected := svc.billingService.CalculateWebSearchCost(1, nil, 1.1)
		responseCost, err := svc.billingService.CalculateCost(
			openAICheapResponseModel, UsageTokens{InputTokens: 20, OutputTokens: 10}, 1.1,
		)
		require.NoError(t, err)
		require.Less(t, responseCost.TotalCost, expected.TotalCost)

		err = svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
			Result: &OpenAIForwardResult{
				RequestID:             "openai_response_model_web_search",
				Model:                 openAIPriceyResponseModel,
				UpstreamModel:         openAIPriceyResponseModel,
				UpstreamResponseModel: openAICheapResponseModel,
				Usage:                 OpenAIUsage{InputTokens: 20, OutputTokens: 10},
				WebSearchCalls:        1,
				Duration:              time.Second,
			},
			APIKey:  &APIKey{ID: 10},
			User:    &User{ID: 20},
			Account: &Account{ID: 30},
			ChannelUsageFields: ChannelUsageFields{
				OriginalModel:      openAIPriceyResponseModel,
				ChannelMappedModel: openAIPriceyResponseModel,
				BillingModelSource: BillingModelSourceResponse,
			},
		})

		require.NoError(t, err)
		require.InDelta(t, expected.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
		require.InDelta(t, expected.ActualCost, userRepo.lastAmount, 1e-12)
	})
}

func TestOpenAIGatewayServiceRecordUsage_ResponseGlobalPriceCannotBypassActualChannelCandidate(t *testing.T) {
	const (
		groupID              = int64(42)
		unpricedPrimaryModel = "unpriced-primary-model"
		channelPricedModel   = "channel-priced-model"
	)
	channelInputPrice := 0.01
	channelOutputPrice := 0.02
	repo := &mockChannelRepository{
		listAllFn: func(context.Context) ([]Channel, error) {
			return []Channel{{
				ID:       9,
				Name:     "response-model-test",
				Status:   StatusActive,
				GroupIDs: []int64{groupID},
				ModelPricing: []ChannelModelPricing{{
					Platform:    PlatformOpenAI,
					Models:      []string{channelPricedModel},
					BillingMode: BillingModeToken,
					InputPrice:  &channelInputPrice,
					OutputPrice: &channelOutputPrice,
				}},
			}}, nil
		},
		getGroupPlatformsFn: func(context.Context, []int64) (map[int64]string, error) {
			return map[int64]string{groupID: PlatformOpenAI}, nil
		},
	}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
	channelService := NewChannelService(repo, nil, nil, nil)
	svc.resolver = NewModelPricingResolver(channelService, svc.billingService)
	group := &Group{ID: groupID, Platform: PlatformOpenAI, RateMultiplier: 1.1}
	apiKey := &APIKey{ID: 10, GroupID: &group.ID, Group: group}
	tokens := UsageTokens{InputTokens: 20, OutputTokens: 10}
	resolved := svc.resolver.Resolve(context.Background(), PricingInput{Model: channelPricedModel, GroupID: &group.ID})
	require.Equal(t, PricingSourceChannel, resolved.Source)
	expected, err := svc.billingService.CalculateCostUnified(CostInput{
		Model:          channelPricedModel,
		GroupID:        &group.ID,
		Tokens:         tokens,
		RequestCount:   1,
		RateMultiplier: 1.1,
		Resolver:       svc.resolver,
		Resolved:       resolved,
	})
	require.NoError(t, err)
	globalResponseCost, err := svc.billingService.CalculateCost(openAICheapResponseModel, tokens, 1.1)
	require.NoError(t, err)
	require.Less(t, globalResponseCost.TotalCost, expected.TotalCost)

	err = svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:             "openai_response_model_actual_channel_candidate",
			Model:                 unpricedPrimaryModel,
			UpstreamResponseModel: openAICheapResponseModel,
			Usage:                 OpenAIUsage{InputTokens: tokens.InputTokens, OutputTokens: tokens.OutputTokens},
			Duration:              time.Second,
		},
		APIKey:  apiKey,
		User:    &User{ID: 20},
		Account: &Account{ID: 30},
		ChannelUsageFields: ChannelUsageFields{
			ChannelID:          9,
			OriginalModel:      unpricedPrimaryModel,
			ChannelMappedModel: channelPricedModel,
			BillingModelSource: BillingModelSourceResponse,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, expected.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, expected.ActualCost, userRepo.lastAmount, 1e-12)
}

func TestChannelMappingResultToUsageFields_ResponseModelSourcePassesThrough(t *testing.T) {
	r := ChannelMappingResult{
		MappedModel:        "claude-fable-5",
		ChannelID:          4,
		BillingModelSource: BillingModelSourceResponse,
	}
	fields := r.ToUsageFields("claude-fable-5", "claude-fable-5")
	require.Equal(t, int64(4), fields.ChannelID)
	require.Equal(t, BillingModelSourceResponse, fields.BillingModelSource)
}

func BenchmarkOpenAIRecordUsageCostResolution_DefaultMode(b *testing.B) {
	svc := newOpenAIRecordUsageServiceForTest(
		&openAIRecordUsageLogRepoStub{},
		&openAIRecordUsageUserRepoStub{},
		&openAIRecordUsageSubRepoStub{},
		nil,
	)
	result := &OpenAIForwardResult{Model: openAIPriceyResponseModel}
	tokens := UsageTokens{InputTokens: 20, OutputTokens: 10}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		resolution, err := svc.calculateOpenAIRecordUsageCostWithResolution(
			context.Background(), result, &APIKey{}, []string{openAIPriceyResponseModel},
			1.1, 1.1, 1.1, 1.1, tokens, "", false,
		)
		if err != nil {
			b.Fatal(err)
		}
		responseModelBillingBenchmarkSink = resolution.Cost
	}
}

func BenchmarkResponseModelBillingDeclaration(b *testing.B) {
	for _, source := range []string{BillingModelSourceChannelMapped, BillingModelSourceResponse} {
		b.Run(source, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if responseModelBillingDeclaration(source, openAICheapResponseModel, false, false) != "" {
					responseModelBillingBenchmarkSink = &CostBreakdown{}
				}
			}
		})
	}
}
