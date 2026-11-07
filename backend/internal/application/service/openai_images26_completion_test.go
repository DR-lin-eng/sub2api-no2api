//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGPTImage26APIKeyUsageAcrossEndpoints(t *testing.T) {
	for _, model := range []string{"gpt-image-2.6-flare", "gpt-image-2.6-sunburst"} {
		for _, edit := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/edit=%t/stream=%t", model, edit, stream), func(t *testing.T) {
					body := []byte(fmt.Sprintf(`{"model":%q,"prompt":"draw","stream":%t,"images":[{"image_url":"data:image/png;base64,AA=="}]}`, model, stream))
					c, _ := newOpenAIImagesTestContext(t, body)
					if edit {
						c.Request.URL.Path = openAIImagesEditsEndpoint
					}
					usage := `"usage":{"input_tokens":100,"input_tokens_details":{"image_tokens":80,"cached_tokens_details":{"image_tokens":30,"text_tokens":10}},"output_tokens":20}`
					response := `{"data":[{"b64_json":"AA=="}],` + usage + `}`
					contentType := "application/json"
					if stream {
						contentType = "text/event-stream"
						response = "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"AA==\"," + usage + "}\n\ndata: [DONE]\n\n"
					}
					upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(response))}}
					svc := newOpenAIImagesService(upstream)
					parsed, err := svc.ParseOpenAIImagesRequest(c, body)
					require.NoError(t, err)
					result, err := svc.ForwardImages(context.Background(), c, newOpenAIImagesAPIKeyAccount(), body, parsed, "")
					require.NoError(t, err)
					require.Equal(t, 1, result.ImageCount)
					require.Equal(t, 20, result.Usage.ImageOutputTokens)
					require.Equal(t, 40, result.Usage.CacheReadInputTokens)
					require.Equal(t, 30, result.Usage.ImageCacheReadTokens)
				})
			}
		}
	}
}

func TestGPTImage26RejectsInvalidNativeOptions(t *testing.T) {
	for _, options := range []string{
		`"complexity":"auto"`, `"complexity":3`,
		`"frame_count":1,"output_format":"gif"`,
		`"frame_count":17,"output_format":"gif"`,
		`"frame_duration_ms":19,"output_format":"gif"`,
		`"frame_count":2`, `"svg":{"model":"gpt-6-astra"}`,
		`"output_format":"gif","stream":true`,
		`"output_format":"gif","partial_images":1`,
		`"output_format":"gif","output_compression":80`,
		`"output_format":"svg","n":2`,
		`"output_format":"svg","stream":true`,
		`"output_format":"svg","partial_images":0`,
		`"output_format":"svg","output_compression":80`,
		`"safety_identifier":false`,
		`"safety_identifier":"` + strings.Repeat("x", 129) + `"`,
	} {
		t.Run(options, func(t *testing.T) {
			body := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw",` + options + `}`)
			c, _ := newOpenAIImagesTestContext(t, body)
			_, err := newOpenAIImagesService(nil).ParseOpenAIImagesRequest(c, body)
			require.Error(t, err)
		})
	}
}

func TestGPTImage26NativeSafetyIdentifier(t *testing.T) {
	for _, fields := range []string{`"safety_identifier":"hashed-user","user":"legacy"`, `"safety_identifier":null,"user":"legacy"`} {
		body := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw",` + fields + `}`)
		c, _ := newOpenAIImagesTestContext(t, body)
		parsed, err := newOpenAIImagesService(nil).ParseOpenAIImagesRequest(c, body)
		require.NoError(t, err)
		payload, _, err := buildOpenAIImagesOAuthPayload(parsed, parsed.Model)
		require.NoError(t, err)
		if strings.Contains(fields, "hashed-user") {
			require.Equal(t, "hashed-user", gjson.GetBytes(payload, "safety_identifier").String())
		}
		require.Equal(t, "legacy", gjson.GetBytes(payload, "user").String())
	}
}

func TestGPTImage26SVGAuthorUsageSurvivesSettlementSerialization(t *testing.T) {
	usage, ok := codexDirectImagesUsage([]byte(`{"usage":{"input_tokens":10,"output_tokens":20},"svg_usage":{"model":"gpt-6-astra","input_tokens":100,"output_tokens":50,"reasoning_tokens":30}}`))
	require.True(t, ok)
	encoded, err := json.Marshal(OpenAIForwardResult{Usage: usage})
	require.NoError(t, err)
	require.Equal(t, "gpt-6-astra", gjson.GetBytes(encoded, "Usage.svg_usage.model").String())
	require.EqualValues(t, 100, gjson.GetBytes(encoded, "Usage.svg_usage.input_tokens").Int())
}

