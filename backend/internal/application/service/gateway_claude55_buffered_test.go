package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestClaude55BufferedBridgesRetainFragmentedSignaturesAndUsage(t *testing.T) {
	events := []apicompat.AnthropicStreamEvent{
		{Type: "message_start", Message: &apicompat.AnthropicResponse{ID: "msg_signed", Model: "alias", Usage: apicompat.AnthropicUsage{InputTokens: 7}}},
		{Type: "content_block_start", ContentBlock: &apicompat.AnthropicContentBlock{Type: "thinking"}},
		{Type: "content_block_delta", Delta: &apicompat.AnthropicDelta{Type: "thinking_delta", Thinking: "thought"}},
		{Type: "content_block_delta", Delta: &apicompat.AnthropicDelta{Type: "signature_delta", Signature: "provider-"}},
		{Type: "content_block_delta", Delta: &apicompat.AnthropicDelta{Type: "signature_delta", Signature: "signature"}},
		{Type: "content_block_stop"},
		{Type: "message_delta", Delta: &apicompat.AnthropicDelta{StopReason: "end_turn"}, Usage: &apicompat.AnthropicUsage{OutputTokens: 11}},
		{Type: "message_stop"},
	}
	index := 0
	var wire strings.Builder
	for _, event := range events {
		event.Index = &index
		raw, _ := json.Marshal(event)
		wire.WriteString("event: " + event.Type + "\ndata: " + string(raw) + "\n\n")
	}
	for _, native := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(wire.String()))}
		if native {
			result, err := (&OpenAIGatewayService{}).handleResponsesBufferedFromNativeAnthropic(resp, c, "alias", "claude-opus-5-5", "claude-opus-5-5", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
			require.NoError(t, err)
			require.Equal(t, 7, result.Usage.InputTokens)
			require.Equal(t, 11, result.Usage.OutputTokens)
		} else {
			result, err := (&GatewayService{}).handleResponsesBufferedStreamingResponse(resp, c, "alias", "claude-opus-5-5", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
			require.NoError(t, err)
			require.Equal(t, 7, result.Usage.InputTokens)
			require.Equal(t, 11, result.Usage.OutputTokens)
		}
		require.Equal(t, "alias", gjson.Get(recorder.Body.String(), "model").String())
		input := json.RawMessage(gjson.Get(recorder.Body.String(), "output").Raw)
		replay, err := apicompat.ResponsesToAnthropicRequest(&apicompat.ResponsesRequest{Model: "claude-opus-5-5", Input: input})
		require.NoError(t, err)
		require.Equal(t, "provider-signature", gjson.GetBytes(replay.Messages[0].Content, "0.signature").String())
	}
}
