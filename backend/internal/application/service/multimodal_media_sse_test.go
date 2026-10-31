//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func multimodalImageForwardFixture(t *testing.T, request []byte, contentType string, response io.Reader) (*OpenAIGatewayService, *gin.Context, *httptest.ResponseRecorder, *httpUpstreamRecorder, *Account) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(request))
	c.Request.Header.Set("Content-Type", contentType)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}, "X-Request-Id": {"req-multimodal"}},
		Body:       io.NopCloser(response),
	}}
	account := &Account{ID: 17, Platform: PlatformMultimodal, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://media.example/v1"}}
	return &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}, c, recorder, upstream, account
}

func TestForwardMultimodalMediaImageSSEClientIntentAndAccounting(t *testing.T) {
	wire := "data: {\"created\":1,\"data\":[{\"index\":0,\"b64_json\":\"/9j/shared-a\",\"size\":\"1024x1024\"}]}\n\n" +
		"data: {\"data\":[{\"index\":1,\"b64_json\":\"/9j/shared-b\",\"size\":\"2048x2048\"}],\"usage\":{\"input_tokens\":7,\"output_tokens\":3}}\n\n" +
		"data: [DONE]\n\n"
	for _, streamField := range []string{"", `,"stream":false`, `,"stream":true`} {
		t.Run("stream"+streamField, func(t *testing.T) {
			body := []byte(`{"model":"seedream","prompt":"fixture"` + streamField + `}`)
			svc, c, recorder, upstream, account := multimodalImageForwardFixture(t, body, "application/json", strings.NewReader(wire))
			result, err := svc.ForwardMultimodalMedia(context.Background(), c, account, MultimodalMediaEndpointImagesGenerations, "", body, "application/json", nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool(), "upstream must use SSE independently of the client's format")
			require.Equal(t, 2, result.ImageCount)
			require.Equal(t, []string{"1024x1024", "2048x2048"}, result.ImageOutputSizes)
			require.Equal(t, 7, result.Usage.InputTokens)
			require.Equal(t, 3, result.Usage.OutputTokens)
			wantsStream := strings.Contains(streamField, "true")
			require.Equal(t, wantsStream, result.Stream)
			if wantsStream {
				require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
				require.Equal(t, wire, recorder.Body.String())
				require.True(t, recorder.Flushed)
			} else {
				require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
				require.True(t, gjson.Valid(recorder.Body.String()))
				require.Len(t, gjson.Get(recorder.Body.String(), "data").Array(), 2)
			}
		})
	}
}

type multimodalChunkReader struct {
	chunks     []string
	beforeRead func()
	finalErr   error
}

func (r *multimodalChunkReader) Read(p []byte) (int, error) {
	if r.beforeRead != nil {
		r.beforeRead()
	}
	if len(r.chunks) == 0 {
		if r.finalErr != nil {
			return 0, r.finalErr
		}
		return 0, io.EOF
	}
	n := copy(p, r.chunks[0])
	r.chunks[0] = r.chunks[0][n:]
	if r.chunks[0] == "" {
		r.chunks = r.chunks[1:]
	}
	return n, nil
}

func TestForwardMultimodalMediaImageSSEFlushesBeforeNextRead(t *testing.T) {
	reader := &multimodalChunkReader{chunks: []string{
		"data: {\"progress\":1}\n\n",
		"data: {\"data\":[{\"b64_json\":\"final\"}]}\n\n",
	}}
	body := []byte(`{"model":"seedream","stream":true}`)
	svc, c, recorder, _, account := multimodalImageForwardFixture(t, body, "application/json", reader)
	reads := 0
	reader.beforeRead = func() {
		reads++
		if reads > 1 {
			require.True(t, recorder.Flushed, "progress must arrive before the next upstream read")
			require.Contains(t, recorder.Body.String(), `"progress":1`)
		}
	}
	result, err := svc.ForwardMultimodalMedia(context.Background(), c, account, MultimodalMediaEndpointImagesGenerations, "", body, "application/json", nil)
	require.NoError(t, err)
	require.Equal(t, 1, result.ImageCount)
}

