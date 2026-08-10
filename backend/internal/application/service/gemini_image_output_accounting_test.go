//go:build unit

package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const geminiTestPNG = "iVBORw0KGgoAAAANSUhEUg=="

func newGeminiImageTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost,
		"/v1beta/models/nana-banana-2:generateContent", strings.NewReader("{}"))
	return c
}

func geminiImageResponse(parts string) string {
	return `{"candidates":[{"content":{"role":"model","parts":[` + parts + `]},"finishReason":"STOP"}]}`
}

func TestCountGeminiInlineImageOutputs(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    int
	}{
		{name: "camelCase", payload: geminiImageResponse(`{"inlineData":{"mimeType":"image/png","data":"` + geminiTestPNG + `"}}`), want: 1},
		{name: "snake_case", payload: geminiImageResponse(`{"inline_data":{"mime_type":"image/png","data":"` + geminiTestPNG + `"}}`), want: 1},
		{name: "multiple", payload: geminiImageResponse(`{"inlineData":{"mimeType":"image/png","data":"` + geminiTestPNG + `"}},{"inlineData":{"mimeType":"image/webp","data":"` + geminiTestPNG + `"}}`), want: 2},
		{name: "non_image", payload: geminiImageResponse(`{"inlineData":{"mimeType":"audio/mpeg","data":"` + geminiTestPNG + `"}}`), want: 0},
		{name: "empty_data", payload: geminiImageResponse(`{"inlineData":{"mimeType":"image/png","data":""}}`), want: 0},
		{name: "invalid_json", payload: "not-json", want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, countGeminiInlineImageOutputs([]byte(tc.payload)))
		})
	}
}

func TestObserveGeminiImageOutputsUsesLargestPayload(t *testing.T) {
	c := newGeminiImageTestContext(t)
	beginGeminiImageOutputObservation(c)
	oneImage := []byte(geminiImageResponse(`{"inlineData":{"mimeType":"image/png","data":"` + geminiTestPNG + `"}}`))
	for range 4 {
		observeGeminiImageOutputs(c, oneImage)
	}
	require.Equal(t, 1, observedGeminiImageOutputs(c), "cumulative SSE chunks must not be double counted")

	observeGeminiImageOutputs(c, []byte(geminiImageResponse(`{"inlineData":{"mimeType":"image/png","data":"`+geminiTestPNG+`"}},{"inlineData":{"mimeType":"image/png","data":"`+geminiTestPNG+`"}}`)))
	observeGeminiImageOutputs(c, []byte(`{"usageMetadata":{"promptTokenCount":9}}`))
	require.Equal(t, 2, observedGeminiImageOutputs(c))
}

func TestBeginGeminiImageOutputObservationResetsPerForward(t *testing.T) {
	c := newGeminiImageTestContext(t)
	beginGeminiImageOutputObservation(c)
	observeGeminiImageOutputs(c, []byte(geminiImageResponse(`{"inlineData":{"mimeType":"image/png","data":"`+geminiTestPNG+`"}}`)))
	require.Equal(t, 1, observedGeminiImageOutputs(c))

	beginGeminiImageOutputObservation(c)
	require.Zero(t, observedGeminiImageOutputs(c))
}

func TestResolveGeminiImageCount(t *testing.T) {
	t.Run("observed custom model", func(t *testing.T) {
		c := newGeminiImageTestContext(t)
		beginGeminiImageOutputObservation(c)
		observeGeminiImageOutputs(c, []byte(geminiImageResponse(`{"inlineData":{"mimeType":"image/png","data":"`+geminiTestPNG+`"}}`)))
		require.False(t, isImageGenerationModel("nana-banana-2"))
		require.Equal(t, 1, resolveGeminiImageCount(c, "nana-banana-2", "nana-banana-2"))
	})

	t.Run("mapped model fallback", func(t *testing.T) {
		c := newGeminiImageTestContext(t)
		beginGeminiImageOutputObservation(c)
		require.Equal(t, 1, resolveGeminiImageCount(c, "my-image-alias", "gemini-2.5-flash-image"))
	})
}

func TestHandleNativeNonStreamingResponseFeedsImageCounter(t *testing.T) {
	c := newGeminiImageTestContext(t)
	beginGeminiImageOutputObservation(c)
	body := geminiImageResponse(`{"inlineData":{"mimeType":"image/png","data":"` + geminiTestPNG + `"}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	usage, err := (&GeminiMessagesCompatService{}).handleNativeNonStreamingResponse(c, resp, false)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 1, observedGeminiImageOutputs(c))
}
