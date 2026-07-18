package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIResponsesImagePlanHTTPStub struct {
	mu       sync.Mutex
	calls    int
	started  chan struct{}
	release  <-chan struct{}
	response func(*http.Request) *http.Response
}

func (s *openAIResponsesImagePlanHTTPStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	if req != nil && req.Body != nil {
		_, _ = io.Copy(io.Discard, req.Body)
		_ = req.Body.Close()
	}
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()
	if call == 1 && s.started != nil {
		close(s.started)
	}
	if s.release != nil {
		<-s.release
	}
	if s.response != nil {
		return s.response(req), nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"image/png"}},
		Body:       io.NopCloser(bytes.NewReader(testOpenAIResponsesPNG())),
	}, nil
}

func (s *openAIResponsesImagePlanHTTPStub) DoWithTLS(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func (s *openAIResponsesImagePlanHTTPStub) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func testOpenAIResponsesPNG() []byte {
	return []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
}

func testOpenAIResponsesImageContext(body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, recorder
}

func TestOpenAIResponsesImagePlanMaterializesDataURLOnceForMultiImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	image := append(testOpenAIResponsesPNG(), bytes.Repeat([]byte{0x42}, 1<<20)...)
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)
	body, err := json.Marshal(map[string]any{
		"model": "gpt-5.4",
		"n":     10,
		"input": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": "Edit this image"},
				map[string]any{"type": "input_image", "image_url": dataURL},
				map[string]any{"type": "input_image", "image_url": dataURL},
			},
		}},
		"tools": []any{map[string]any{"type": "image_generation", "action": "edit"}},
	})
	require.NoError(t, err)
	plan, err := CompileOpenAIResponsesImagePlan(body)
	require.NoError(t, err)
	require.Equal(t, 10, plan.N)
	require.Len(t, plan.inputs, 2)
	c, _ := testOpenAIResponsesImageContext(body)

	require.NoError(t, (&OpenAIGatewayService{}).PrepareOpenAIResponsesImagePlan(context.Background(), c, plan))
	require.Empty(t, plan.inputs[0].ImageURL)
	require.Empty(t, plan.inputs[1].ImageURL)
	require.Equal(t, image, plan.inputs[0].Data)
	require.Same(t, &plan.inputs[0].Data[0], &plan.inputs[1].Data[0])

	// Reusing the plan for every n=1 child must not decode or replace the data.
	first := &plan.inputs[0].Data[0]
	for i := 0; i < plan.N; i++ {
		require.NoError(t, (&OpenAIGatewayService{}).PrepareOpenAIResponsesImagePlan(context.Background(), c, plan))
		require.Same(t, first, &plan.inputs[0].Data[0])
	}
}

func TestOpenAIResponsesImagePlanDownloadsRepeatedURLOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &openAIResponsesImagePlanHTTPStub{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: stub}
	body := []byte(`{
		"model":"gpt-5.4",
		"input":[{"role":"user","content":[
			{"type":"input_text","text":"Edit both"},
			{"type":"input_image","image_url":"https://cdn.example.com/same.png"},
			{"type":"input_image","image_url":"https://cdn.example.com/same.png"}
		]}],
		"tools":[{"type":"image_generation","action":"edit"}]
	}`)
	plan, err := CompileOpenAIResponsesImagePlan(body)
	require.NoError(t, err)
	c, _ := testOpenAIResponsesImageContext(body)

	require.NoError(t, svc.PrepareOpenAIResponsesImagePlan(context.Background(), c, plan))
	require.Equal(t, 1, stub.callCount())
	require.Same(t, &plan.inputs[0].Data[0], &plan.inputs[1].Data[0])
}

func TestOpenAIResponsesImagePlanSingleflightsFileIDPerAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	started := make(chan struct{})
	release := make(chan struct{})
	stub := &openAIResponsesImagePlanHTTPStub{started: started, release: release}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: stub}
	body := []byte(`{
		"model":"gpt-5.4",
		"input":[{"role":"user","content":[
			{"type":"input_text","text":"Edit this"},
			{"type":"input_image","file_id":"file_same"}
		]}],
		"tools":[{"type":"image_generation","action":"edit"}]
	}`)
	plan, err := CompileOpenAIResponsesImagePlan(body)
	require.NoError(t, err)
	account := &Account{
		ID: 91, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://api.example.com/v1"},
	}

	const callers = 8
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() {
			_, err := svc.resolveOpenAIResponsesImagePlanInputForAccount(context.Background(), account, plan, plan.inputs[0])
			errs <- err
		}()
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("file download did not start")
	}
	close(release)
	for i := 0; i < callers; i++ {
		require.NoError(t, <-errs)
	}
	require.Equal(t, 1, stub.callCount())
}

