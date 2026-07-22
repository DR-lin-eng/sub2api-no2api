package service

import (
	"strings"
	"testing"
)

var grokFreeToolCacheIntentBenchmarkSink grokFreeToolCacheIntent

func BenchmarkGrokFreeToolCacheIntentLargeBody(b *testing.B) {
	body := []byte(`{"model":"grok-4.5","tools":[{"type":"function","name":"search"}],"tool_choice":"auto","input":"` + strings.Repeat("x", 8<<20) + `"}`)
	b.SetBytes(int64(len(body)))

	b.Run("intent_snapshot", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			grokFreeToolCacheIntentBenchmarkSink = inspectGrokFreeToolCacheIntent(body)
		}
	})

	b.Run("full_body_copy_then_snapshot", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			copyOfBody := append([]byte(nil), body...)
			grokFreeToolCacheIntentBenchmarkSink = inspectGrokFreeToolCacheIntent(copyOfBody)
		}
	})
}
