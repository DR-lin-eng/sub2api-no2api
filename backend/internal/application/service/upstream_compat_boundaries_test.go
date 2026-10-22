//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAnthropicForeignOAuthBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{PlatformOpenAI, PlatformGemini, PlatformAntigravity, PlatformGrok, "unknown"} {
		for _, kind := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
			t.Run(platform+kind, func(t *testing.T) {
				account := &Account{Platform: platform, Type: kind}
				svc := &GatewayService{}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				req, _, err := svc.buildUpstreamRequest(context.Background(), c, account, nil, "fixture-token", "oauth", "gpt-6", false, false)
				require.Nil(t, req)
				var fail *UpstreamFailoverError
				require.True(t, errors.As(err, &fail))
				require.Equal(t, anthropicMessagesForeignOAuthReason, fail.Reason)
				recorder := httptest.NewRecorder()
				c, _ = gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
				require.NoError(t, svc.ForwardCountTokens(context.Background(), c, account, &ParsedRequest{}))
				require.Equal(t, http.StatusNotFound, recorder.Code)
			})
		}
	}
	require.True(t, anthropicMessagesCredentialAllowed(&Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}))
	require.True(t, anthropicMessagesCredentialAllowed(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}))
}

type vertexCapacityRepo struct {
	AccountRepository
	resets []time.Time
}

func (r *vertexCapacityRepo) SetRateLimited(_ context.Context, _ int64, at time.Time) error {
	r.resets = append(r.resets, at)
	return nil
}

func TestVertexSharedCapacityDoesNotCoolAccount(t *testing.T) {
	shared := []byte(`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"Resource exhausted. Please try again later."}}`)
	for _, tt := range []struct {
		name   string
		body   []byte
		shared bool
	}{
		{"shared", shared, true},
		{"array", []byte(`[{"error":{"status":"RESOURCE_EXHAUSTED","message":"Resource exhausted. Please try again later."}}]`), true},
		{"quota", []byte(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"Resource exhausted. quota exceeded; try again later"}}`), false},
		{"details", []byte(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"Resource exhausted. Please try again later.","details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"60s"}]}}`), false},
		{"malformed", []byte(`not-json`), false},
	} {
		t.Run(tt.name, func(t *testing.T) { require.Equal(t, tt.shared, isVertexSharedCapacity429(tt.body)) })
	}
	repo := &vertexCapacityRepo{}
	svc := &GeminiMessagesCompatService{accountRepo: repo}
	account := &Account{ID: 8060, Platform: PlatformGemini, Type: AccountTypeServiceAccount}
	svc.handleGeminiUpstreamError(context.Background(), account, 429, nil, shared)
	require.Empty(t, repo.resets)
	svc.handleGeminiUpstreamError(context.Background(), account, 429, nil, []byte(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"project quota exhausted"}}`))
	require.Len(t, repo.resets, 1)
	require.InDelta(t, 60, time.Until(repo.resets[0]).Seconds(), 2)
}

func TestConfiguredMinOutputTokens(t *testing.T) {
	require.Equal(t, 128, configuredMinOutputTokens(nil))
	for _, floor := range []int{0, 15, 16, 64, 128, 512} {
		cfg := &config.Config{Gateway: config.GatewayConfig{MinOutputTokens: floor}}
		want := floor
		if want < 16 {
			want = 128
		}
		response, err := anthropicToResponses(cfg, &apicompat.AnthropicRequest{Model: "gpt-6", MaxTokens: 1})
		require.NoError(t, err)
		require.Equal(t, want, *response.MaxOutputTokens)
	}
}

func TestGeminiArgonFallbackPricing(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	for _, tt := range []struct {
		model                string
		input, output, cache float64
	}{
		{"gemini-4-argon", 2e-6, 10e-6, 0.1e-6},
		{"gemini-4-argon-high", 2e-6, 10e-6, 0.1e-6},
		{"gemini-4-argon-medium", 2e-6, 10e-6, 0.1e-6},
		{"gemini-4-argon-low", 2e-6, 10e-6, 0.1e-6},
		{"gemini-4-argon-cyber-permissive", 4e-6, 20e-6, 0.2e-6},
	} {
		t.Run(tt.model, func(t *testing.T) {
			price := svc.getFallbackPricing(tt.model)
			require.NotNil(t, price)
			require.Equal(t, tt.input, price.InputPricePerToken)
			require.Equal(t, tt.output, price.OutputPricePerToken)
			require.Equal(t, tt.cache, price.CacheReadPricePerToken)
			require.Zero(t, price.LongContextInputThreshold)
		})
	}
}

func TestGeminiArgonExplicitTierPriceTakesPrecedence(t *testing.T) {
	base := &LiteLLMModelPricing{InputCostPerToken: 2e-6}
	exact := &LiteLLMModelPricing{InputCostPerToken: 3e-6}
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{"gemini-4-argon": base, "gemini-4-argon-low": exact}}
	require.Same(t, exact, svc.GetModelPricing("models/gemini-4-argon-low"))
	require.Same(t, base, svc.GetModelPricing("publishers/google/models/gemini-4-argon-high"))
}
func TestGeminiMergePreservesDistinctTextSignatures(t *testing.T) {
	response := mergeCollectedPartsToResponse(map[string]any{}, []map[string]any{{"text": "alpha", "thoughtSignature": "signature-a"}, {"text": "beta", "thoughtSignature": "signature-b"}})
	parts := extractGeminiParts(response)
	require.Len(t, parts, 2)
	require.Equal(t, "alpha", parts[0]["text"])
	require.Equal(t, "signature-a", parts[0]["thoughtSignature"])
	require.Equal(t, "beta", parts[1]["text"])
	require.Equal(t, "signature-b", parts[1]["thoughtSignature"])
}
