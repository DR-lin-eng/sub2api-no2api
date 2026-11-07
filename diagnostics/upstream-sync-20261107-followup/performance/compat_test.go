package apicompat

import (
	"strings"
	"testing"
)

var followupReasoningSink *ChatCompletionsResponse

func BenchmarkSyncBufferedReasoning(b *testing.B) {
	response := &ResponsesResponse{Status: "completed"}
	for i := 0; i < 1024; i++ {
		response.Output = append(response.Output, ResponsesOutput{Type: "reasoning", Summary: []ResponsesSummary{{Type: "summary_text", Text: strings.Repeat("x", 256)}}})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		followupReasoningSink = ResponsesToChatCompletions(response, "fixture")
	}
}
