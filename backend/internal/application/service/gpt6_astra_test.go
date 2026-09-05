package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGPT6AstraModelIdentityAndCapabilities(t *testing.T) {
	require.Equal(t, "gpt-6-astra", normalizeKnownOpenAICodexModel("gpt-6-astra"))
	require.Equal(t, "gpt-6-astra", normalizeKnownOpenAICodexModel("openai/gpt6_astra"))
	require.True(t, isOpenAIOAuthServableModel("gpt-6-astra"))
	require.True(t, codexManifestKnownImageInputModel("gpt-6-astra"))
	require.True(t, codexManifestKnownPriorityTierModel("gpt-6-astra"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-6-astra"))
	require.Equal(t, "max", normalizeOpenAIReasoningEffortForModel("max", "gpt-6-astra"))
	require.Equal(t, "low", normalizeOpenAIReasoningEffortForModel("minimal", "gpt-6-astra"))
	require.Equal(t, "low", normalizeOpenAIReasoningEffortForModel("none", "gpt-6-astra"))
	require.Empty(t, normalizeKnownOpenAICodexModel("gpt-6-orion"))
	require.False(t, codexManifestKnownImageInputModel("gpt-6-orion"))
	require.False(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-6-orion"))
}

func TestGPT6AstraForwardNormalizesUnsupportedReasoningEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":2}}`)),
	}}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	account := &Account{
		ID:          606,
		Name:        "astra-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://example.com",
			"model_mapping": map[string]any{
				"astra": "gpt-6-astra",
			},
		},
		Extra: map[string]any{"use_responses_api": true},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	result, err := svc.Forward(context.Background(), c, account, []byte(
		`{"model":"astra","stream":false,"reasoning":{"effort":"minimal"},"input":"hello"}`,
	))

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "gpt-6-astra", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "low", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "low", *result.ReasoningEffort)
}

func TestGPT6AstraResponsesShapeOnChatURLNormalizesUnsupportedReasoningEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"id":"resp_astra_shape","type":"response.completed","response":{"id":"resp_astra_shape","object":"response","model":"gpt-6-astra","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n"))),
	}}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	account := &Account{
		ID:          608,
		Name:        "astra-shape-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://example.com",
		},
		Extra: map[string]any{"openai_responses_supported": true},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(
		`{"model":"gpt-6-astra","stream":false,"service_tier":"fast","reasoning":{"effort":"minimal"},"input":"hello"}`,
	), "", "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "low", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
	require.Equal(t, "priority", gjson.GetBytes(upstream.lastBody, "service_tier").String())
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "low", *result.ReasoningEffort)
	require.NotNil(t, result.ServiceTier)
	require.Equal(t, "priority", *result.ServiceTier)
}

