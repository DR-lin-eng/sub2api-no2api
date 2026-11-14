//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
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

func image26AnimatedGIF(t *testing.T, frames int) string {
	t.Helper()
	animation := &gif.GIF{}
	for i := 0; i < frames; i++ {
		frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
		frame.SetColorIndex(i%2, 0, 1)
		animation.Image = append(animation.Image, frame)
		animation.Delay = append(animation.Delay, 20)
	}
	var body bytes.Buffer
	require.NoError(t, gif.EncodeAll(&body, animation))
	return base64.StdEncoding.EncodeToString(body.Bytes())
}

func TestGPTImage26GIFFrameCountBilling(t *testing.T) {
	requestFrames := 4
	parsed := &OpenAIImagesRequest{OutputFormat: "gif", FrameCount: &requestFrames}
	for _, tc := range []struct {
		name    string
		body    string
		parsed  *OpenAIImagesRequest
		assets  int
		enabled bool
		want    int
	}{
		{"actual frames beat request", fmt.Sprintf(`{"data":[{"b64_json":%q}]}`, image26AnimatedGIF(t, 3)), parsed, 1, true, 3},
		{"URL representation preserves actual frames", fmt.Sprintf(`{"frame_count":4,"data":[{"url":%q}]}`, "data:image/gif;base64,"+image26AnimatedGIF(t, 3)), parsed, 1, true, 3},
		{"two animations", fmt.Sprintf(`{"data":[{"b64_json":%q},{"b64_json":%q}]}`, image26AnimatedGIF(t, 4), image26AnimatedGIF(t, 4)), parsed, 2, true, 8},
		{"default nine frames", `{"data":[{"url":"https://example.test/a.gif"}]}`, &OpenAIImagesRequest{OutputFormat: "gif"}, 1, true, 9},
		{"returned metadata", `{"frame_count":6,"data":[{"url":"https://example.test/a.gif"}]}`, parsed, 1, true, 6},
		{"per result metadata", `{"frame_count":6,"data":[{"url":"a.gif","frame_count":2},{"url":"b.gif","frame_count":3}]}`, parsed, 2, true, 5},
		{"switch off", fmt.Sprintf(`{"data":[{"b64_json":%q}]}`, image26AnimatedGIF(t, 4)), parsed, 1, false, 1},
		{"SVG stays one", `{"data":[{"b64_json":"PHN2Zy8+"}]}`, &OpenAIImagesRequest{OutputFormat: "svg"}, 1, true, 1},
		{"no output", `{"data":[]}`, parsed, 0, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count, sizes := openAIImagesBillableOutputs([]byte(tc.body), tc.parsed, tc.assets, []string{"1024x1024", "3840x2160"}[:tc.assets], tc.enabled)
			require.Equal(t, tc.want, count)
			if tc.assets > 0 {
				require.Len(t, sizes, tc.want)
			}
		})
	}
}

func TestGPTImage26GIFAndSVGSettlement(t *testing.T) {
	for _, format := range []string{"gif", "svg"} {
		t.Run(format, func(t *testing.T) {
			gid := int64(266)
			price := 0.25
			group := &Group{ID: gid, Platform: PlatformOpenAI, RateMultiplier: 2, ImageCountBilling: true, ImagePrice1K: &price}
			apiKey := &APIKey{ID: 2, GroupID: &gid, Group: group}
			logs := &openAIRecordUsageLogRepoStub{inserted: true}
			billing := &openAIRecordUsageBillingRepoStub{}
			svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(logs, billing, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			svc.billingCacheService = nil
			result := &OpenAIForwardResult{RequestID: "formats-26-" + format, Model: "gpt-image-2.6-flare", ImageEndpoint: true, ImageCount: 1, ImageSize: "1K", Usage: OpenAIUsage{InputTokens: 10, OutputTokens: 20, ImageOutputTokens: 20}}
			if format == "gif" {
				result.ImageCount = 4
			} else {
				result.Usage.SVGUsage = &OpenAIImagesSVGUsage{Model: "gpt-6-astra", InputTokens: 100, OutputTokens: 50}
			}
			require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{Result: result, APIKey: apiKey, User: &User{ID: 1}, Account: &Account{ID: 3}}))
			require.InDelta(t, float64(result.ImageCount)*price, logs.lastLog.TotalCost, 1e-12)
			require.InDelta(t, float64(result.ImageCount)*price*2, billing.lastCmd.BalanceCost, 1e-8)
			require.Equal(t, result.ImageCount, logs.lastLog.ImageCount)
			require.Equal(t, 1, billing.calls, "one reliable settlement per generated work")
		})
	}
}

