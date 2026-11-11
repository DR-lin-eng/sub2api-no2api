//go:build unit

package service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/gif"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func chatGPTGIFWithDelay(t *testing.T, frames, delayMS int) string {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(image26AnimatedGIF(t, frames))
	require.NoError(t, err)
	animation, err := gif.DecodeAll(bytes.NewReader(data))
	require.NoError(t, err)
	for i := range animation.Delay {
		animation.Delay[i] = delayMS / 10
	}
	var output bytes.Buffer
	require.NoError(t, gif.EncodeAll(&output, animation))
	return base64.StdEncoding.EncodeToString(output.Bytes())
}

func TestChatGPTAppGIFStandardAPIAndAccountProbeShareProtocol(t *testing.T) {
	for _, model := range []string{"gpt-image-2.6-flare", "gpt-image-2.6-sunburst"} {
		t.Run(model, func(t *testing.T) {
			encoded := image26AnimatedGIF(t, 4)
			gateway, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/backend-api/f/conversation", r.URL.Path)
				chatGPTAppImageStream(w, "data:image/gif;base64,"+encoded)
			})
			frames, delay := 4, 200
			options := AccountImageTestOptions{OutputFormat: "gif", FrameCount: &frames, FrameDurationMS: &delay}
			probe := &AccountTestService{settingService: &SettingService{}, oauthGatewayLimiter: gateway, cfg: &config.Config{}}
			c := newOpenAIImagesContext(t, []byte(`{}`))
			ctx := WithAccountImageTestOptions(t.Context(), options)
			require.NoError(t, probe.testOpenAIImageOAuth(c, ctx, chatGPTAppImageTestAccount(), model, "draw a rotating logo"))
			probePrompt := gjson.GetBytes(upstream.imageCalls()[0].body, "messages.0.content.parts.0").String()

			// Reproduce the reported button: API-key account -> second gateway -> OAuth.
			probe.httpUpstream = imagesChainUpstream{t: t, gateway: gateway}
			account := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://pool.example/v1"}}
			c, rec := newOpenAIImagesTestContext(t, nil)
			require.NoError(t, probe.testOpenAIImageAPIKey(c, ctx, account, model, "draw a rotating logo"))
			require.Contains(t, rec.Body.String(), `"success":true`)
			require.Contains(t, rec.Body.String(), "data:image/gif;base64,"+encoded)
			require.Len(t, upstream.imageCalls(), 2)
			require.Equal(t, probePrompt, gjson.GetBytes(upstream.imageCalls()[1].body, "messages.0.content.parts.0").String())
			require.Contains(t, probePrompt, "exactly 4 frames and 200 milliseconds")
			for _, call := range upstream.imageCalls() {
				for _, field := range []string{"output_format", "frame_count", "frame_duration_ms", "animation", "requestedMimeType"} {
					require.False(t, gjson.GetBytes(call.body, field).Exists())
				}
			}
			require.NotContains(t, rec.Body.String(), "legacy")
			require.NotContains(t, rec.Body.String(), "fallback")
		})
	}
}

func TestChatGPTAppGIFDefaultsAndActualFrameBilling(t *testing.T) {
	for _, byFrame := range []bool{false, true} {
		for _, format := range []string{"b64_json", "url"} {
			t.Run(fmt.Sprintf("frames=%t/%s", byFrame, format), func(t *testing.T) {
				encoded := image26AnimatedGIF(t, 9)
				svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
					chatGPTAppImageStream(w, "data:image/gif;base64,"+encoded)
				})
				body := fmt.Appendf(nil, `{"model":"gpt-image-2.6-flare","prompt":"draw","output_format":"gif","size":"auto","n":2,"response_format":%q}`, format)
				c, rec := newOpenAIImagesTestContext(t, body)
				c.Set("api_key", &APIKey{Group: &Group{ImageCountBilling: byFrame}})
				parsed, err := svc.ParseOpenAIImagesRequest(c, body)
				require.NoError(t, err)
				result, err := svc.ForwardImages(t.Context(), c, chatGPTAppImageTestAccount(), body, parsed, "")
				require.NoError(t, err)
				units := 2
				if byFrame {
					units = 18
				}
				require.Equal(t, units, result.ImageCount)
				require.Len(t, result.ImageOutputSizes, units)
				require.Len(t, upstream.imageCalls(), 2)
				require.Len(t, gjson.GetBytes(rec.Body.Bytes(), "data").Array(), 2)
				if format == "url" {
					require.Equal(t, "data:image/gif;base64,"+encoded, gjson.GetBytes(rec.Body.Bytes(), "data.0.url").String())
				} else {
					require.Equal(t, encoded, gjson.GetBytes(rec.Body.Bytes(), "data.0.b64_json").String())
				}
			})
		}
	}
}