func TestGPTImage26SVGRecordUsageChargesBothModelsOnce(t *testing.T) {
	for _, byImage := range []bool{false, true} {
		t.Run(fmt.Sprintf("byImage=%t", byImage), func(t *testing.T) {
			groupID := int64(265)
			group := &Group{ID: groupID, Platform: PlatformOpenAI, RateMultiplier: 2, ImageRateIndependent: true, ImageRateMultiplier: 3}
			apiKey := &APIKey{ID: 2, GroupID: &groupID, Group: group}
			logs := &openAIRecordUsageLogRepoStub{inserted: true}
			billing := &openAIRecordUsageBillingRepoStub{}
			svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(logs, billing, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			svc.billingCacheService = nil
			svc.resolver = newOpenAITokenImageChannelPricingResolverForTest(t, groupID, "gpt-image-2.6-flare")
			if byImage {
				svc.resolver = newOpenAIImageChannelPricingResolverForTest(t, groupID, "gpt-image-2.6-flare", 0.25)
			}
			usage, ok := codexDirectImagesUsage([]byte(`{"usage":{"input_tokens":10,"output_tokens":20},"svg_usage":{"model":"gpt-6-astra","input_tokens":100,"output_tokens":50,"reasoning_tokens":30}}`))
			require.True(t, ok)
			result := &OpenAIForwardResult{RequestID: "image-svg-26", Model: "gpt-image-2.6-flare", Usage: usage, ImageEndpoint: true, ImageCount: 1, ImageSize: "1K"}
			input := &OpenAIRecordUsageInput{Result: result, APIKey: apiKey, User: &User{ID: 1}, Account: &Account{ID: 3}}
			// Preserve the separate model and counters when the input is serialized.
			encoded, err := json.Marshal(input)
			require.NoError(t, err)
			var restored OpenAIRecordUsageInput
			require.NoError(t, json.Unmarshal(encoded, &restored))
			require.NoError(t, svc.RecordUsage(context.Background(), &restored))
			require.Equal(t, 1, billing.calls)
			require.Equal(t, 110, logs.lastLog.InputTokens)
			require.Equal(t, 70, logs.lastLog.OutputTokens)
			require.Equal(t, 20, logs.lastLog.ImageOutputTokens)
			require.Equal(t, reasoningCount(30), logs.lastLog.ReasoningTokens)
			if byImage {
				require.InDelta(t, 0.25, logs.lastLog.TotalCost, 1e-12)
				require.InDelta(t, 0.75, logs.lastLog.ActualCost, 1e-12)
			} else {
				author, err := svc.billingService.CalculateCost("gpt-6-astra", UsageTokens{InputTokens: 100, OutputTokens: 50}, 2)
				require.NoError(t, err)
				// Channel rendering card: $3/M text input and $15/M image output.
				require.InDelta(t, 10*3e-6+20*15e-6+author.TotalCost, logs.lastLog.TotalCost, 1e-12)
				require.InDelta(t, 10*3e-6*2+20*15e-6*3+author.ActualCost, logs.lastLog.ActualCost, 1e-12)
			}
			require.InDelta(t, logs.lastLog.ActualCost, billing.lastCmd.BalanceCost, 1e-8)
			require.Equal(t, 10, restored.Result.Usage.InputTokens, "settling must not mutate raw rendering usage")
		})
	}
}

func TestGPTImage26ExplicitOutputBreakdownAndBoundedCache(t *testing.T) {
	usage, ok := codexDirectImagesUsage([]byte(`{"usage":{"input_tokens":10,"input_tokens_details":{"image_tokens":40,"cached_tokens":50,"cached_tokens_details":{"image_tokens":80}},"output_tokens":20,"output_tokens_details":{"text_tokens":20}}}`))
	require.True(t, ok)
	require.Zero(t, usage.ImageOutputTokens, "explicit text-only output must not be classified as image output")
	require.Equal(t, 10, usage.ImageInputTokens)
	require.Equal(t, 10, usage.CacheReadInputTokens)
	require.Equal(t, 10, usage.ImageCacheReadTokens)
}

func TestImagesUsagePreservesCompatibleResponseWrappers(t *testing.T) {
	for _, body := range []string{
		`{"response":{"usage":{"input_tokens":100,"output_tokens":20,"output_tokens_details":{"image_tokens":12}}}}`,
		`{"data":{"usage":{"input_tokens":100,"output_tokens":20,"output_tokens_details":{"image_tokens":12}}}}`,
	} {
		usage, ok := codexDirectImagesUsage([]byte(body))
		require.True(t, ok)
		require.Equal(t, 100, usage.InputTokens)
		require.Equal(t, 20, usage.OutputTokens)
		require.Equal(t, 12, usage.ImageOutputTokens)
	}
}
