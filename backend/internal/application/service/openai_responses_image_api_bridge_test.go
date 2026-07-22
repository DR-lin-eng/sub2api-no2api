package service

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAccountForceOpenAIImageAPI_APIKeyOnly(t *testing.T) {
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"openai_force_image_api": true}}
	require.True(t, apiKey.ForceOpenAIImageAPI())

	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"openai_force_image_api": true}}
	require.False(t, oauth.ForceOpenAIImageAPI())

	apiKey.Extra = map[string]any{PlatformOpenAI: map[string]any{"openai_force_image_api": true}}
	require.True(t, apiKey.ForceOpenAIImageAPI())
}

func TestBuildOpenAIResponsesImageAPIBridgeRequest_MapsOfficialFields(t *testing.T) {
	body := []byte(`{
		"model":"gpt-image-2",
		"instructions":"Keep the background transparent.",
		"input":[{"role":"user","content":[{"type":"input_text","text":"Draw a cat"}]}],
		"tools":[{"type":"image_generation","size":"1024x1024","quality":"high","output_format":"webp"}],
		"response_format":"url"
	}`)

	bridge, err := buildOpenAIResponsesImageAPIBridgeRequest(body, "gpt-image-2", "gpt-image-2")

	require.NoError(t, err)
	require.Equal(t, "Keep the background transparent.\n\nDraw a cat", gjson.GetBytes(bridge.Body, "prompt").String())
	require.Equal(t, "1024x1024", gjson.GetBytes(bridge.Body, "size").String())
	require.Equal(t, "high", gjson.GetBytes(bridge.Body, "quality").String())
	require.Equal(t, "webp", gjson.GetBytes(bridge.Body, "output_format").String())
	require.Equal(t, "url", gjson.GetBytes(bridge.Body, "response_format").String())
	require.Equal(t, "url", bridge.Parsed.ResponseFormat)
}

func TestNormalizeOpenAIResponsesImageToolRequest_InjectsAndSplitsStreamingMultiImage(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"Draw a cat","stream":true,"n":4,"previous_response_id":"resp_previous"}`)

	normalized, n, imageModel, err := NormalizeOpenAIResponsesImageToolRequest(body)

	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, "gpt-image-2", imageModel)
	require.Equal(t, "image_generation", gjson.GetBytes(normalized, "tools.0.type").String())
	require.Equal(t, "resp_previous", gjson.GetBytes(normalized, "previous_response_id").String())

	child, err := PrepareOpenAIResponsesImageChildRequest(normalized, true)
	require.NoError(t, err)
	require.EqualValues(t, 1, gjson.GetBytes(child, "n").Int())
	require.True(t, gjson.GetBytes(child, "stream").Bool())

	bridge, err := buildOpenAIResponsesImageAPIBridgeRequest(child, "gpt-5.4", "gpt-image-2")
	require.NoError(t, err)
	require.Equal(t, "resp_previous", gjson.GetBytes(child, "previous_response_id").String())
	require.Equal(t, 1, bridge.Parsed.N)
	require.True(t, bridge.Parsed.Stream)
}

func TestBuildOpenAIResponsesImageAPIBridgeRequest_EditUsesOfficialMultipart(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.4",
		"input":[{"role":"user","content":[
			{"type":"input_text","text":"Make it blue"},
			{"type":"input_image","image_url":"data:image/png;base64,aW1hZ2U="}
		]}],
		"tools":[{"type":"image_generation","model":"gpt-image-2","action":"edit","input_fidelity":"high"}],
		"stream":true
	}`)

	bridge, err := buildOpenAIResponsesImageAPIBridgeRequest(body, "gpt-5.4", "gpt-image-2")

	require.NoError(t, err)
	require.Equal(t, openAIImagesEditsEndpoint, bridge.Parsed.Endpoint)
	require.True(t, bridge.Parsed.Multipart)
	mediaType, params, err := mime.ParseMediaType(bridge.ContentType)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	reader := multipart.NewReader(bytes.NewReader(bridge.Body), params["boundary"])
	form, err := reader.ReadForm(openAIImageMaxDownloadBytes)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-image-2"}, form.Value["model"])
	require.Equal(t, []string{"Make it blue"}, form.Value["prompt"])
	require.Equal(t, []string{"true"}, form.Value["stream"])
	require.Len(t, form.File["image"], 1)
}

