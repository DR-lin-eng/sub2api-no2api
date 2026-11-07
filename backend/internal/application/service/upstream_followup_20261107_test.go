//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai_compat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNativeImageCacheBillingPreservesImageMultiplier(t *testing.T) {
	usage, ok := codexDirectImagesUsage([]byte(`{"usage":{"input_tokens":100,"input_tokens_details":{"image_tokens":80,"cached_tokens":40,"cached_tokens_details":{"image_tokens":30,"text_tokens":10}},"output_tokens":20}}`))
	require.True(t, ok)
	require.Equal(t, 80, usage.ImageInputTokens)
	require.Equal(t, 30, usage.ImageCacheReadTokens)
	svc := &BillingService{}
	price, err := svc.GetModelPricing("gpt-image-2.6-flare")
	require.NoError(t, err)
	imageMultiplier := 3.0
	cost := svc.computeTokenBreakdown(price, UsageTokens{InputTokens: 60, ImageInputTokens: 50, OutputTokens: 20, ImageOutputTokens: 20, CacheReadTokens: 40, ImageCacheReadTokens: 30}, 2, &imageMultiplier, "", false)
	require.InDelta(t, 0.0011225, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.003305, cost.ActualCost, 1e-12)
	require.InDelta(t, 0.0000725, cost.CacheReadCost, 1e-12)
}

func TestTerraCodexMetadataRetainsExplicitUpstreamValues(t *testing.T) {
	body := []byte(`{"models":[{"slug":"gpt-6.1-terra","supports_parallel_tool_calls":false,"supported_reasoning_levels":[]}]}`)
	result, err := enrichCodexModelsManifest(body, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth})
	require.NoError(t, err)
	require.EqualValues(t, 500000, gjson.GetBytes(result, "models.0.context_window").Int())
	require.False(t, gjson.GetBytes(result, "models.0.supports_parallel_tool_calls").Bool())
	require.Equal(t, "[]", gjson.GetBytes(result, "models.0.supported_reasoning_levels").Raw)
	var descriptor struct {
		ModelMessages struct {
			Instructions string `json:"instructions_template"`
		} `json:"model_messages"`
	}
	require.NoError(t, json.Unmarshal(openai.CodexGPT61TerraMetadata, &descriptor))
	require.Equal(t, descriptor.ModelMessages.Instructions, openai.CodexBaseInstructionsForModel("gpt-6.1-terra"))
	// Returned metadata is request-owned; callers cannot mutate the compiled descriptor.
	first := codexKnownModelFallback("gpt-6.1-terra")
	first["context_window"] = json.RawMessage(`1`)
	require.JSONEq(t, `500000`, string(codexKnownModelFallback("gpt-6.1-terra")["context_window"]))
	for _, typ := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		require.True(t, openAISubscriptionLongContextExempt(&Account{Platform: PlatformOpenAI, Type: typ}, &OpenAIForwardResult{UpstreamModel: "gpt-6.1-terra"}))
	}
	require.False(t, openAISubscriptionLongContextExempt(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, &OpenAIForwardResult{UpstreamModel: "gpt-6.1-terra"}))
}

func TestAntigravityInternal500TypedConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.AntigravityInternal500ImageCooldownSeconds = 45
	cfg.Gateway.AntigravityInternal500Tier1Seconds = 120
	cfg.Gateway.AntigravityInternal500Tier2Seconds = 600
	disable := false
	cfg.Gateway.AntigravityInternal500DisableAccount = &disable
	svc := &AntigravityGatewayService{settingService: &SettingService{cfg: cfg}}
	require.Equal(t, 45*time.Second, svc.internal500ImageCooldown())
	tier1, tier2, disabled := svc.internal500PenaltyTiers()
	require.Equal(t, 2*time.Minute, tier1)
	require.Equal(t, 10*time.Minute, tier2)
	require.False(t, disabled)
	require.Equal(t, defaultInternal500ImageCooldown, internal500Duration(1<<62, defaultInternal500ImageCooldown))
}

func TestResponsesProbeSkipsExplicitModes(t *testing.T) {
	for _, mode := range []string{string(openai_compat.ResponsesSupportModeForceResponses), string(openai_compat.ResponsesSupportModeForceChatCompletions)} {
		account := Account{ID: 96, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{openai_compat.ExtraKeyResponsesMode: mode}, Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://api.openai.com/v1"}}
		upstream := &httpUpstreamRecorder{}
		svc := &AccountTestService{accountRepo: &stubOpenAIAccountRepo{accounts: []Account{account}}, httpUpstream: upstream}
		svc.ProbeOpenAIAPIKeyResponsesSupport(context.Background(), account.ID)
		require.Nil(t, upstream.lastReq)
	}
}

func TestNativeImagesRewriteRetainsUnknownFields(t *testing.T) {
	body := []byte(`{"future":{"enabled":true},"data":[{"b64_json":"AA==","future_item":{"value":1},"output_format":"gif"}]}`)
	result, err := rewriteCodexDirectImagesResponse(body, &OpenAIImagesRequest{Model: "gpt-image-2.6-flare", ResponseFormat: "url"})
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(result, "future.enabled").Bool())
	require.EqualValues(t, 1, gjson.GetBytes(result, "data.0.future_item.value").Int())
	require.Equal(t, "data:image/gif;base64,AA==", gjson.GetBytes(result, "data.0.url").String())
	require.False(t, gjson.GetBytes(result, "data.0.b64_json").Exists())
}

func TestNativeImagesResponseIgnoresNullAndRejectsNonStringOutput(t *testing.T) {
	body := []byte(`{"data":[null,{"b64_json":"AA=="}]}`)
	result, err := rewriteCodexDirectImagesResponse(body, &OpenAIImagesRequest{Model: "gpt-image-2.6-flare"})
	require.NoError(t, err)
	require.Equal(t, gjson.Null, gjson.GetBytes(result, "data.0").Type)
	images, err := parseCodexDirectImagesResponse(body)
	require.NoError(t, err)
	require.Len(t, images, 1)
	_, err = parseCodexDirectImagesResponse([]byte(`{"data":[{"b64_json":42}]}`))
	require.Error(t, err)
}

func TestAccountTestNativeImagesUsesSamePayloadAndProtocol(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: openAIImagesJSONResponse()}
	svc := &AccountTestService{httpUpstream: upstream}
	c, rec := newOpenAIImagesTestContext(t, nil)
	require.NoError(t, svc.testOpenAIImageOAuth(c, context.Background(), directImagesTestAccount(), "gpt-image-2.6-flare", "draw a cat"))
	require.Equal(t, "/backend-api/codex/images/generations", upstream.lastReq.URL.Path)
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Accept"))
	require.Empty(t, upstream.lastReq.Header.Get("OpenAI-Beta"))
	require.Contains(t, rec.Body.String(), `"success":true`)
	require.NotContains(t, rec.Body.String(), "Responses driver")
}
