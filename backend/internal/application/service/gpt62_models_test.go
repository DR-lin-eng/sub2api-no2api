//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/modules/modelcatalog"
	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/apicompat"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGPT62DiscoveryAndAliases(t *testing.T) {
	for _, family := range []string{"astra", "sol"} {
		model := "gpt-6.2-" + family
		for _, alias := range []string{model, "openai/GPT6.2_" + strings.ToUpper(family), model + "-max", model + "-2026-11-18", model + "-openai-compact"} {
			require.Equal(t, model, normalizeKnownOpenAICodexModel(alias))
			require.True(t, isOpenAIOAuthServableModel(alias))
			require.True(t, shouldAutoInjectPromptCacheKeyForCompat(alias))
		}
		require.Contains(t, openai.DefaultModelIDs(), model)
		descriptor := modelcatalog.KnownFallback(model)
		require.JSONEq(t, `1050000`, string(descriptor["context_window"]))
		require.JSONEq(t, `128000`, string(descriptor["max_output_tokens"]))
		require.JSONEq(t, `["low","medium","high","xhigh","max"]`, string(descriptor["supported_reasoning_levels"]))
		manifest := convertOpenAIModelListToCodexManifest([]byte(fmt.Sprintf(`{"data":[{"id":%q}]}`, model)))
		require.Equal(t, model, gjson.GetBytes(manifest, "models.0.slug").String())
		require.Equal(t, int64(1050000), gjson.GetBytes(manifest, "models.0.context_window").Int())
		request := &apicompat.AnthropicRequest{Model: model + "-2026-11-18"}
		applyOpenAICompatModelNormalization(request)
		require.Equal(t, model, request.Model)
		require.Nil(t, request.OutputConfig, "a snapshot date must not become a reasoning effort")
	}
	for _, unknown := range []string{"gpt-6.2", "gpt-6.2-orion", "gpt-6.2-astra-unknown", "gpt-6.2-sol-unknown"} {
		require.Empty(t, normalizeKnownOpenAICodexModel(unknown))
		require.False(t, isOpenAIOAuthServableModel(unknown))
		require.False(t, isOpenAIGPT62Model(unknown))
	}
}

func TestGPT62OfficialPricingSourcesAndContextBoundary(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)
	catalog, err := (&PricingService{}).parsePricingData(data)
	require.NoError(t, err)
	for _, model := range []string{"gpt-6.2-astra", "gpt-6.2-sol"} {
		require.InDelta(t, 2.0, catalog[model].LongContextInputCostMultiplier, 1e-15)
		require.InDelta(t, 1.5, catalog[model].LongContextOutputCostMultiplier, 1e-15)
	}
	for _, tc := range []struct {
		model                           string
		input, output, cacheRead, write float64
	}{
		{"gpt-6.2-astra", 8e-6, 40e-6, 0.4e-6, 10e-6},
		{"gpt-6.2-sol", 2e-6, 10e-6, 0.1e-6, 2.5e-6},
	} {
		for _, source := range []string{"static", "dynamic-fallback", "bundled"} {
			t.Run(tc.model+"/"+source, func(t *testing.T) {
				var ps *PricingService
				if source != "static" {
					ps = &PricingService{pricingData: map[string]*LiteLLMModelPricing{}}
					if source == "bundled" {
						ps.pricingData = catalog
					}
				}
				svc := NewBillingService(&config.Config{}, ps)
				price, err := svc.GetModelPricing("openai/" + tc.model + "-max")
				require.NoError(t, err)
				require.InDelta(t, tc.input, price.InputPricePerToken, 1e-15)
				require.InDelta(t, tc.output, price.OutputPricePerToken, 1e-15)
				require.InDelta(t, tc.cacheRead, price.CacheReadPricePerToken, 1e-15)
				require.InDelta(t, tc.write, price.CacheCreationPricePerToken, 1e-15)
				for _, contextSize := range []int{272000, 272001} {
					for _, tier := range []struct {
						name   string
						factor float64
					}{{"default", 1}, {"fast", 2}, {"priority", 2}} {
						tokens := UsageTokens{InputTokens: contextSize - 300, CacheReadTokens: 100, CacheCreationTokens: 200, OutputTokens: 10}
						cost, err := svc.CalculateCostWithServiceTier(tc.model, tokens, 0.25, tier.name)
						require.NoError(t, err)
						inputFactor, outputFactor := 1.0, 1.0
						if contextSize > 272000 {
							inputFactor, outputFactor = 2, 1.5
						}
						want := ((float64(tokens.InputTokens)*tc.input+100*tc.cacheRead+200*tc.write)*inputFactor + 10*tc.output*outputFactor) * tier.factor
						require.InDelta(t, want, cost.TotalCost, 1e-12)
						require.InDelta(t, want*0.25, cost.ActualCost, 1e-12)
						require.Equal(t, contextSize > 272000, cost.LongContextBillingApplied)
					}
				}
			})
		}
	}
}

