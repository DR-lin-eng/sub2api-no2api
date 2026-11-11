//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestImagesProtocolStandardAccountsNeverUseApp(t *testing.T) {
	for _, kind := range []string{"apikey", "siwc"} {
		for _, model := range []string{"gpt-image-2", "gpt-image-2.6-sunburst"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", kind, model, stream), func(t *testing.T) {
					body := fmt.Appendf(nil, `{"model":%q,"prompt":"draw","size":"1024x1024","quality":"high","output_format":"png","stream":%t}`, model, stream)
					response := `{"data":[{"b64_json":"aGVsbG8="}],"usage":{"input_tokens":10,"output_tokens":20}}`
					contentType := "application/json"
					if stream {
						contentType = "text/event-stream"
						response = "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"aGVsbG8=\"}\n\ndata: [DONE]\n\n"
					}
					upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(response))}}
					svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, settingService: &SettingService{}}
					account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://native.example/v1"}}
					wantURL := "https://native.example/v1/images/generations"
					if kind == "siwc" {
						account.Type = AccountTypeOAuth
						account.Credentials = map[string]any{"access_token": "fixture-token", "auth_mode": "siwc", "chatgpt_account_id": "must-not-be-forwarded"}
						wantURL = "https://api.openai.com/v1/images/generations"
					}
					c := newOpenAIImagesContext(t, body)
					parsed, err := svc.ParseOpenAIImagesRequest(c, body)
					require.NoError(t, err)
					result, err := svc.ForwardImages(t.Context(), c, account, body, parsed, "")
					require.NoError(t, err)
					require.Equal(t, wantURL, upstream.lastReq.URL.String())
					require.Len(t, upstream.requests, 1)
					require.JSONEq(t, string(body), string(upstream.lastBody), "native parameters must not be stripped or converted into conversation text")
					require.Equal(t, stream, result.Stream)
					require.Empty(t, upstream.lastReq.Header.Get("originator"))
					require.Empty(t, upstream.lastReq.Header.Get("chatgpt-account-id"))
					require.Empty(t, upstream.lastReq.Header.Get("OpenAI-Sentinel-Chat-Requirements-Token"))
				})
			}
		}
	}
}

func TestImagesProtocolOAuthNativeParametersKeepOpenAIContract(t *testing.T) {
	for _, fields := range []string{
		`"size":"1024x1024"`, `"quality":"high"`, `"complexity":"low"`,
		`"output_format":"png"`,
		`"output_format":"svg","svg":{"model":"gpt-6-astra","reasoning":{"effort":"high"}}`,
		`"background":"transparent"`, `"moderation":"low"`, `"safety_identifier":"hashed-user"`,
	} {
		t.Run(fields, func(t *testing.T) {
			body := []byte(`{"model":"gpt-image-2.6-sunburst","prompt":"a logo",` + fields + `}`)
			encoded := encodeOpenAIImageTestPNG(t, 3, 3)
			if strings.Contains(fields, `"svg"`) {
				encoded = base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0L1 1"/></svg>`))
			}
			if strings.Contains(fields, `"gif"`) {
				encoded = image26AnimatedGIF(t, 4)
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"data":[{"b64_json":%q}]}`, encoded)))}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, settingService: &SettingService{}}
			c := newOpenAIImagesContext(t, body)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			_, err = svc.ForwardImages(t.Context(), c, chatGPTAppImageTestAccount(), body, parsed, "")
			require.NoError(t, err)
			require.Len(t, upstream.requests, 1, "no conversation request or failed first generation")
			require.Equal(t, "/backend-api/codex/images/generations", upstream.lastReq.URL.Path)
			gjson.ParseBytes([]byte(`{` + fields + `}`)).ForEach(func(k, v gjson.Result) bool {
				require.JSONEq(t, v.Raw, gjson.GetBytes(upstream.lastBody, k.Str).Raw)
				return true
			})
			require.NotContains(t, c.Writer.Header().Get("Content-Type"), "text/event-stream")
			require.NotContains(t, string(upstream.lastBody), "fallback")
		})
	}
}

func TestImagesSIWCAccountProbeUsesPublicImagesEndpoint(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"data":[{"b64_json":"aGVsbG8="}]}`))}}
	svc := &AccountTestService{cfg: &config.Config{}, httpUpstream: upstream, settingService: &SettingService{}}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"auth_mode": "siwc", "access_token": "fixture-token"}}
	c := newOpenAIImagesContext(t, []byte(`{}`))
	require.NoError(t, svc.testOpenAIImageOAuth(c, t.Context(), account, "gpt-image-2", "draw"))
	require.Equal(t, "https://api.openai.com/v1/images/generations", upstream.lastReq.URL.String())
	require.Empty(t, upstream.lastReq.Header.Get("originator"))
	require.Empty(t, upstream.lastReq.Header.Get("chatgpt-account-id"))
	for _, key := range []string{"system_hints", "messages", "supported_encodings"} {
		require.False(t, gjson.GetBytes(upstream.lastBody, key).Exists())
	}
}

