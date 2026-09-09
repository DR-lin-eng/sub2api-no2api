//go:build unit

package service

import (
	"bytes"
	"context"
	"testing"
)

func BenchmarkUpstreamSync20260909(b *testing.B) {
	b.Run("channel_cache_hit", func(b *testing.B) {
		svc := NewChannelService(&mockChannelRepository{}, nil, nil, nil)
		ctx := context.Background()
		if _, err := svc.loadCache(ctx); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, err := svc.loadCache(ctx); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("grok_without_field_256KiB", func(b *testing.B) {
		body := append([]byte(`{"messages":[{"role":"user","content":"`), bytes.Repeat([]byte("x"), 256<<10)...)
		body = append(body, []byte(`"}]}`)...)
		b.ReportAllocs()
		b.SetBytes(int64(len(body)))
		b.ResetTimer()
		for b.Loop() {
			if _, err := sanitizeGrokResponsesUnsupportedFields(body); err != nil {
				b.Fatal(err)
			}
		}
	})
}