func TestOpenAIResponsesImagePlanStreamsOfficialEditMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"gpt-5.4",
		"input":[{"role":"user","content":[
			{"type":"input_text","text":"Make it blue"},
			{"type":"input_image","image_url":"data:image/png;base64,aW1hZ2U="},
			{"type":"input_image","image_url":"data:image/png;base64,aW1hZ2Uy"}
		]}],
		"tools":[{
			"type":"image_generation","action":"edit","input_fidelity":"high",
			"input_image_mask":{"image_url":"data:image/png;base64,bWFzaw=="}
		}],
		"response_format":"url",
		"stream":true
	}`)
	plan, err := CompileOpenAIResponsesImagePlan(body)
	require.NoError(t, err)
	c, _ := testOpenAIResponsesImageContext(body)
	bridge, err := (&OpenAIGatewayService{}).buildOpenAIResponsesImageAPIBridgePlanRequest(
		context.Background(), c, nil, plan, "gpt-5.4", "gpt-image-2", 1, true, true,
	)
	require.NoError(t, err)
	defer func() { _ = bridge.BodyReader.Close() }()
	_, ok := bridge.BodyReader.(*io.PipeReader)
	require.True(t, ok, "edit multipart should be streamed through an io.Pipe")
	mediaType, params, err := mime.ParseMediaType(bridge.ContentType)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	form, err := multipart.NewReader(bridge.BodyReader, params["boundary"]).ReadForm(openAIImageMaxDownloadBytes)
	require.NoError(t, err)
	defer func() { _ = form.RemoveAll() }()
	require.Equal(t, []string{"gpt-image-2"}, form.Value["model"])
	require.Equal(t, []string{"Make it blue"}, form.Value["prompt"])
	require.Equal(t, []string{"url"}, form.Value["response_format"])
	require.Equal(t, []string{"true"}, form.Value["stream"])
	require.Equal(t, []string{"high"}, form.Value["input_fidelity"])
	require.Len(t, form.File["image[]"], 2)
	require.Len(t, form.File["mask"], 1)
}

func TestOpenAIResponsesImagePlanCaptureOnlyJSONWritesNoChildBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","input":"Draw a cat"}`)
	plan, err := CompileOpenAIResponsesImagePlan(body)
	require.NoError(t, err)
	stub := &openAIResponsesImagePlanHTTPStub{response: func(*http.Request) *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"created":1710000010,"data":[{"b64_json":"aW1hZ2U="}]}`)),
		}
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: stub}
	c, recorder := testOpenAIResponsesImageContext(body)
	account := &Account{
		ID: 92, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://api.example.com/v1"},
	}

	result, err := svc.ForwardOpenAIResponsesImagePlan(
		context.Background(), c, account, plan, "gpt-5.4", "gpt-image-2", 1, false, true, time.Now(),
	)
	require.NoError(t, err)
	require.Empty(t, recorder.Body.Bytes())
	require.Len(t, result.ResponsesImageItems, 1)
	require.Equal(t, "aW1hZ2U=", result.ResponsesImageItems[0].Result)
}

func TestOpenAIResponsesImagePlanCaptureOnlyStreamPreservesImageEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","input":"Draw a cat","stream":true}`)
	plan, err := CompileOpenAIResponsesImagePlan(body)
	require.NoError(t, err)
	stub := &openAIResponsesImagePlanHTTPStub{response: func(*http.Request) *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(
				"event: image_generation.partial_image\n" +
					`data: {"type":"image_generation.partial_image","partial_image_index":0,"b64_json":"cGFydGlhbA=="}` + "\n\n" +
					"event: image_generation.completed\n" +
					`data: {"type":"image_generation.completed","b64_json":"ZmluYWw="}` + "\n\n",
			)),
		}
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: stub}
	c, recorder := testOpenAIResponsesImageContext(body)
	account := &Account{
		ID: 93, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://api.example.com/v1"},
	}

	result, err := svc.ForwardOpenAIResponsesImagePlan(
		context.Background(), c, account, plan, "gpt-5.4", "gpt-image-2", 1, true, true, time.Now(),
	)
	require.NoError(t, err)
	output := recorder.Body.String()
	require.Contains(t, output, "response.image_generation_call.partial_image")
	require.Contains(t, output, "response.output_item.added")
	require.Contains(t, output, "response.output_item.done")
	require.NotContains(t, output, "response.created")
	require.NotContains(t, output, "response.completed")
	require.Len(t, result.ResponsesImageItems, 1)
	require.Equal(t, "ZmluYWw=", result.ResponsesImageItems[0].Result)
}

