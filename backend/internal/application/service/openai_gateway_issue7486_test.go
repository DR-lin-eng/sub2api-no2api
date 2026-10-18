package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesHostedImageGenUsageCountsForBilling(t *testing.T) {
	// Issue #7486: Codex can put the generated-image count under
	// response.tool_usage.image_gen while leaving response.output empty.
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"response":{"usage":{"input_tokens":13171151,"output_tokens":0},"tool_usage":{"image_gen":{"images":1}},"output":[]}}`)),
	}

	result, err := (&OpenAIGatewayService{}).handleNonStreamingResponse(context.Background(), resp, c, nil, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.imageCount)
}

func TestOpenAIResponsesInputImageAloneDoesNotBecomeImageGenerationCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"input":[{"type":"message","content":[{"type":"input_image","image_url":"data:image/png;base64,AA=="}]}],"usage":{"input_tokens":12,"output_tokens":1},"output":[]}`)),
	}

	result, err := (&OpenAIGatewayService{}).handleNonStreamingResponse(context.Background(), resp, c, nil, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Zero(t, result.imageCount)
}
