package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIWSCompressionTestingT interface {
	Helper()
	Errorf(format string, args ...any)
	FailNow()
}

func openAIWSCompressionPNG(t openAIWSCompressionTestingT, width, height int, transparent bool) string {
	t.Helper()
	src := image.NewNRGBA(image.Rect(0, 0, width, height))
	seed := uint32(0x9e3779b9)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			seed = seed*1664525 + 1013904223
			alpha := uint8(255)
			if transparent && (x+y)%3 == 0 {
				alpha = 96
			}
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(seed >> 24), G: uint8(seed >> 16), B: uint8(seed >> 8), A: alpha})
		}
	}
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, src))
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
}

func openAIWSCompressionConfig() config.OpenAIWSImageCompressionConfig {
	return config.OpenAIWSImageCompressionConfig{
		Enabled:                  true,
		Mode:                     "adaptive",
		TargetRequestBytes:       128 * 1024,
		MinRequestBytes:          1,
		MinImageBytes:            1,
		MaxEdgePixels:            512,
		MaxDecodedPixelsPerImage: 4 * 1024 * 1024,
		MaxTotalDecodedPixels:    8 * 1024 * 1024,
		MaxImagesPerRequest:      16,
		MaxTotalEncodeMS:         5000,
		QualityLadder:            []int{85, 78, 70},
	}
}

func TestPrepareOpenAIWSImagePayloadCompressesInlinePNG(t *testing.T) {
	imageURL := openAIWSCompressionPNG(t, 1024, 1024, false)
	body := []byte(`{"type":"response.create","model":"gpt-5.6-sol","input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"` + imageURL + `"}]}]}`)
	cfg := openAIWSCompressionConfig()
	prepared, result := prepareOpenAIWSImagePayload(body, cfg, true, 0)
	require.True(t, result.Changed)
	require.Less(t, len(prepared), len(body))
	require.Equal(t, "data:image/jpeg;base64", gjsonGetPrefix(prepared))
}

func TestPrepareOpenAIWSImagePayloadSkipsSmallRequest(t *testing.T) {
	body := []byte(`{"input":[{"type":"input_text","text":"small"}]}`)
	cfg := openAIWSCompressionConfig()
	cfg.MinRequestBytes = 1024
	prepared, result := prepareOpenAIWSImagePayload(body, cfg, true, 0)
	require.False(t, result.Changed)
	require.Equal(t, body, prepared)
}

func TestPrepareOpenAIWSImagePayloadPreservesAlphaWhenConfigured(t *testing.T) {
	imageURL := openAIWSCompressionPNG(t, 256, 256, true)
	body := []byte(`{"input":[{"content":[{"type":"input_image","image_url":"` + imageURL + `"}]}]}`)
	cfg := openAIWSCompressionConfig()
	cfg.PreserveAlpha = true
	prepared, result := prepareOpenAIWSImagePayload(body, cfg, true, 0)
	require.False(t, result.Changed)
	require.Equal(t, body, prepared)
}

func TestPrepareOpenAIWSImagePayloadHandlesStringifiedToolOutput(t *testing.T) {
	imageURL := openAIWSCompressionPNG(t, 512, 512, false)
	nested := `[{"type":"input_image","image_url":"` + imageURL + `"}]`
	body := []byte(`{"input":[{"type":"function_call_output","call_id":"call_1","output":` + mustJSONQuoted(nested) + `}]}`)
	cfg := openAIWSCompressionConfig()
	prepared, result := prepareOpenAIWSImagePayload(body, cfg, true, 0)
	require.True(t, result.Changed)
	require.NotEqual(t, string(body), string(prepared))
	require.Contains(t, string(prepared), `data:image/jpeg;base64`)
}

func TestOpenAIWSRoutingUsesHardLimitAfterImagePolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
	account := &Account{ID: 99, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	for _, tc := range []struct {
		name  string
		limit int64
		want  bool
	}{
		{name: "32 MiB", limit: 32 * 1024 * 1024, want: true},
		{name: "64 MiB", limit: 64 * 1024 * 1024, want: false},
		{name: "128 MiB", limit: 128 * 1024 * 1024, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc.cfg.Gateway.OpenAIWS.WSMaxRequestBytes = tc.limit
			body := []byte(`{"input":"` + strings.Repeat("x", 40*1024*1024) + `"}`)
			require.Equal(t, tc.want, svc.shouldBypassOpenAIWSForSize(context.Background(), c, account, body, body))
		})
	}
}

func TestOpenAIWSApiKeyBudgetIsConfigurableAndOAuthIsFixed(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAIWS.WSMaxRequestBytes = 100 * 1024 * 1024
	svc.cfg.Gateway.OpenAIWS.APIKeyWSMaxRequestBytes = 32 * 1024 * 1024
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.Equal(t, int64(32*1024*1024), svc.openAIWSHTTPBridgeThresholdBytesForAccount(apiKey))
	require.Equal(t, int64(128*1024*1024), svc.openAIWSHTTPBridgeThresholdBytesForAccount(oauth))
	require.Equal(t, int64(32*1024*1024), resolveOpenAIWSMessageReadLimitForAccount(svc.cfg, apiKey))
	require.Equal(t, int64(128*1024*1024), resolveOpenAIWSMessageReadLimitForAccount(svc.cfg, oauth))
}

func TestOpenAIWSImageCompressionApiKeySwitch(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAIWS.ImageCompression = openAIWSCompressionConfig()
	svc.cfg.Gateway.OpenAIWS.ImageCompression.APIKeyEnabled = false
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	imageURL := openAIWSCompressionPNG(t, 1024, 1024, false)
	body := []byte(`{"input":[{"content":[{"type":"input_image","image_url":"` + imageURL + `"}]}]}`)
	apiPrepared, apiResult := svc.prepareOpenAIWSImagePayloadForAccount(apiKey, body)
	require.False(t, apiResult.Changed)
	require.Equal(t, body, apiPrepared)
	oauthPrepared, oauthResult := svc.prepareOpenAIWSImagePayloadForAccount(oauth, body)
	require.True(t, oauthResult.Changed)
	require.Less(t, len(oauthPrepared), len(body))
}

func BenchmarkPrepareOpenAIWSImagePayload(b *testing.B) {
	imageURL := openAIWSCompressionPNG(b, 1024, 1024, false)
	body := []byte(`{"type":"response.create","model":"gpt-5.6-sol","input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"` + imageURL + `"}]}]}`)
	cfg := openAIWSCompressionConfig()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = prepareOpenAIWSImagePayload(body, cfg, true, 0)
	}
}

func mustJSONQuoted(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func gjsonGetPrefix(body []byte) string {
	needle := []byte(`"image_url":"`)
	index := bytes.Index(body, needle)
	if index < 0 {
		return ""
	}
	start := index + len(needle)
	end := bytes.IndexByte(body[start:], '"')
	if end < 0 {
		return ""
	}
	value := string(body[start : start+end])
	return strings.Split(value, ",")[0]
}
