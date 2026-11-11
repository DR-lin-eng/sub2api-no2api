//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type chatGPTAppImageCall struct {
	host, path, method                string
	body                              []byte
	headers                           http.Header
	public, upload, redirectsDisabled bool
}

type chatGPTAppImageLocalUpstream struct {
	target *url.URL
	mu     sync.Mutex
	calls  []chatGPTAppImageCall
}

func (u *chatGPTAppImageLocalUpstream) imageCalls() []chatGPTAppImageCall {
	u.mu.Lock()
	defer u.mu.Unlock()
	var calls []chatGPTAppImageCall
	for _, call := range u.calls {
		if call.path != "/backend-api/sentinel/chat-requirements/prepare" {
			calls = append(calls, call)
		}
	}
	return calls
}

func (u *chatGPTAppImageLocalUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.mu.Lock()
	u.calls = append(u.calls, chatGPTAppImageCall{host: req.URL.Host, path: req.URL.Path, method: req.Method, body: body, headers: req.Header.Clone(), public: HTTPUpstreamPublicDestination(req.Context()), upload: HTTPUpstreamPublicUpload(req.Context()), redirectsDisabled: HTTPUpstreamRedirectsDisabled(req.Context())})
	u.mu.Unlock()
	clone := req.Clone(req.Context())
	copyURL := *req.URL
	clone.URL = &copyURL
	clone.URL.Scheme = u.target.Scheme
	clone.URL.Host = u.target.Host
	clone.Body = io.NopCloser(strings.NewReader(string(body)))
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return client.Do(clone)
}

func (u *chatGPTAppImageLocalUpstream) DoWithTLS(r *http.Request, p string, a int64, n int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(r, p, a, n)
}

func newChatGPTAppImageTestService(t *testing.T, handler http.HandlerFunc) (*OpenAIGatewayService, *chatGPTAppImageLocalUpstream) {
	return newChatGPTAppImageTestServiceWithPrepare(t, nil, handler)
}