func TestOpenAIGatewayForward_ForcedImageAPIEditUsesSelectedAccountEditsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"gpt-5.4",
		"input":[{"role":"user","content":[
			{"type":"input_text","text":"Make it blue"},
			{"type":"input_image","image_url":"data:image/png;base64,aW1hZ2U="}
		]}],
		"tools":[{"type":"image_generation","model":"gpt-image-2","action":"edit"}]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"created":1710000010,"data":[{"b64_json":"aW1hZ2U="}]}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 77, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://image.example/v1"},
		Extra:       map[string]any{"openai_force_image_api": true},
	}

	result, err := svc.ForwardOpenAIResponsesViaImagesAPI(context.Background(), c, account, body, "gpt-5.4", "gpt-image-2", time.Now())

	require.NoError(t, err)
	require.Equal(t, openAIImagesEditsEndpoint, result.UpstreamEndpoint)
	require.Equal(t, "https://image.example/v1/images/edits", upstream.lastReq.URL.String())
	mediaType, params, err := mime.ParseMediaType(upstream.lastReq.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	form, err := multipart.NewReader(bytes.NewReader(upstream.lastBody), params["boundary"]).ReadForm(openAIImageMaxDownloadBytes)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-image-2"}, form.Value["model"])
	require.Len(t, form.File["image"], 1)
	require.Equal(t, "aW1hZ2U=", gjson.Get(recorder.Body.String(), "output.0.result").String())
}

func TestOpenAIGatewayForward_ForcedImageAPIURLUsesLocalGeneratedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2","input":"Draw a cat","response_format":"url"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	req.Host = "api.funai.works"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	rawURL := "https://cdn.image-upstream.example/generated/result.png?signature=abc"
	store := &generatedImageStoreStub{}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"created":1710000010,"data":[{"url":"` + rawURL + `"}]}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, cache: store, httpUpstream: upstream}
	account := &Account{
		ID: 43, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://image.example/v1"},
		Extra:       map[string]any{"openai_force_image_api": true},
	}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "url", gjson.GetBytes(upstream.lastBody, "response_format").String())
	localURL := gjson.Get(rec.Body.String(), "output.0.result").String()
	require.True(t, strings.HasPrefix(localURL, "https://api.funai.works/generated/"))
	require.True(t, strings.HasSuffix(localURL, ".png"))
	require.NotContains(t, rec.Body.String(), "cdn.image-upstream.example")
	require.Len(t, store.urls, 1)
	for hash, storedURL := range store.urls {
		require.Equal(t, rawURL, storedURL)
		require.Equal(t, 30*time.Minute, store.ttls[hash])
		require.Contains(t, localURL, hash)
	}
}

func TestOpenAIGatewayForward_ForcedImageAPIPostprocessFailureStillReturnsBillableImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2","input":"Draw a cat","response_format":"url"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	req.Host = "api.funai.works"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"created":1710000010,"data":[{"url":"https://cdn.image-upstream.example/generated/result.png"}]}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 43, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://image.example/v1"},
		Extra:       map[string]any{"openai_force_image_api": true},
	}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.Error(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, "gpt-image-2", result.BillingModel)
	require.NotContains(t, recorder.Body.String(), "cdn.image-upstream.example")
}

func TestOpenAIGatewayForward_ForcedImageAPIURLStreamNeverLeaksUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2","input":"Draw a cat","stream":true,"response_format":"url"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	req.Host = "api.funai.works"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	rawURL := "https://cdn.image-upstream.example/generated/result.webp"
	store := &generatedImageStoreStub{}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"event: image_generation.completed\n" +
				`data: {"type":"image_generation.completed","url":"` + rawURL + `"}` + "\n\n",
		)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, cache: store, httpUpstream: upstream}
	account := &Account{
		ID: 44, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://image.example/v1"},
		Extra:       map[string]any{"openai_force_image_api": true},
	}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, rec.Body.String(), "event: response.completed")
	require.Contains(t, rec.Body.String(), "https://api.funai.works/generated/")
	require.NotContains(t, rec.Body.String(), "cdn.image-upstream.example")
	require.Len(t, store.urls, 1)
}

