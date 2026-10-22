//go:build unit

package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func BenchmarkUpstream20261013(b *testing.B) {
	for _, blocks := range []int{1, 1024} {
		b.Run(fmt.Sprintf("ToolText/%d", blocks), func(b *testing.B) {
			content := make([]any, blocks)
			for i := range content {
				content[i] = map[string]any{"type": "text", "text": strings.Repeat("x", 1024)}
			}
			input := []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "t", "content": content}}}}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_, err := convertClaudeMessagesToGeminiContents(input, map[string]string{"t": "inspect"})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
	b.Run("RateLimitText", func(b *testing.B) {
		account := &Account{Platform: PlatformAntigravity}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			account.isModelRateLimitedWithContext(context.Background(), "gemini-3-flash")
		}
	})
	b.Run("LegacyRules", func(b *testing.B) {
		account := &Account{Credentials: map[string]any{"temp_unschedulable_enabled": true, "temp_unschedulable_rules": []any{map[string]any{"error_code": 503, "keywords": []any{"busy"}, "duration_minutes": 10}}}}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			matchTempUnschedulableRules(account, 503, []byte("upstream busy"))
		}
	})
}
