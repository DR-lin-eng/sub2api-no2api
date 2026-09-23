package service

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestReasoningTokensFromPayloadPreservesExplicitZeroAndRejectsFallbacks(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload string
		want    *int64
	}{
		{name: "responses", payload: `{"response":{"usage":{"output_tokens_details":{"reasoning_tokens":256}}}}`, want: reasoningTokenPtr(256)},
		{name: "chat completions", payload: `{"usage":{"completion_tokens_details":{"reasoning_tokens":80}}}`, want: reasoningTokenPtr(80)},
		{name: "gemini", payload: `{"usageMetadata":{"thoughtsTokenCount":200}}`, want: reasoningTokenPtr(200)},
		{name: "explicit zero", payload: `{"usage":{"reasoning_tokens":0}}`, want: reasoningTokenPtr(0)},
		{name: "missing", payload: `{"usage":{"output_tokens":20,"total_tokens":25}}`, want: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			var payload map[string]any
			require.NoError(t, json.Unmarshal([]byte(test.payload), &payload))
			require.Equal(t, test.want, reasoningTokensFromPayload(payload))
		})
	}
}

func reasoningTokenPtr(value int64) *int64 { return &value }

func TestReasoningTokensAreCapturedFromSSETerminalUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name   string
		stream string
		read   func(*AccountTestService, *gin.Context, io.Reader) error
		want   int64
	}{
		{name: "responses", stream: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"21\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"output_tokens_details\":{\"reasoning_tokens\":256}}}}\n\n", read: (*AccountTestService).processOpenAIStream, want: 256},
		{name: "chat completions", stream: "data: {\"choices\":[{\"delta\":{\"content\":\"21\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"completion_tokens_details\":{\"reasoning_tokens\":80}}}\n\ndata: [DONE]\n\n", read: (*AccountTestService).processOpenAIChatCompletionsStream, want: 80},
		{name: "gemini", stream: "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"21\"}]},\"finishReason\":\"STOP\"}]}\n\ndata: {\"usageMetadata\":{\"thoughtsTokenCount\":120}}\n\n", read: (*AccountTestService).processGeminiStream, want: 120},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			require.NoError(t, test.read(&AccountTestService{}, ctx, strings.NewReader(test.stream)))
			_, _, tokens := parseTestSSEOutputWithReasoning(recorder.Body.String())
			require.NotNil(t, tokens)
			require.Equal(t, test.want, *tokens)
		})
	}
}