func TestForwardMultimodalMediaImageSSEKeepsUsageAfterReadFailure(t *testing.T) {
	reader := &multimodalChunkReader{
		chunks:   []string{"data: {\"data\":[{\"b64_json\":\"delivered\"}],\"usage\":{\"input_tokens\":8}}\n\n"},
		finalErr: io.ErrUnexpectedEOF,
	}
	body := []byte(`{"model":"seedream","stream":true}`)
	svc, c, recorder, _, account := multimodalImageForwardFixture(t, body, "application/json", reader)
	result, err := svc.ForwardMultimodalMedia(context.Background(), c, account, MultimodalMediaEndpointImagesGenerations, "", body, "application/json", nil)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.NotNil(t, result)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, 8, result.Usage.InputTokens)
	require.Contains(t, recorder.Body.String(), "event: error\n")
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
}

func TestForwardMultimodalMediaImageSSEDrainsAfterClientDisconnect(t *testing.T) {
	reader := &multimodalChunkReader{chunks: []string{
		"data: {\"data\":[{\"index\":0,\"b64_json\":\"first\"}]}\n\n",
		"data: {\"data\":[{\"index\":1,\"b64_json\":\"second\"}],\"usage\":{\"output_tokens\":9}}\n\n",
	}}
	body := []byte(`{"model":"seedream","stream":true}`)
	svc, c, _, _, account := multimodalImageForwardFixture(t, body, "application/json", reader)
	c.Writer = &failingOpenAIImageWriter{ResponseWriter: c.Writer, failAfter: 0}
	result, err := svc.ForwardMultimodalMedia(context.Background(), c, account, MultimodalMediaEndpointImagesGenerations, "", body, "application/json", nil)
	require.NoError(t, err)
	require.Empty(t, reader.chunks, "drain accepted upstream work for reliable accounting")
	require.Equal(t, 2, result.ImageCount)
	require.Equal(t, 9, result.Usage.OutputTokens)
}

func TestForwardMultimodalMediaImageSSERejectsUnpricedOutputBeforeDelivery(t *testing.T) {
	for _, stream := range []bool{false, true} {
		body := []byte(`{"model":"seedream","stream":` + strconv.FormatBool(stream) + `}`)
		wire := "data: {\"data\":[{\"b64_json\":\"unpriced-image\",\"size\":\"3840x2160\"}],\"error\":null}\n\n"
		svc, c, recorder, _, account := multimodalImageForwardFixture(t, body, "application/json", strings.NewReader(wire))
		price := 0.01
		group := &Group{ID: 12, Platform: PlatformMultimodal, ModelPricing: []ChannelModelPricing{{
			Models: []string{"seedream"}, BillingMode: BillingModeImage,
			Intervals: []PricingInterval{{TierLabel: ImageBillingSize1K, PerRequestPrice: &price}},
		}}}
		group.compileModelPricingIndex()
		c.Set("api_key", &APIKey{ID: 2, UserID: 3, GroupID: &group.ID, Group: group})
		svc.billingService = NewBillingService(&config.Config{}, nil)
		svc.resolver = NewModelPricingResolver(nil, svc.billingService)
		_, err := svc.ForwardMultimodalMedia(context.Background(), c, account, MultimodalMediaEndpointImagesGenerations, "", body, "application/json", nil)
		require.ErrorIs(t, err, ErrImagePriceNotConfigured)
		require.NotContains(t, recorder.Body.String(), "unpriced-image")
	}
}