func newChatGPTAppImageTestServiceWithPrepare(t *testing.T, prepare, handler http.HandlerFunc) (*OpenAIGatewayService, *chatGPTAppImageLocalUpstream) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/backend-api/sentinel/chat-requirements/prepare" {
			if prepare != nil {
				prepare(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"token":"fixture-requirements-token"}`)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	upstream := &chatGPTAppImageLocalUpstream{target: target}
	return &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}, upstream
}

func chatGPTAppImageTestAccount() *Account {
	return &Account{ID: 811, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "fixture-bearer", "chatgpt_account_id": "fixture-account", "plan_type": "team"}}
}

func chatGPTAppImageStream(w http.ResponseWriter, encoded string) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "data: {\"conversation_id\":\"conv-fixture\",\"message\":{\"author\":{\"role\":\"assistant\"},\"metadata\":{\"model_slug\":\"selected-chat-model\"},\"output\":[{\"type\":\"image_generation_call\",\"id\":\"image-fixture\",\"status\":\"completed\",\"result\":%q}]}}\n\ndata: [DONE]\n\n", encoded)
}

func TestChatGPTAppImagesDefaultAndLegacyRollback(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("legacy=%t/stream=%t", legacy, stream), func(t *testing.T) {
				encoded := encodeOpenAIImageTestPNG(t, 4, 3)
				svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
					if legacy {
						require.Equal(t, "/backend-api/codex/responses", r.URL.Path)
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprintf(w, "data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"image_generation_call\",\"result\":%q}]}}\n\ndata: [DONE]\n\n", encoded)
					} else {
						require.Equal(t, "/backend-api/f/conversation", r.URL.Path)
						chatGPTAppImageStream(w, "data:image/png;base64,"+encoded)
					}
				})
				if legacy {
					enableLegacyOpenAIImagesForTest(svc)
				}
				body := []byte(fmt.Sprintf(`{"model":"gpt-image-2","prompt":"  exact prompt\n","stream":%t}`, stream))
				c := newOpenAIImagesContext(t, body)
				parsed, err := svc.ParseOpenAIImagesRequest(c, body)
				require.NoError(t, err)
				result, err := svc.ForwardImages(context.Background(), c, chatGPTAppImageTestAccount(), body, parsed, "")
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 1, result.ImageCount)
				require.Equal(t, []string{"4x3"}, result.ImageOutputSizes)
				require.Len(t, upstream.imageCalls(), 1)
				call := upstream.imageCalls()[0]
				require.Equal(t, "Bearer fixture-bearer", call.headers.Get("Authorization"))
				require.Equal(t, "fixture-account", call.headers.Get("chatgpt-account-id"))
				if !legacy {
					require.Equal(t, "picture_v2", gjson.GetBytes(call.body, "system_hints.0").String())
					require.Equal(t, "  exact prompt\n", gjson.GetBytes(call.body, "messages.0.content.parts.0").String())
					require.Equal(t, "2", call.headers.Get("x-oai-mcp-form-version"))
					require.Equal(t, "codex_webview", call.headers.Get("x-openai-web-frontend"))
					require.Equal(t, "fixture-requirements-token", call.headers.Get("OpenAI-Sentinel-Chat-Requirements-Token"))
					require.True(t, call.redirectsDisabled)
					require.Empty(t, call.headers.Get("version"))
				}
			})
		}
	}
}

func TestChatGPTAppImagesCompletedDownloadFailureRetainsSettlement(t *testing.T) {
	svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/backend-api/f/conversation" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("x-request-id", "generation-request")
			fmt.Fprint(w, "data: {\"conversation_id\":\"conv-fixture\",\"message\":{\"author\":{\"role\":\"tool\"},\"content\":{\"content_type\":\"multimodal_text\",\"parts\":[{\"content_type\":\"image_asset_pointer\",\"asset_pointer\":\"sediment://file_output\",\"width\":3,\"height\":3}]}}}\n\ndata: [DONE]\n\n")
			return
		}
		require.Equal(t, "/backend-api/files/download/file_output", r.URL.Path)
		w.Header().Set("x-request-id", "download-request")
		w.WriteHeader(503)
		fmt.Fprint(w, `{"error":{"message":"download unavailable"}}`)
	})
	body := []byte(`{"model":"gpt-image-2","prompt":"draw"}`)
	c := newOpenAIImagesContext(t, body)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	result, err := svc.ForwardImages(t.Context(), c, chatGPTAppImageTestAccount(), body, parsed, "")
	require.Error(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, "generation-request", result.RequestID)
	require.Equal(t, "conv-fixture", result.ResponseID)
	require.Equal(t, []string{"3x3"}, result.ImageOutputSizes)
	require.Equal(t, 503, c.Writer.Status())
	require.Len(t, upstream.imageCalls(), 2)
}

func TestChatGPTAppImagesConfiguredAccountProbeFollowsProtocol(t *testing.T) {
	encoded := encodeOpenAIImageTestPNG(t, 3, 3)
	_, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/f/conversation", r.URL.Path)
		chatGPTAppImageStream(w, encoded)
	})
	probe := &AccountTestService{settingService: &SettingService{}, cfg: &config.Config{}, httpUpstream: upstream}
	c := newOpenAIImagesContext(t, []byte(`{}`))
	require.NoError(t, probe.testOpenAIImageOAuth(c, t.Context(), chatGPTAppImageTestAccount(), "gpt-image-2", "probe prompt"))
	require.Len(t, upstream.imageCalls(), 1)
}

func TestChatGPTAppImagesFileResolverAndStorageCredentials(t *testing.T) {
	encoded := encodeOpenAIImageTestPNG(t, 5, 4)
	data, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/f/conversation":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"conversation_id\":\"conv-fixture\",\"message\":{\"author\":{\"role\":\"tool\"},\"content\":{\"content_type\":\"multimodal_text\",\"parts\":[{\"content_type\":\"image_asset_pointer\",\"asset_pointer\":\"sediment://file_generated\"}]}}}\n\ndata: [DONE]\n\n")
		case "/backend-api/files/download/file_generated":
			require.Equal(t, "conv-fixture", r.URL.Query().Get("conversation_id"))
			fmt.Fprint(w, `{"status":"success","download_url":"https://storage.example/image.png"}`)
		case "/image.png":
			require.Empty(t, r.Header.Get("Authorization"))
			require.Empty(t, r.Header.Get("ChatGPT-Account-ID"))
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(data)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	body := []byte(`{"model":"gpt-image-2","prompt":"draw"}`)
	c := newOpenAIImagesContext(t, body)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	result, err := svc.ForwardImages(context.Background(), c, chatGPTAppImageTestAccount(), body, parsed, "")
	require.NoError(t, err)
	require.Equal(t, 1, result.ImageCount)
	require.Len(t, upstream.imageCalls(), 3)
	require.True(t, upstream.imageCalls()[2].public)
	require.True(t, upstream.imageCalls()[2].redirectsDisabled)
}

func TestChatGPTAppImagesEditUploadLifecycle(t *testing.T) {
	encoded := encodeOpenAIImageTestPNG(t, 3, 2)
	svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/files":
			fmt.Fprint(w, `{"file_id":"file_reference","upload_url":"https://storage.example/upload","upload_headers":{"x-ms-blob-type":"BlockBlob","Authorization":"must-not-forward"}}`)
		case "/upload":
			require.Equal(t, http.MethodPut, r.Method)
			require.Empty(t, r.Header.Get("Authorization"))
			require.Equal(t, "BlockBlob", r.Header.Get("x-ms-blob-type"))
			w.WriteHeader(201)
		case "/backend-api/files/process_upload_stream":
			w.Header().Set("Content-Type", "application/x-ndjson")
			fmt.Fprint(w, "{\"file_id\":\"file_reference\",\"event\":\"file.processing.file_ready\"}\n{\"file_id\":\"file_reference\",\"event\":\"file.processing.completed\"}\n")
		case "/backend-api/f/conversation":
			chatGPTAppImageStream(w, encoded)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	body := []byte(fmt.Sprintf(`{"model":"gpt-image-2","prompt":"edit","images":[{"image_url":"data:image/png;base64,%s"}]}`, encoded))
	c := newOpenAIImagesContext(t, body)
	c.Request.URL.Path = "/v1/images/edits"
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	result, err := svc.ForwardImages(context.Background(), c, chatGPTAppImageTestAccount(), body, parsed, "")
	require.NoError(t, err)
	require.Equal(t, 1, result.ImageCount)
	require.Len(t, upstream.imageCalls(), 4)
	require.Equal(t, "reference_image", gjson.GetBytes(upstream.imageCalls()[0].body, "upload_source").String())
	require.False(t, gjson.GetBytes(upstream.imageCalls()[0].body, "store_in_library").Bool())
	require.Equal(t, "sediment://file_reference", gjson.GetBytes(upstream.imageCalls()[3].body, "messages.0.content.parts.0.asset_pointer").String())
}

func TestChatGPTAppImagesErrorsDoNotRegenerate(t *testing.T) {
	svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		fmt.Fprint(w, `{"error":{"message":"generation failed"}}`)
	})
	body := []byte(`{"model":"gpt-image-2","prompt":"draw"}`)
	c := newOpenAIImagesContext(t, body)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	result, err := svc.ForwardImages(context.Background(), c, chatGPTAppImageTestAccount(), body, parsed, "")
	require.Nil(t, result)
	require.Error(t, err)
	require.Len(t, upstream.imageCalls(), 1)
	require.Equal(t, 502, c.Writer.Status())
}

func TestChatGPTAppImagesUnsupportedOptionsFailBeforeUpstream(t *testing.T) {
	svc, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unsupported request reached upstream") })
	for _, field := range []string{`"output_format":"svg"`} {
		body := []byte(`{"model":"gpt-image-2.6-flare","prompt":"draw",` + field + `}`)
		c := newOpenAIImagesContext(t, body)
		parsed, err := svc.ParseOpenAIImagesRequest(c, body)
		require.NoError(t, err)
		_, err = svc.forwardChatGPTAppImages(context.Background(), c, chatGPTAppImageTestAccount(), parsed, "")
		require.ErrorContains(t, err, "image generation options")
		require.NotContains(t, err.Error(), "legacy")
		require.NotContains(t, err.Error(), "fallback")
		require.Equal(t, 400, c.Writer.Status())
	}
	require.Empty(t, upstream.calls)
}

func TestChatGPTAppImagesSettingsRoundtrip(t *testing.T) {
	ctx := context.Background()
	repo := &gatewayTTLSettingRepo{data: map[string]string{}}
	settings := NewSettingService(repo, &config.Config{})
	require.False(t, settings.OpenAIImagesLegacyEnabled(ctx))
	view := settings.parseSettings(repo.data)
	require.False(t, view.OpenAIImagesLegacyEnabled)
	view.OpenAIImagesLegacyEnabled = true
	require.NoError(t, settings.UpdateSettings(ctx, view))
	require.True(t, settings.OpenAIImagesLegacyEnabled(ctx))
	require.Equal(t, "true", repo.data[SettingKeyOpenAIImagesLegacyEnabled])
	require.True(t, NewSettingService(repo, &config.Config{}).OpenAIImagesLegacyEnabled(ctx))
	view.OpenAIImagesLegacyEnabled = false
	require.NoError(t, settings.UpdateSettings(ctx, view))
	require.False(t, settings.OpenAIImagesLegacyEnabled(ctx))
}
