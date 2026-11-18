package service

import (
	"strings"
	"testing"
)

var syncUsageSink ClaudeUsage

// Identical input and caller for frozen main and candidate; this measures only
// parsing and merging, without a network, scheduler, database or billing worker.
func BenchmarkSyncAnthropicUsage(b *testing.B) {
	for _, sample := range []struct{ name, event string }{
		{"text_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"` + strings.Repeat("x", 256) + `"}}`},
		{"cache_snapshot", `{"type":"message_delta","usage":{"input_tokens":100,"output_tokens":12,"cache_creation_input_tokens":40,"cache_read_input_tokens":20,"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":30}}}`},
	} {
		b.Run(sample.name, func(b *testing.B) {
			service := &GatewayService{}
			var usage ClaudeUsage
			b.ReportAllocs()
			b.SetBytes(int64(len(sample.event)))
			for i := 0; i < b.N; i++ {
				service.parseSSEUsagePassthrough(sample.event, &usage)
			}
			syncUsageSink = usage
		})
	}
}
