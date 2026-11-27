//go:build unit

package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newTestToolRewriteContext(pairs [][2]string) *gin.Context {
	c := &gin.Context{}
	if len(pairs) > 0 {
		rw := &ToolNameRewrite{
			Forward:        make(map[string]string, len(pairs)),
			Reverse:        make(map[string]string, len(pairs)),
			ReverseOrdered: pairs,
		}
		for _, p := range pairs {
			rw.Reverse[p[0]] = p[1]
			rw.Forward[p[1]] = p[0]
		}
		c.Set(toolNameRewriteKey, rw)
	}
	return c
}

func TestToolNameStreamRestorer_AnthropicSplitDeltas(t *testing.T) {
	c := newTestToolRewriteContext([][2]string{
		{"analyze_mcp00", "mcp__filesystem__read_file"},
		{"compute_bas01", "bash"},
	})
	restorer := newToolNameStreamRestorer(c)

	blocks := []string{
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Calling anal\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"yze_mcp\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"00 and cc_ses\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"s_list now; trailing comp\"}}\n\n",
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
	}

	var out strings.Builder
	for _, b := range blocks {
		for _, part := range restorer.RestoreAnthropicSSEBlock(b) {
			out.WriteString(part)
		}
	}
	for _, part := range restorer.FlushPending() {
		out.WriteString(part)
	}

	rendered := out.String()
	require.Contains(t, rendered, "mcp__filesystem__read_file")
	require.Contains(t, rendered, "sessions_list")
	require.Contains(t, rendered, `"text":"sessions_list now; trailing "`)
	require.Contains(t, rendered, `"text":"comp"`)
	require.NotContains(t, rendered, "analyze_mcp00")
	require.NotContains(t, rendered, "cc_sess_list")
	stopIdx := strings.Index(rendered, `"content_block_stop"`)
	trailIdx := strings.Index(rendered, `"text":"comp"`)
	require.Greater(t, trailIdx, 0)
	require.Less(t, trailIdx, stopIdx, "held-back partial prefix must flush before content_block_stop")
}

func TestToolNameStreamRestorer_AnthropicInputJSONDeltaSplit(t *testing.T) {
	c := newTestToolRewriteContext([][2]string{
		{"lookup_ sea02", "search_docs"},
		{"lookup_sea02", "search_docs"},
	})
	restorer := newToolNameStreamRestorer(c)

	b1 := "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"tool\\\":\\\"lookup_\"}}\n\n"
	b2 := "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"sea02\\\"}\"}}\n\n"

	var combined strings.Builder
	for _, p := range restorer.RestoreAnthropicSSEBlock(b1) {
		combined.WriteString(p)
	}
	for _, p := range restorer.RestoreAnthropicSSEBlock(b2) {
		combined.WriteString(p)
	}
	require.Contains(t, combined.String(), `search_docs`)
	require.NotContains(t, combined.String(), `lookup_sea02`)
}