func TestAggregateMultimodalImageSSEIdentityAndBounds(t *testing.T) {
	t.Run("shared prefixes do not merge unrelated outputs", func(t *testing.T) {
		body := []byte("data: {\"data\":[{\"b64_json\":\"same-prefix\"}]}\n\n" +
			"data: {\"data\":[{\"b64_json\":\"same-prefix-longer\"}]}\n\n")
		out, ok := aggregateMultimodalImageSSE(body)
		require.True(t, ok)
		require.Len(t, gjson.GetBytes(out, "data").Array(), 2)
	})
	t.Run("multiline and explicit image identity", func(t *testing.T) {
		body := []byte("data: {\"data\":[{\"id\":\"image-1\",\"b64_json\":\"preview\"}]}\n\n" +
			"data: {\"data\":[\n" + "data: {\"id\":\"image-1\",\"b64_json\":\"final\",\"size\":\"2048x2048\"}]}\n\n")
		out, ok := aggregateMultimodalImageSSE(body)
		require.True(t, ok)
		require.Len(t, gjson.GetBytes(out, "data").Array(), 1)
		require.Equal(t, "final", gjson.GetBytes(out, "data.0.b64_json").String())
		require.Equal(t, "2048x2048", gjson.GetBytes(out, "data.0.size").String())
	})
	t.Run("image index aliases preserve identity across event shapes", func(t *testing.T) {
		body := []byte("data: {\"image_index\":0,\"data\":[{\"b64_json\":\"preview\"}]}\n\n" +
			"data: {\"data\":[{\"output_index\":0,\"b64_json\":\"final\"}]}\n\n")
		out, ok := aggregateMultimodalImageSSE(body)
		require.True(t, ok)
		require.Len(t, gjson.GetBytes(out, "data").Array(), 1)
		require.Equal(t, "final", gjson.GetBytes(out, "data.0.b64_json").String())
	})
	t.Run("incremental arrays do not reuse unrelated positional identities", func(t *testing.T) {
		body := []byte("data: {\"data\":[{\"url\":\"a\"},{\"url\":\"b\"}]}\n\n" +
			"data: {\"data\":[{\"url\":\"c\"},{\"url\":\"d\"}]}\n\n")
		out, ok := aggregateMultimodalImageSSE(body)
		require.True(t, ok)
		require.Len(t, gjson.GetBytes(out, "data").Array(), 4)
	})
	t.Run("identical outputs in one array count independently", func(t *testing.T) {
		frame := "data: {\"data\":[{\"url\":\"same\"},{\"url\":\"same\"}]}\n\n"
		out, ok := aggregateMultimodalImageSSE([]byte(frame + frame))
		require.True(t, ok)
		require.Len(t, gjson.GetBytes(out, "data").Array(), 2)
	})
	t.Run("large valid image frame exceeds the former scanner limit", func(t *testing.T) {
		image := strings.Repeat("a", 16*1024*1024+1)
		out, ok := aggregateMultimodalImageSSE([]byte("data: {\"data\":[{\"b64_json\":\"" + image + "\"}]}\n\n"))
		require.True(t, ok)
		require.Equal(t, len(image), len(gjson.GetBytes(out, "data.0.b64_json").String()))
	})
	t.Run("total input is bounded and malformed frames fail", func(t *testing.T) {
		err := readMultimodalSSEFrames(strings.NewReader(strings.Repeat("x", 33)), 32, func([]byte) error { return nil })
		require.ErrorIs(t, err, ErrUpstreamResponseBodyTooLarge)
		_, ok := aggregateMultimodalImageSSE([]byte("data: {broken}\n\n"))
		require.False(t, ok)
	})
	t.Run("null error is not a failed event", func(t *testing.T) {
		out, ok := aggregateMultimodalImageSSE([]byte("data: {\"error\":null,\"data\":[{\"b64_json\":\"ok\"}]}\n\n"))
		require.True(t, ok)
		require.Equal(t, "ok", gjson.GetBytes(out, "data.0.b64_json").String())
	})
}

func TestMultimodalImageMultipartStreamIntent(t *testing.T) {
	for _, value := range []string{"false", "true"} {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.WriteField("model", "seedream"))
		require.NoError(t, writer.WriteField("stream", value))
		require.NoError(t, writer.Close())
		original := append([]byte(nil), body.Bytes()...)
		require.Equal(t, value == "true", multimodalMediaClientWantsStream(original, writer.FormDataContentType()))
		upstream, contentType, err := forceMultimodalImageUpstreamStream(MultimodalMediaEndpointImagesEdits, original, writer.FormDataContentType())
		require.NoError(t, err)
		require.True(t, multimodalMediaClientWantsStream(upstream, contentType))
	}
}

func TestMultimodalVideoRejectsUnbillableResolutionAliases(t *testing.T) {
	for _, resolution := range []string{"1k", "2K", "4k", "1440p", "2160p"} {
		t.Run(resolution, func(t *testing.T) {
			body := []byte(`{"model":"seedance","resolution":"` + resolution + `"}`)
			info := ParseGrokMediaRequest("application/json", body)
			require.Equal(t, resolution, info.ResolutionRaw)
			require.ErrorIs(t, ValidateMultimodalVideoResolution(info.ResolutionRaw), ErrVideoPriceNotConfigured)
			svc, c, _, upstream, account := multimodalImageForwardFixture(t, body, "application/json", strings.NewReader(`{}`))
			_, err := svc.ForwardMultimodalMedia(context.Background(), c, account, MultimodalMediaEndpointVideosGenerations, "", body, "application/json", nil)
			require.True(t, errors.Is(err, ErrVideoPriceNotConfigured))
			require.Nil(t, upstream.lastReq, "reject before submitting work upstream")
		})
	}
}