func TestGPT62UltrafastPromotionPacificBoundaryAndOverrides(t *testing.T) {
	for _, model := range []string{"gpt-6.2-astra", "openai/gpt6.2_sol-max"} {
		for _, source := range []string{PricingSourceFallback, PricingSourceLiteLLM, PricingSourceChannel} {
			svc := NewBillingService(&config.Config{}, nil)
			resolver := NewModelPricingResolver(nil, svc)
			// An explicit catalog/channel Standard card must retain its own prices.
			base := &ModelPricing{InputPricePerToken: 3e-6, OutputPricePerToken: 7e-6,
				CacheReadPricePerToken: 0.3e-6, CacheCreationPricePerToken: 4e-6, ApplyServiceTierMultiplier: true}
			original := *base
			resolved := &ResolvedPricing{Mode: BillingModeToken, Source: source, BasePricing: base}
			for _, tc := range []struct {
				at     string
				factor float64
			}{{"2027-03-01T07:59:59.999999999Z", 3}, {"2027-03-01T08:00:00Z", 6}, {"2027-03-01T16:00:00+08:00", 6}} {
				at, err := time.Parse(time.RFC3339Nano, tc.at)
				require.NoError(t, err)
				for _, contextSize := range []int{272000, 272001} {
					input := CostInput{Ctx: context.Background(), Model: model, RateMultiplier: 0.5, Resolver: resolver, Resolved: resolved,
						PricingAt: at, ServiceTier: "ultrafast", Tokens: UsageTokens{InputTokens: contextSize - 300, CacheReadTokens: 100, CacheCreationTokens: 200, OutputTokens: 10}}
					cost, err := svc.CalculateCostUnified(input)
					require.NoError(t, err)
					inputFactor, outputFactor := 1.0, 1.0
					if contextSize > 272000 {
						inputFactor, outputFactor = 2, 1.5
					}
					want := ((float64(input.Tokens.InputTokens)*3e-6+100*0.3e-6+200*4e-6)*inputFactor + 10*7e-6*outputFactor) * tc.factor
					require.InDelta(t, want, cost.TotalCost, 1e-12, "%s %s %s", model, source, tc.at)
					require.InDelta(t, want*0.5, cost.ActualCost, 1e-12)
					require.Equal(t, original, *base, "price snapshots must stay immutable across the promotion boundary")
				}
			}
		}
	}
}

func TestGPT62UltrafastPromotionWithoutResolver(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	for model, inputRate := range map[string]float64{"gpt-6.2-astra": 8e-6, "gpt-6.2-sol": 2e-6} {
		for _, tc := range []struct {
			at     time.Time
			factor float64
		}{{time.Date(2027, 3, 1, 7, 59, 59, 0, time.UTC), 3}, {time.Date(2027, 3, 1, 8, 0, 0, 0, time.UTC), 6}} {
			cost, err := svc.CalculateCostUnified(CostInput{Model: model, PricingAt: tc.at, Tokens: UsageTokens{InputTokens: 100}, RateMultiplier: 1, ServiceTier: "ultrafast"})
			require.NoError(t, err)
			require.InDelta(t, 100*inputRate*tc.factor, cost.TotalCost, 1e-15)
		}
	}
}

func TestGPT62ChatForwardingStreamsAndReasoning(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, model := range []string{"gpt-6.2-astra", "gpt-6.2-sol"} {
		for _, viaResponses := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				for _, effort := range []string{"none", "minimal", "max"} {
					t.Run(fmt.Sprintf("%s/responses=%t/stream=%t/%s", model, viaResponses, stream, effort), func(t *testing.T) {
						response := fmt.Sprintf(`{"id":"chatcmpl_gpt62","object":"chat.completion","model":%q,"service_tier":"default","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"delta":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":2,"total_tokens":13}}`, model)
						contentType := "application/json"
						if viaResponses {
							response = fmt.Sprintf(`{"id":"resp_gpt62","object":"response","model":%q,"service_tier":"default","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":11,"output_tokens":2,"total_tokens":13}}`, model)
							response = "data: {\"type\":\"response.completed\",\"response\":" + response + "}\n\ndata: [DONE]\n\n"
							contentType = "text/event-stream"
						} else if stream {
							response = "data: " + response + "\n\ndata: [DONE]\n\n"
							contentType = "text/event-stream"
						}
						upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(response))}}
						svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
						account := rawChatCompletionsTestAccount()
						account.Credentials["model_mapping"] = map[string]any{"alias": model}
						account.Extra = map[string]any{"openai_responses_supported": viaResponses}
						body, err := json.Marshal(map[string]any{"model": "alias", "stream": stream, "service_tier": "ultrafast", "reasoning_effort": effort,
							"temperature": 0.3, "top_p": 0.9, "logprobs": true, "top_logprobs": 2, "messages": []map[string]string{{"role": "user", "content": "hello"}}})
						require.NoError(t, err)
						rec := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(rec)
						c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
						result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
						require.NoError(t, err)
						require.Equal(t, 200, rec.Code)
						require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String())
						path := "reasoning_effort"
						if viaResponses {
							path = "reasoning.effort"
						}
						wantEffort := effort
						if effort != "max" {
							wantEffort = "low"
						}
						require.Equal(t, wantEffort, gjson.GetBytes(upstream.lastBody, path).String())
						for _, field := range []string{"temperature", "top_p", "logprobs", "top_logprobs"} {
							require.False(t, gjson.GetBytes(upstream.lastBody, field).Exists(), field)
						}
						require.NotNil(t, result.ReasoningEffort)
						require.Equal(t, wantEffort, *result.ReasoningEffort)
						require.NotNil(t, result.Usage.ServiceTier)
						require.Equal(t, "default", *result.Usage.ServiceTier, "actual upstream tier must win over requested Ultrafast")
						require.Equal(t, 11, result.Usage.InputTokens)
						require.Equal(t, 2, result.Usage.OutputTokens)
					})
				}
			}
		}
	}
}

