package service

import (
	"strings"
	"testing"

	"github.com/tidwall/sjson"
)

// Frozen PR #8114 reference: the upstream implementation writes each thinking
// field separately, copying the full request once per field.
func upstreamGeminiThinkingReference(body, claude []byte) []byte {
	intent := parseClaudeThinkingIntent(claude)
	level := geminiThinkingLevelFor("gemini-3.8-flash", intent)
	body, err := sjson.SetBytes(body, "generationConfig.thinkingConfig.thinkingLevel", level)
	if err != nil {
		return body
	}
	updated, err := sjson.SetBytes(body, "generationConfig.thinkingConfig.includeThoughts", true)
	if err != nil {
		return body
	}
	return updated
}

func BenchmarkFollowupGeminiThinking(b *testing.B) {
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"` + strings.Repeat("x", 8<<20) + `"}]}]}`)
	claude := []byte(`{"output_config":{"effort":"high"}}`)
	b.Run("upstream_two_writes", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			upstreamGeminiThinkingReference(body, claude)
		}
	})
	b.Run("candidate_one_write", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			applyGeminiCompatThinkingConfig(body, claude, "gemini-3.8-flash", true)
		}
	})
}