func TestOpenAIResponsesImagePlanHydratesPreviousResponseAsEditInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{openaiWSStateStore: NewOpenAIWSStateStore(nil)}
	const groupID = int64(42)
	require.NoError(t, svc.StoreOpenAIForcedImageResponseState(
		context.Background(),
		groupID,
		"resp_previous",
		[]OpenAIResponsesImageOutputItem{{
			ID: "ig_previous", Type: "image_generation_call", Status: "completed",
			Result: base64.StdEncoding.EncodeToString(testOpenAIResponsesPNG()), OutputFormat: "png",
		}},
	))
	body := []byte(`{
		"model":"gpt-5.4",
		"previous_response_id":"resp_previous",
		"input":"Make it photorealistic",
		"tools":[{"type":"image_generation","action":"edit"}]
	}`)
	plan, err := CompileOpenAIResponsesImagePlan(body)
	require.NoError(t, err)
	require.Empty(t, plan.inputs)

	require.NoError(t, svc.HydrateOpenAIResponsesImagePlan(context.Background(), groupID, plan))
	require.Equal(t, "edit", plan.action)
	require.Len(t, plan.inputs, 1)
	require.True(t, strings.HasPrefix(plan.inputs[0].ImageURL, "data:image/png;base64,"))
	c, _ := testOpenAIResponsesImageContext(body)
	require.NoError(t, svc.PrepareOpenAIResponsesImagePlan(context.Background(), c, plan))
	require.Equal(t, testOpenAIResponsesPNG(), plan.inputs[0].Data)
	bridge, err := svc.buildOpenAIResponsesImageAPIBridgePlanRequest(
		context.Background(), c, nil, plan, "gpt-5.4", "gpt-image-2", 1, false, true,
	)
	require.NoError(t, err)
	defer func() { _ = bridge.BodyReader.Close() }()
	require.Equal(t, openAIImagesEditsEndpoint, bridge.Parsed.Endpoint)
}

func TestOpenAIResponsesImagePlanHydratesOfficialImageGenerationCallReference(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{openaiWSStateStore: NewOpenAIWSStateStore(nil)}
	const groupID = int64(43)
	require.NoError(t, svc.StoreOpenAIForcedImageResponseState(
		context.Background(),
		groupID,
		"resp_source",
		[]OpenAIResponsesImageOutputItem{{
			ID: "ig_source", Type: "image_generation_call", Status: "completed",
			Result: "https://api.example.com/generated/source.webp", OutputFormat: "webp",
		}},
	))
	body := []byte(`{
		"model":"gpt-5.4",
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"Change the colors"}]},
			{"type":"image_generation_call","id":"ig_source"}
		],
		"tools":[{"type":"image_generation"}]
	}`)
	plan, err := CompileOpenAIResponsesImagePlan(body)
	require.NoError(t, err)

	require.NoError(t, svc.HydrateOpenAIResponsesImagePlan(context.Background(), groupID, plan))
	require.Equal(t, "edit", plan.action)
	require.Len(t, plan.inputs, 1)
	require.Equal(t, "https://api.example.com/generated/source.webp", plan.inputs[0].ImageURL)
}

func TestOpenAIResponsesImagePlanRejectsMissingPreviousResponseState(t *testing.T) {
	svc := &OpenAIGatewayService{openaiWSStateStore: NewOpenAIWSStateStore(nil)}
	plan, err := CompileOpenAIResponsesImagePlan([]byte(`{
		"model":"gpt-5.4",
		"previous_response_id":"resp_missing",
		"input":"Edit the previous image",
		"tools":[{"type":"image_generation","action":"edit"}]
	}`))
	require.NoError(t, err)

	err = svc.HydrateOpenAIResponsesImagePlan(context.Background(), 44, plan)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found or has expired")
}

func TestOpenAIResponsesImagePlanExplicitGenerateDoesNotLoadPreviousImage(t *testing.T) {
	svc := &OpenAIGatewayService{openaiWSStateStore: NewOpenAIWSStateStore(nil)}
	plan, err := CompileOpenAIResponsesImagePlan([]byte(`{
		"model":"gpt-5.4",
		"previous_response_id":"resp_missing",
		"input":"Create a completely new image",
		"tools":[{"type":"image_generation","action":"generate"}]
	}`))
	require.NoError(t, err)

	require.NoError(t, svc.HydrateOpenAIResponsesImagePlan(context.Background(), 45, plan))
	require.Equal(t, "generate", plan.action)
	require.Empty(t, plan.inputs)
}

func TestOpenAIResponsesImagePlanPreviousResponseStateWorksAcrossInstances(t *testing.T) {
	cache := &stubOpenAIWSSharedCache{}
	writer := &OpenAIGatewayService{cache: cache}
	reader := &OpenAIGatewayService{cache: cache}
	const groupID = int64(46)
	require.NoError(t, writer.StoreOpenAIForcedImageResponseState(
		context.Background(),
		groupID,
		"resp_cross_instance",
		[]OpenAIResponsesImageOutputItem{{
			ID: "ig_cross_instance", Type: "image_generation_call", Status: "completed",
			Result: "aW1hZ2U=", OutputFormat: "png",
		}},
	))
	plan, err := CompileOpenAIResponsesImagePlan([]byte(`{
		"model":"gpt-5.4",
		"previous_response_id":"resp_cross_instance",
		"input":"Continue editing"
	}`))
	require.NoError(t, err)

	require.NoError(t, reader.HydrateOpenAIResponsesImagePlan(context.Background(), groupID, plan))
	require.Equal(t, "edit", plan.action)
	require.Len(t, plan.inputs, 1)
}