func TestChatGPTAppGIFRejectsWrongFileOrControlsWithoutRegeneration(t *testing.T) {
	for _, tc := range []struct {
		name, encoded, format, want string
		units                       int
	}{
		{"PNG", encodeOpenAIImageTestPNG(t, 2, 2), "png", "did not return an animated GIF", 1},
		{"wrong frames", image26AnimatedGIF(t, 3), "gif", "contains 3 frames", 3},
		{"wrong timing", chatGPTGIFWithDelay(t, 4, 100), "gif", "100 ms delay", 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
				chatGPTAppImageStream(w, "data:image/"+tc.format+";base64,"+tc.encoded)
			})
			body := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw","output_format":"gif","frame_count":4}`)
			c, rec := newOpenAIImagesTestContext(t, body)
			c.Set("api_key", &APIKey{Group: &Group{ImageCountBilling: true}})
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			result, err := svc.ForwardImages(t.Context(), c, chatGPTAppImageTestAccount(), body, parsed, "")
			require.ErrorContains(t, err, tc.want)
			require.NotNil(t, result, "completed upstream work must retain settlement")
			require.Equal(t, tc.units, result.ImageCount)
			require.NotContains(t, rec.Body.String(), `"b64_json"`)
			require.NotContains(t, rec.Body.String(), "fallback")
			require.Len(t, upstream.imageCalls(), 1)
		})
	}
}

func TestChatGPTAppGIFRequestTimingRoundingAndNativeBoundaries(t *testing.T) {
	for _, tc := range []struct{ requested, rounded int }{{20, 20}, {24, 20}, {25, 30}, {200, 200}, {2001, 2000}, {655351, 655350}, {int(^uint(0) >> 1), 655350}} {
		delay := tc.requested
		parsed := &OpenAIImagesRequest{OutputFormat: "gif", FrameDurationMS: &delay}
		_, rounded := chatGPTAppGIFOptions(parsed)
		require.Equal(t, tc.rounded, rounded)
		encoded := chatGPTGIFWithDelay(t, 9, rounded)
		require.NoError(t, validateChatGPTAppImageOutput(parsed, openAIResponsesImageResult{OutputFormat: "gif", Result: encoded}))
	}
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw","output_format":"svg","svg":{"model":"gpt-6-astra"}}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, openAIImagesGenerationsEndpoint, strings.NewReader(string(body)))
	parsed, err := newOpenAIImagesService(nil).ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	require.True(t, openAIImagesRequiresNativeProtocol(parsed))
}

func TestChatGPTAppGIFStandardAccountsRemainNative(t *testing.T) {
	for _, kind := range []string{"apikey", "siwc"} {
		for _, edit := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/edit=%t", kind, edit), func(t *testing.T) {
				body := []byte(`{"model":"gpt-image-2.6-sunburst","prompt":"animate this","output_format":"gif","size":"auto","frame_count":4,"frame_duration_ms":200}`)
				if edit {
					body = []byte(strings.TrimSuffix(string(body), "}") + `,"images":[{"image_url":"data:image/png;base64,AA=="}]}`)
				}
				encoded := image26AnimatedGIF(t, 4)
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"data":[{"b64_json":%q}]}`, encoded)))}}
				svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
				account := newOpenAIImagesAPIKeyAccount()
				account.Credentials["base_url"] = "https://api.openai.com/v1"
				if kind == "siwc" {
					account.Type = AccountTypeOAuth
					account.Credentials = map[string]any{"auth_mode": "siwc", "access_token": "fixture-token"}
				}
				c, _ := newOpenAIImagesTestContext(t, body)
				if edit {
					c.Request.URL.Path = openAIImagesEditsEndpoint
				}
				parsed, err := svc.ParseOpenAIImagesRequest(c, body)
				require.NoError(t, err)
				_, err = svc.ForwardImages(t.Context(), c, account, body, parsed, "")
				require.NoError(t, err)
				require.JSONEq(t, string(body), string(upstream.lastBody))
				require.Equal(t, "https://api.openai.com"+c.Request.URL.Path, upstream.lastReq.URL.String())
				require.Len(t, upstream.requests, 1)
				require.Empty(t, upstream.lastReq.Header.Get("originator"))
			})
		}
	}
}

func TestChatGPTAppGIFExplicitLegacyModeRetainsNativeRequest(t *testing.T) {
	encoded := image26AnimatedGIF(t, 4)
	svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/codex/images/generations", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, encoded)
	})
	enableLegacyOpenAIImagesForTest(svc)
	body := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw","output_format":"gif","frame_count":4}`)
	c, _ := newOpenAIImagesTestContext(t, body)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	_, err = svc.ForwardImages(t.Context(), c, chatGPTAppImageTestAccount(), body, parsed, "")
	require.NoError(t, err)
	require.Len(t, upstream.imageCalls(), 1)
	require.Equal(t, "gif", gjson.GetBytes(upstream.imageCalls()[0].body, "output_format").String())
	require.EqualValues(t, 4, gjson.GetBytes(upstream.imageCalls()[0].body, "frame_count").Int())
}
