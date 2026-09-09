//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIImagesResponsesDriverOverrideKeepsImageToolModel(t *testing.T) {
	for _, override := range []string{"", "  ", " gpt-5.6-sol "} {
		t.Run(override, func(t *testing.T) {
			t.Setenv("SUB2API_IMAGES_MAIN_MODEL", override)
			wantDriver := openAIImagesResponsesMainModel
			if strings.TrimSpace(override) != "" {
				wantDriver = strings.TrimSpace(override)
			}
			parsed := &OpenAIImagesRequest{Endpoint: openAIImagesGenerationsEndpoint, Model: "gpt-image-2.5-flare", Prompt: "draw a cup"}
			body, err := buildOpenAIImagesResponsesRequest(parsed, parsed.Model)
			require.NoError(t, err)
			require.Equal(t, wantDriver, gjson.GetBytes(body, "model").String())
			require.Equal(t, "gpt-image-2.5-flare", gjson.GetBytes(body, "tools.0.model").String())
		})
	}
}

func TestNormalizeOpenAIResponsesImageOnlyModelUsesConfiguredDriver(t *testing.T) {
	t.Setenv("SUB2API_IMAGES_MAIN_MODEL", "gpt-5.6-sol")
	body := map[string]any{"model": "gpt-image-2.5-sunburst", "input": "draw"}
	require.True(t, normalizeOpenAIResponsesImageOnlyModel(body))
	require.Equal(t, "gpt-5.6-sol", body["model"])
	require.Equal(t, "gpt-image-2.5-sunburst", body["tools"].([]any)[0].(map[string]any)["model"])
}

func TestOpenAIImagesToolUsagePreservesBoundedImageInputTokens(t *testing.T) {
	var usage OpenAIUsage
	svc := &OpenAIGatewayService{}
	svc.parseOpenAIImagesSSEUsageBytes([]byte(`{"type":"response.completed","response":{"tool_usage":{"image_gen":{"input_tokens":1550,"input_tokens_details":{"image_tokens":1521},"output_tokens":515,"output_tokens_details":{"image_tokens":515}}}}}`), &usage)
	require.Equal(t, 1550, usage.InputTokens)
	require.Equal(t, 1521, usage.ImageInputTokens)
	require.Equal(t, 515, usage.ImageOutputTokens)

	usage = OpenAIUsage{}
	svc.parseOpenAIImagesSSEUsageBytes([]byte(`{"type":"response.completed","response":{"tool_usage":{"image_gen":{"input_tokens":10,"input_tokens_details":{"image_tokens":99},"output_tokens":1,"output_tokens_details":{"image_tokens":1}}}}}`), &usage)
	require.Equal(t, 10, usage.ImageInputTokens)
}

func TestGPTImage25FallbackPricingIsDistinctFromGPTImage2(t *testing.T) {
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gpt-image-2": {InputCostPerToken: 5e-6, InputCostPerImageToken: 8e-6, OutputCostPerImageToken: 3e-5},
	}}
	pricing := svc.GetModelPricing("gpt-image-2.5-flare-2026-09-08")
	require.NotNil(t, pricing)
	require.Equal(t, 5e-6, pricing.InputCostPerToken)
	require.Equal(t, 8e-6, pricing.InputCostPerImageToken)
	require.Equal(t, 3e-5, pricing.OutputCostPerImageToken)
	custom := &LiteLLMModelPricing{InputCostPerToken: 7e-6}
	svc.pricingData["gpt-image-2.5-flare-2026-09-08"] = custom
	require.Same(t, custom, svc.GetModelPricing("gpt-image-2.5-flare-2026-09-08"))
}

func TestOpenAIImagesRetiredDriverErrorDoesNotCooldownImageModel(t *testing.T) {
	t.Setenv("SUB2API_IMAGES_MAIN_MODEL", "gpt-5.4-mini")
	repo := &modelNotFoundAccountRepoStub{}
	svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"The 'gpt-5.4-mini' model is not supported when using Codex with a ChatGPT account."}}`)),
	}
	account := &Account{ID: 77, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	_, err := svc.handleOpenAIImagesErrorResponse(context.Background(), resp, c, account, "gpt-image-2.5-flare")
	require.Error(t, err)
	require.Empty(t, repo.modelRateLimitCalls)
	require.Empty(t, repo.tempCalls)
	require.Contains(t, rec.Body.String(), "gpt-5.4-mini")
}