func TestGPT6AstraChatCompletionsNormalizesUnsupportedReasoningEffort(t *testing.T) {
	tests := []struct {
		name         string
		useResponses bool
		upstreamBody string
		effortPath   string
	}{
		{
			name:         "Chat conversion",
			useResponses: true,
			upstreamBody: strings.Join([]string{
				`data: {"id":"resp_astra","type":"response.completed","response":{"id":"resp_astra","object":"response","model":"gpt-6-astra","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`,
				"",
				"data: [DONE]",
				"",
			}, "\n"),
			effortPath: "reasoning.effort",
		},
		{
			name:         "Raw Chat",
			useResponses: false,
			upstreamBody: `{"id":"chatcmpl_astra","object":"chat.completion","model":"gpt-6-astra","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
			effortPath:   "reasoning_effort",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			contentType := "application/json"
			if tt.useResponses {
				contentType = "text/event-stream"
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{contentType}},
				Body:       io.NopCloser(strings.NewReader(tt.upstreamBody)),
			}}
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = false
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
			account := &Account{
				ID:          607,
				Name:        "astra-chat-apikey",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Concurrency: 1,
				Credentials: map[string]any{
					"api_key":  "sk-test",
					"base_url": "https://example.com",
				},
				Extra: map[string]any{"openai_responses_supported": tt.useResponses},
			}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)

			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(
				`{"model":"gpt-6-astra","stream":false,"reasoning_effort":"none","temperature":0.3,"top_p":0.9,"messages":[{"role":"user","content":"hello"}]}`,
			), "", "")

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "low", gjson.GetBytes(upstream.lastBody, tt.effortPath).String())
			if tt.useResponses {
				require.False(t, gjson.GetBytes(upstream.lastBody, "temperature").Exists())
				require.False(t, gjson.GetBytes(upstream.lastBody, "top_p").Exists())
			}
			require.NotNil(t, result.ReasoningEffort)
			require.Equal(t, "low", *result.ReasoningEffort)
		})
	}
}

func TestGPT6AstraPricingServiceStaticFallback(t *testing.T) {
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gpt-5.1-codex": {InputCostPerToken: 1.25e-6},
	}}
	pricing := svc.GetModelPricing("openai/gpt6_astra")
	require.NotNil(t, pricing)
	require.InDelta(t, 10e-6, pricing.InputCostPerToken, 1e-12)
	require.InDelta(t, 50e-6, pricing.OutputCostPerToken, 1e-12)
	require.InDelta(t, 12.5e-6, pricing.CacheCreationInputTokenCost, 1e-12)
	require.InDelta(t, 1e-6, pricing.CacheReadInputTokenCost, 1e-12)
	require.Nil(t, svc.GetModelPricing("gpt-6"))
	require.Nil(t, svc.GetModelPricing("gpt-6-orion"))
	require.Nil(t, svc.GetModelPricing("gpt6_orion"))
}

func TestGPT6AstraOfficialFallbackPricing(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	pricing, err := svc.GetModelPricing("gpt-6-astra")
	require.NoError(t, err)
	requireGPT6AstraPricing(t, pricing)

	tokens := UsageTokens{
		InputTokens:         1000,
		OutputTokens:        100,
		CacheCreationTokens: 200,
		CacheReadTokens:     300,
	}
	standard, err := svc.CalculateCostWithServiceTier("gpt-6-astra", tokens, 1, "")
	require.NoError(t, err)
	expectedStandard := 1000*10e-6 + 100*50e-6 + 200*12.5e-6 + 300*1e-6
	require.InDelta(t, expectedStandard, standard.TotalCost, 1e-12)

	fast, err := svc.CalculateCostWithServiceTier("gpt-6-astra", tokens, 1, "priority")
	require.NoError(t, err)
	require.InDelta(t, expectedStandard*2, fast.TotalCost, 1e-12)
	fastAlias, err := svc.CalculateCostWithServiceTier("gpt-6-astra", tokens, 1, " fast ")
	require.NoError(t, err)
	require.InDelta(t, fast.TotalCost, fastAlias.TotalCost, 1e-12)

	flex, err := svc.CalculateCostWithServiceTier("gpt-6-astra", tokens, 1, "flex")
	require.NoError(t, err)
	require.InDelta(t, expectedStandard*0.5, flex.TotalCost, 1e-12)
}

func TestGPT6AstraLongContextPricing(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 272001, OutputTokens: 100}
	cost, err := svc.CalculateCostWithServiceTier("gpt-6-astra", tokens, 1, "")
	require.NoError(t, err)
	require.True(t, cost.LongContextBillingApplied)
	require.InDelta(t, float64(tokens.InputTokens)*20e-6, cost.InputCost, 1e-10)
	require.InDelta(t, float64(tokens.OutputTokens)*75e-6, cost.OutputCost, 1e-10)
}

func TestGPT6AstraBundledPricingCatalog(t *testing.T) {
	path := filepath.Join("..", "..", "..", "resources", "model-pricing", "model_prices_and_context_window.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	pricingSvc := &PricingService{}
	catalog, err := pricingSvc.parsePricingData(data)
	require.NoError(t, err)
	pricing := catalog["gpt-6-astra"]
	require.NotNil(t, pricing)
	require.InDelta(t, 10e-6, pricing.InputCostPerToken, 1e-12)
	require.InDelta(t, 20e-6, pricing.InputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 50e-6, pricing.OutputCostPerToken, 1e-12)
	require.InDelta(t, 100e-6, pricing.OutputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 12.5e-6, pricing.CacheCreationInputTokenCost, 1e-12)
	require.InDelta(t, 25e-6, pricing.CacheCreationInputTokenCostPriority, 1e-12)
	require.InDelta(t, 1e-6, pricing.CacheReadInputTokenCost, 1e-12)
	require.InDelta(t, 2e-6, pricing.CacheReadInputTokenCostPriority, 1e-12)
	require.Equal(t, 272000, pricing.LongContextInputTokenThreshold)

	pricingSvc.pricingData = catalog
	billingSvc := NewBillingService(&config.Config{}, pricingSvc)
	require.True(t, billingSvc.HasIdentifiedTokenPricing("gpt-6-astra"))

	var raw map[string]struct {
		MaxInputTokens             int      `json:"max_input_tokens"`
		MaxOutputTokens            int      `json:"max_output_tokens"`
		Endpoints                  []string `json:"supported_endpoints"`
		Modalities                 []string `json:"supported_modalities"`
		SupportsMaxReasoningEffort bool     `json:"supports_max_reasoning_effort"`
		SupportsPDFInput           bool     `json:"supports_pdf_input"`
	}
	require.NoError(t, json.Unmarshal(data, &raw))
	entry, ok := raw["gpt-6-astra"]
	require.True(t, ok)
	require.Equal(t, 922000, entry.MaxInputTokens)
	require.Equal(t, 128000, entry.MaxOutputTokens)
	require.ElementsMatch(t, []string{"/v1/responses", "/v1/chat/completions", "/v1/batch"}, entry.Endpoints)
	require.ElementsMatch(t, []string{"text", "image"}, entry.Modalities)
	require.True(t, entry.SupportsMaxReasoningEffort)
	require.False(t, entry.SupportsPDFInput)
}

func requireGPT6AstraPricing(t *testing.T, pricing *ModelPricing) {
	t.Helper()
	require.NotNil(t, pricing)
	require.InDelta(t, 10e-6, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 20e-6, pricing.InputPricePerTokenPriority, 1e-12)
	require.InDelta(t, 50e-6, pricing.OutputPricePerToken, 1e-12)
	require.InDelta(t, 100e-6, pricing.OutputPricePerTokenPriority, 1e-12)
	require.InDelta(t, 12.5e-6, pricing.CacheCreationPricePerToken, 1e-12)
	require.InDelta(t, 25e-6, pricing.CacheCreationPricePerTokenPriority, 1e-12)
	require.InDelta(t, 1e-6, pricing.CacheReadPricePerToken, 1e-12)
	require.InDelta(t, 2e-6, pricing.CacheReadPricePerTokenPriority, 1e-12)
	require.Equal(t, 272000, pricing.LongContextInputThreshold)
	require.InDelta(t, 2.0, pricing.LongContextInputMultiplier, 1e-12)
	require.InDelta(t, 1.5, pricing.LongContextOutputMultiplier, 1e-12)
}