func TestAccountGPTImage26FormatsVerifyActualFiles(t *testing.T) {
	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
		for _, format := range []string{"svg", "gif"} {
			t.Run(accountType+"/"+format, func(t *testing.T) {
				encoded := base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0L1 1"/></svg>`))
				if format == "gif" {
					encoded = image26AnimatedGIF(t, 4)
				}
				response, _ := json.Marshal(map[string]any{"data": []any{map[string]any{"b64_json": encoded}}})
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(response))}}
				svc := &AccountTestService{httpUpstream: upstream, cfg: &config.Config{}}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				frames := 4
				ctx := WithAccountImageTestOptions(t.Context(), AccountImageTestOptions{OutputFormat: format})
				if format == "gif" {
					ctx = WithAccountImageTestOptions(t.Context(), AccountImageTestOptions{OutputFormat: format, FrameCount: &frames})
				}
				c.Request = httptest.NewRequest("POST", "/test", nil).WithContext(ctx)
				a := &Account{ID: 1, Platform: PlatformOpenAI, Type: accountType, Credentials: map[string]any{"api_key": "fixture", "access_token": "fixture", "base_url": "https://example.test/v1"}}
				var err error
				if accountType == AccountTypeAPIKey {
					err = svc.testOpenAIImageAPIKey(c, ctx, a, "gpt-image-2.6-flare", "draw")
				} else {
					err = svc.testOpenAIImageOAuth(c, ctx, a, "gpt-image-2.6-flare", "draw")
				}
				require.NoError(t, err)
				payload, _ := io.ReadAll(upstream.lastReq.Body)
				require.Equal(t, format, gjson.GetBytes(payload, "output_format").String())
				require.Contains(t, rec.Body.String(), "data:"+openAIImageOutputMIMEType(format)+";base64,")
				require.Contains(t, rec.Body.String(), `"success":true`)
			})
		}
	}
	require.Error(t, validateAccountImageTestOutput(base64.StdEncoding.EncodeToString([]byte("not SVG")), "svg"))
	require.Error(t, validateAccountImageTestOutput(base64.StdEncoding.EncodeToString([]byte("not GIF")), "gif"))
	require.Error(t, validateAccountImageTestOutput(image26AnimatedGIF(t, 1), "gif"))
	gifBytes, err := base64.StdEncoding.DecodeString(image26AnimatedGIF(t, 4))
	require.NoError(t, err)
	_, err = openAIImagesGIFFrames(base64.StdEncoding.EncodeToString(gifBytes[:len(gifBytes)-1]))
	require.Error(t, err, "a truncated animation must not be confirmed")
}

func TestGPTImage26EmptyAdvancedOutputIsNotBillable(t *testing.T) {
	for _, format := range []string{"gif", "svg"} {
		t.Run(format, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
			upstream := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"data":[]}`))}
			_, count, _, err := (&OpenAIGatewayService{cfg: &config.Config{}}).handleOpenAIImagesNonStreamingResponse(upstream, c, &OpenAIImagesRequest{OutputFormat: format, N: 1})
			require.Error(t, err)
			require.Zero(t, count)
			require.Empty(t, rec.Body.String(), "empty success responses must not be committed or charged")
		})
	}
}

func TestGPTImage26BillingBoundsUnvalidatedCompatibleFrameOptions(t *testing.T) {
	frames := int(^uint(0) >> 1)
	count, sizes := openAIImagesBillableOutputs([]byte(`{"data":[{"url":"https://example.test/result.gif"}]}`), &OpenAIImagesRequest{Model: "compatible-image-alias", OutputFormat: "gif", FrameCount: &frames}, 1, []string{"1024x1024"}, true)
	require.Equal(t, defaultOpenAIImagesGIFFrameCount, count)
	require.Len(t, sizes, defaultOpenAIImagesGIFFrameCount)
}