func TestImagesProtocolOAuthNativeStreaming(t *testing.T) {
	body := []byte(`{"model":"gpt-image-2.6-sunburst","prompt":"draw","stream":true,"size":"1024x1024","quality":"high","output_format":"png"}`)
	encoded := encodeOpenAIImageTestPNG(t, 3, 3)
	response := fmt.Sprintf("data: {\"type\":\"image_generation.completed\",\"b64_json\":%q,\"output_format\":\"png\"}\n\ndata: [DONE]\n\n", encoded)
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(response))}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, settingService: &SettingService{}}
	c, rec := newOpenAIImagesTestContext(t, body)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	result, err := svc.ForwardImages(t.Context(), c, chatGPTAppImageTestAccount(), body, parsed, "")
	require.NoError(t, err)
	require.True(t, result.Stream)
	require.Equal(t, 1, result.ImageCount)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "/backend-api/codex/images/generations", upstream.lastReq.URL.Path)
	require.Equal(t, "png", gjson.GetBytes(upstream.lastBody, "output_format").String())
	require.Equal(t, "1024x1024", gjson.GetBytes(upstream.lastBody, "size").String())
	require.Contains(t, rec.Body.String(), encoded)
	require.NotContains(t, rec.Body.String(), "legacy")
	require.NotContains(t, rec.Body.String(), "fallback")
}

type imagesChainUpstream struct {
	HTTPUpstream
	t       *testing.T
	gateway *OpenAIGatewayService
}

func (u imagesChainUpstream) DoWithTLS(req *http.Request, proxy string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, accountID, concurrency)
}

func (u imagesChainUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	require.NoError(u.t, err)
	require.Equal(u.t, "/v1/images/generations", req.URL.Path)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, req.URL.Path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	parsed, err := u.gateway.ParseOpenAIImagesRequest(c, body)
	require.NoError(u.t, err)
	_, err = u.gateway.ForwardImages(context.Background(), c, chatGPTAppImageTestAccount(), body, parsed, "")
	require.NoError(u.t, err)
	return rec.Result(), nil
}

func TestImagesAPIKeyProbeAcrossDownstreamGateway(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"data":[{"b64_json":"aGVsbG8="}]}`))}}
	gateway := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, settingService: &SettingService{}}
	probe := &AccountTestService{cfg: &config.Config{}, httpUpstream: imagesChainUpstream{t: t, gateway: gateway}}
	account := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key", "base_url": "https://downstream.example/v1"}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/test", nil)
	require.NoError(t, probe.testOpenAIImageAPIKey(c, t.Context(), account, "gpt-image-2.6-sunburst", "draw"))
	require.Contains(t, rec.Body.String(), `"success":true`)
	require.NotContains(t, rec.Body.String(), "requires legacy")
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "/backend-api/codex/images/generations", upstream.lastReq.URL.Path)
	require.Equal(t, "1024x1024", gjson.GetBytes(upstream.lastBody, "size").String())
}

func TestImagesSVGEncodingAcrossAccountProbeAndAPI(t *testing.T) {
	raw := `<svg xmlns="http://www.w3.org/2000/svg"><text>图像</text></svg>`
	for _, value := range []string{raw, "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(raw)), base64.RawURLEncoding.EncodeToString([]byte(raw))} {
		for _, kind := range []string{"probe", "oauth-api", "apikey-api"} {
			t.Run(kind+"/"+value[:8], func(t *testing.T) {
				body := []byte(`{"model":"gpt-image-2.6-flare","prompt":"a logo","output_format":"svg","svg":{"model":"gpt-6-astra"}}`)
				response := fmt.Sprintf(`{"data":[{"b64_json":%q}],"provider_metadata":{"keep":true},"svg_usage":{"model":"gpt-6-astra","input_tokens":50,"output_tokens":20}}`, value)
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				account := chatGPTAppImageTestAccount()
				if kind == "probe" {
					ctx := WithAccountImageTestOptions(t.Context(), AccountImageTestOptions{OutputFormat: "svg"})
					c.Request = c.Request.WithContext(ctx)
					probe := &AccountTestService{cfg: &config.Config{}, httpUpstream: upstream, settingService: &SettingService{}}
					require.NoError(t, probe.testOpenAIImageOAuth(c, ctx, account, "gpt-image-2.6-flare", "a logo"))
					require.Contains(t, rec.Body.String(), "data:image/svg+xml;base64,"+base64.StdEncoding.EncodeToString([]byte(raw)))
					require.Contains(t, rec.Body.String(), `"success":true`)
				} else {
					if kind == "apikey-api" {
						account = newOpenAIImagesAPIKeyAccount()
					}
					svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, settingService: &SettingService{}}
					parsed, err := svc.ParseOpenAIImagesRequest(c, body)
					require.NoError(t, err)
					result, err := svc.ForwardImages(t.Context(), c, account, body, parsed, "")
					require.NoError(t, err)
					require.Equal(t, base64.StdEncoding.EncodeToString([]byte(raw)), gjson.GetBytes(rec.Body.Bytes(), "data.0.b64_json").String())
					require.True(t, gjson.GetBytes(rec.Body.Bytes(), "provider_metadata.keep").Bool())
					require.Equal(t, 50, result.Usage.SVGUsage.InputTokens)
				}
				require.NotContains(t, rec.Body.String(), "fallback")
				require.NotContains(t, string(upstream.lastBody), "fallback")
			})
		}
	}
}
