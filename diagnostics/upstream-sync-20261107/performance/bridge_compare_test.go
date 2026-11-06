package apicompat

import (
	"fmt"
	"strings"
	"testing"
)

var syncCompareBridgeSink string

// ReportAllocs measures cumulative allocation traffic, not retained heap/RSS.
// A fixed 64-byte delta makes total-size scaling expose repeated prefix copies.
func BenchmarkUpstreamSyncRead64ByteDeltas(b *testing.B) {
	for _, size := range []int{32 << 10, 128 << 10, 512 << 10} {
		b.Run(fmt.Sprintf("%dKiB", size>>10), func(b *testing.B) {
			const prefix = `{"file_path":"`
			firstEvent := &ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: prefix + strings.Repeat("x", 64-len(prefix))}
			nextEvent := &ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: strings.Repeat("x", 64)}
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				state := NewResponsesEventToAnthropicState()
				state.MessageStartSent = true
				state.ContentBlockOpen = true
				state.CurrentBlockType = "tool_use"
				state.CurrentToolName = "Read"
				state.OutputIndexToBlockIdx[0] = 0
				ResponsesEventToAnthropicEvents(firstEvent, state)
				for received := 64; received < size; received += 64 {
					ResponsesEventToAnthropicEvents(nextEvent, state)
				}
				syncCompareBridgeSink = state.CurrentToolArgs
				if len(syncCompareBridgeSink) != size || state.CurrentToolHadDelta {
					b.Fatalf("unfinished Read arguments: bytes=%d, emitted=%v", len(syncCompareBridgeSink), state.CurrentToolHadDelta)
				}
			}
		})
	}
}