func TestOpenAIGatewayForward_ForcedImageAPIConvertsBackToResponsesAndBillsImageOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2","input":"Draw a cat","size":"1024x1024"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"req_forced_image"},
		},
		Body: io.NopCloser(strings.NewReader(`{
			"created":1710000010,
			"output_format":"png",
			"size":"1024x1024",
			"usage":{"input_tokens":12,"output_tokens":21,"total_tokens":33,"output_tokens_details":{"image_tokens":21}},
			"data":[{"b64_json":"aW1hZ2U=","revised_prompt":"Draw a cat"}]
		}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 41, Name: "forced-image", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://image.example/v1"},
		Extra:       map[string]any{"openai_force_image_api": true},
	}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "https://image.example/v1/images/generations", upstream.lastReq.URL.String())
	require.Equal(t, "gpt-image-2", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "Draw a cat", gjson.GetBytes(upstream.lastBody, "prompt").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "response_format").Exists())
	require.Equal(t, openAIImagesGenerationsEndpoint, result.UpstreamEndpoint)
	require.Equal(t, "gpt-image-2", result.BillingModel)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 21, result.Usage.OutputTokens)
	require.Equal(t, 21, result.Usage.ImageOutputTokens)
	require.Equal(t, "response", gjson.Get(rec.Body.String(), "object").String())
	require.Equal(t, "completed", gjson.Get(rec.Body.String(), "status").String())
	require.Equal(t, "image_generation_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, "aW1hZ2U=", gjson.Get(rec.Body.String(), "output.0.result").String())
}

func TestOpenAIGatewayForward_ForcedImageAPIDefaultConvertsUnexpectedURLToBase64(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2","input":"Draw a cat"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	rawURL := "https://cdn.image-upstream.example/generated/result.png"
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"created":1710000010,"data":[{"url":"` + rawURL + `"}]}`)),
		},
		{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"image/png"}},
			Body:          io.NopCloser(strings.NewReader("png-bytes")),
			ContentLength: 9,
		},
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 45, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://image.example/v1"},
		Extra:       map[string]any{"openai_force_image_api": true},
	}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)
	require.False(t, gjson.GetBytes(upstream.bodies[0], "response_format").Exists())
	require.Equal(t, http.MethodGet, upstream.requests[1].Method)
	require.Equal(t, rawURL, upstream.requests[1].URL.String())
	require.Empty(t, upstream.requests[1].Header.Get("Authorization"))
	require.Equal(t, "cG5nLWJ5dGVz", gjson.Get(rec.Body.String(), "output.0.result").String())
	require.NotContains(t, rec.Body.String(), rawURL)
}

func TestOpenAIGatewayForward_ForcedImageAPIDisconnectSkipsBackfillButKeepsBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2","input":"Draw a cat","stream":true}`)
	requestCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body)).WithContext(requestCtx)
	req.Header.Set("Content-Type", "application/json")
	cancel()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	rawURL := "https://cdn.image-upstream.example/generated/result.png"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"created":1710000010,
			"usage":{"input_tokens":5,"output_tokens":8,"total_tokens":13,"output_tokens_details":{"image_tokens":8}},
			"data":[{"url":"` + rawURL + `"}]
		}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 46, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://image.example/v1"},
		Extra:       map[string]any{"openai_force_image_api": true},
	}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, 8, result.Usage.ImageOutputTokens)
	require.Len(t, upstream.requests, 1, "disconnected clients must not start image URL backfills")
	require.NotContains(t, rec.Body.String(), rawURL)
}

func TestOpenAIGatewayForward_ForcedImageAPIStreamsOfficialResponsesEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2","input":"Draw a cat","stream":true,"partial_images":1}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"event: image_generation.partial_image\n" +
				`data: {"type":"image_generation.partial_image","b64_json":"cGFydGlhbA==","partial_image_index":0}` + "\n\n" +
				"event: image_generation.completed\n" +
				`data: {"type":"image_generation.completed","b64_json":"ZmluYWw=","usage":{"input_tokens":5,"output_tokens":8,"total_tokens":13,"output_tokens_details":{"image_tokens":8}}}` + "\n\n",
		)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 42, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://image.example/v1"},
		Extra:       map[string]any{"openai_force_image_api": true},
	}

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, 8, result.Usage.ImageOutputTokens)
	require.Contains(t, rec.Body.String(), "event: response.created")
	require.Contains(t, rec.Body.String(), "event: response.output_item.added")
	require.Contains(t, rec.Body.String(), "event: response.image_generation_call.partial_image")
	require.Contains(t, rec.Body.String(), `"partial_image_b64":"cGFydGlhbA=="`)
	require.Contains(t, rec.Body.String(), "event: response.output_item.done")
	require.Contains(t, rec.Body.String(), "event: response.completed")
	require.NotContains(t, rec.Body.String(), "image_generation.completed\ndata:")
}
