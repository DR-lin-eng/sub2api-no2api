package service

import (
	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"strings"
	"testing"
)

var sync20261027Sink string

func BenchmarkUpstreamSync20261027(b *testing.B) {
	for _, tc := range []struct{ name, value string }{{"ordinary", "grok-4.6"}, {"oversized", strings.Repeat("界", 1<<20)}} {
		b.Run("model_audit_"+tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				sync20261027Sink = normalizeObservedUpstreamResponseModel(tc.value)
			}
		})
	}
	b.Run("legacy_model_alias", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			req := &apicompat.AnthropicRequest{Model: "gpt-5.4-high"}
			applyOpenAICompatModelNormalization(req)
			sync20261027Sink = req.Model
		}
	})
}