func TestGPT62ResponsesShapeOnChatURLPreservesReasoningPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, model := range []string{"gpt-6.2-astra", "gpt-6.2-sol"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", model, stream), func(t *testing.T) {
				terminal := fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_gpt62","object":"response","model":%q,"status":"completed","service_tier":"default","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":11,"output_tokens":2}}}`, model)
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200,
					Header: http.Header{"Content-Type": []string{"text/event-stream"}},
					Body:   io.NopCloser(strings.NewReader("data: " + terminal + "\n\ndata: [DONE]\n\n"))}}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				account := rawChatCompletionsTestAccount()
				account.Extra = map[string]any{"openai_responses_supported": true}
				body := []byte(fmt.Sprintf(`{"model":%q,"stream":%t,"instructions":"keep stable","service_tier":"ultrafast","reasoning":{"effort":"none"},"temperature":0.3,"top_p":0.9,"top_logprobs":2,"include":["message.output_text.logprobs","reasoning.encrypted_content"],"input":[{"type":"reasoning","id":"rs_prefix","encrypted_content":"opaque-prefix"},{"role":"user","content":"hello"}]}`, model, stream))
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
				result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
				require.NoError(t, err)
				require.Equal(t, "low", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
				require.JSONEq(t, `["reasoning.encrypted_content"]`, gjson.GetBytes(upstream.lastBody, "include").Raw)
				require.Equal(t, gjson.GetBytes(body, "input").Raw, gjson.GetBytes(upstream.lastBody, "input").Raw)
				require.Equal(t, "keep stable", gjson.GetBytes(upstream.lastBody, "instructions").String())
				for _, field := range []string{"temperature", "top_p", "top_logprobs"} {
					require.False(t, gjson.GetBytes(upstream.lastBody, field).Exists())
				}
				require.NotNil(t, result.Usage.ServiceTier)
				require.Equal(t, "default", *result.Usage.ServiceTier)
			})
		}
	}
}

func TestGPT62AnthropicForwardingReasoningAndActualTier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, model := range []string{"gpt-6.2-astra", "gpt-6.2-sol"} {
		for _, stream := range []bool{false, true} {
			for _, disabled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t/disabled=%t", model, stream, disabled), func(t *testing.T) {
					terminal := fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_gpt62","model":%q,"status":"completed","service_tier":"default","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":11,"output_tokens":2}}}`, model)
					upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200,
						Header: http.Header{"Content-Type": []string{"text/event-stream"}},
						Body:   io.NopCloser(strings.NewReader("data: " + terminal + "\n\ndata: [DONE]\n\n"))}}
					svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					account := rawChatCompletionsTestAccount()
					account.Credentials["model_mapping"] = map[string]any{"alias": model}
					account.Extra = map[string]any{"openai_responses_supported": true}
					request := map[string]any{"model": "alias", "max_tokens": 128, "stream": stream,
						"temperature": 0.3, "top_p": 0.9, "output_config": map[string]string{"effort": "max"},
						"messages": []map[string]string{{"role": "user", "content": "hello"}}}
					if disabled {
						request["thinking"] = map[string]string{"type": "disabled"}
					}
					body, err := json.Marshal(request)
					require.NoError(t, err)
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body)))
					result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
					require.NoError(t, err)
					wantEffort := "max"
					if disabled {
						wantEffort = "low"
					}
					require.Equal(t, wantEffort, gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
					require.False(t, gjson.GetBytes(upstream.lastBody, "temperature").Exists())
					require.False(t, gjson.GetBytes(upstream.lastBody, "top_p").Exists())
					require.NotNil(t, result.Usage.ServiceTier)
					require.Equal(t, "default", *result.Usage.ServiceTier)
				})
			}
		}
	}
}