func TestToolNameStreamRestorer_ChatAndResponsesBridges(t *testing.T) {
	c := newTestToolRewriteContext([][2]string{
		{"query_get00", "get_weather"},
	})

	ccRestorer := newToolNameStreamRestorer(c)
	part1 := "use query_"
	part2 := "get00 please"
	finish := "stop"
	tcIdx := 0
	chunks := []apicompat.ChatCompletionsChunk{
		{ID: "1", Object: "chat.completion.chunk", Choices: []apicompat.ChatChunkChoice{{Index: 0, Delta: apicompat.ChatDelta{Content: &part1}}}},
		{ID: "1", Object: "chat.completion.chunk", Choices: []apicompat.ChatChunkChoice{{Index: 0, Delta: apicompat.ChatDelta{Content: &part2}}}},
		{ID: "1", Object: "chat.completion.chunk", Choices: []apicompat.ChatChunkChoice{{Index: 0, Delta: apicompat.ChatDelta{ToolCalls: []apicompat.ChatToolCall{{Index: &tcIdx, Function: apicompat.ChatFunctionCall{Name: "query_get00", Arguments: `{"tool":"query_`}}}}}}},
		{ID: "1", Object: "chat.completion.chunk", Choices: []apicompat.ChatChunkChoice{{Index: 0, Delta: apicompat.ChatDelta{ToolCalls: []apicompat.ChatToolCall{{Index: &tcIdx, Function: apicompat.ChatFunctionCall{Arguments: `get00"}`}}}}, FinishReason: &finish}}},
	}
	var ccText strings.Builder
	var ccToolName string
	var ccToolArgs strings.Builder
	for _, ch := range chunks {
		for _, out := range ccRestorer.RestoreChatChunk(ch) {
			if out.Choices[0].Delta.Content != nil {
				ccText.WriteString(*out.Choices[0].Delta.Content)
			}
			for _, tc := range out.Choices[0].Delta.ToolCalls {
				if tc.Function.Name != "" {
					ccToolName = tc.Function.Name
				}
				ccToolArgs.WriteString(tc.Function.Arguments)
			}
		}
	}
	require.Equal(t, "use get_weather please", ccText.String())
	require.Equal(t, "get_weather", ccToolName)
	require.Equal(t, `{"tool":"get_weather"}`, ccToolArgs.String())

	respRestorer := newToolNameStreamRestorer(c)
	events := []apicompat.ResponsesStreamEvent{
		{Type: "response.function_call_arguments.delta", ItemID: "fc_1", Delta: `{"delegate":"query_`},
		{Type: "response.function_call_arguments.delta", ItemID: "fc_1", Delta: `get00"}`},
		{Type: "response.function_call_arguments.done", ItemID: "fc_1", Arguments: `{"delegate":"query_get00"}`},
		{
			Type: "response.completed",
			Response: &apicompat.ResponsesResponse{
				Output: []apicompat.ResponsesOutput{{
					Type:      "message",
					Content:   []apicompat.ResponsesContentPart{{Type: "output_text", Text: "Called query_get00"}},
					Summary:   []apicompat.ResponsesSummary{{Type: "summary_text", Text: "Used query_get00"}},
					Name:      "query_get00",
					Arguments: `{"delegate":"query_get00"}`,
				}},
			},
		},
	}
	var deltaText strings.Builder
	var doneArgs string
	var completedItem apicompat.ResponsesOutput
	for _, evt := range events {
		for _, out := range respRestorer.RestoreResponsesEvent(evt) {
			if out.Type == "response.function_call_arguments.delta" {
				deltaText.WriteString(out.Delta)
			}
			if out.Type == "response.function_call_arguments.done" {
				doneArgs = out.Arguments
			}
			if out.Type == "response.completed" && out.Response != nil && len(out.Response.Output) == 1 {
				completedItem = out.Response.Output[0]
			}
		}
	}
	require.Equal(t, `{"delegate":"get_weather"}`, deltaText.String())
	require.Equal(t, `{"delegate":"get_weather"}`, doneArgs)
	require.Equal(t, "get_weather", completedItem.Name)
	require.Equal(t, `{"delegate":"get_weather"}`, completedItem.Arguments)
	require.Equal(t, "Called get_weather", completedItem.Content[0].Text)
	require.Equal(t, "Used get_weather", completedItem.Summary[0].Text)
}

func BenchmarkToolNameStreamRestorer_ProseDelta32Tools(b *testing.B) {
	pairs := make([][2]string, 32)
	for i := range 32 {
		prefix := fakeToolNamePrefixes[i%len(fakeToolNamePrefixes)]
		pairs[i] = [2]string{fmt.Sprintf("%stool%02d", prefix, i), fmt.Sprintf("mcp__server__tool_%02d", i)}
	}
	c := newTestToolRewriteContext(pairs)
	rw := toolNameRewriteFromContext(c)
	block := "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Here is an ordinary streaming prose token without rewritten tool names.\"}}\n\n"
	proseBytes := []byte("Here is an ordinary streaming prose token without rewritten tool names.")

	b.Run("StreamRestorer_SSEBlock", func(b *testing.B) {
		restorer := newToolNameStreamRestorer(c)
		b.ReportAllocs()
		for b.Loop() {
			out := restorer.RestoreAnthropicSSEBlock(block)
			if len(out) != 1 {
				b.Fatalf("unexpected len %d", len(out))
			}
		}
	})

	b.Run("StreamRestorer_RestoreInto", func(b *testing.B) {
		restorer := newToolNameStreamRestorer(c)
		dst := make([]byte, 0, 128)
		b.ReportAllocs()
		for b.Loop() {
			dst = restorer.restoreInto(dst[:0], proseBytes)
		}
	})

	b.Run("LegacyPerPair_RestoreToolNamesInBytes", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = restoreToolNamesInBytes(proseBytes, rw)
		}
	})
}
