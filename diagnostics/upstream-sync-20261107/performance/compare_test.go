package service

import (
	"bytes"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"strings"
	"testing"
)

var syncPerfSink any

// The same benchmark is copied into baseline and candidate containers. All
// inputs and externally observed results are identical on both revisions.
func BenchmarkUpstreamSync20261107(b *testing.B) {
	for _, size := range []int{4 << 10, 1 << 20, 8 << 20} {
		body := []byte(fmt.Sprintf(`{"model":"claude-sonnet-4","stream":true,"metadata":{"user_id":"fixture"},"system":[{"type":"text","text":"instructions","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":[{"type":"text","text":%q,"cache_control":{"type":"ephemeral"}}]}]}`, strings.Repeat("x", size)))
		parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformAnthropic)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("request-retry/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				attempt, err := parsed.CloneForBody(body)
				if err != nil {
					b.Fatal(err)
				}
				if err = attempt.ReplaceBody(body); err != nil {
					b.Fatal(err)
				}
				if err = attempt.ReplaceBody(body); err != nil {
					b.Fatal(err)
				}
				syncPerfSink = attempt.Model
			}
		})
		b.Run(fmt.Sprintf("initial-parse/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				p, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformAnthropic)
				if err != nil {
					b.Fatal(err)
				}
				syncPerfSink = p
			}
		})
		b.Run(fmt.Sprintf("cache-control/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				a, c, d, e := collectCacheControlPaths(body)
				syncPerfSink = len(a) + len(c) + len(d) + len(e)
			}
		})
	}
	body := []byte(`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","signature":"valid","thinking":"plan"},{"type":"text","text":"` + strings.Repeat("x", 1<<20) + `"}]}]}`)
	b.Run("thinking-no-change/1048576", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			out := filterThinkingBlocksInternal(body, false)
			if !bytes.Equal(out, body) {
				b.Fatal("changed history")
			}
			syncPerfSink = len(out)
		}
	})
}
