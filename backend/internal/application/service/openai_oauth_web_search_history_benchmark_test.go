//go:build benchmark

package service

import (
	"strings"
	"testing"
)

func BenchmarkOAuthWebSearchHistory20261009(b *testing.B) {
	for _, size := range []struct {
		name  string
		bytes int
	}{{"small", 64}, {"8MiB", 8 << 20}} {
		for _, scenario := range []string{"no_marker", "top_declared", "additional_declared", "lite_insert"} {
			message := `{"type":"message","role":"user","content":"` + strings.Repeat("x", size.bytes) + `"}`
			body := []byte(`{"input":[` + message + `],"tools":[]}`)
			if scenario != "no_marker" {
				body = []byte(`{"input":[` + message + `,{"type":"web_search_call"},{"type":"compaction_trigger"}],"tools":[]}`)
			}
			if scenario == "top_declared" {
				body = []byte(`{"input":[` + message + `,{"type":"web_search_call"}],"tools":[{"type":"web_search"}]}`)
			}
			if scenario == "additional_declared" {
				body = []byte(`{"input":[` + message + `,{"type":"web_search_call"},{"type":"additional_tools","tools":[{"type":"web_search"}]}],"tools":[]}`)
			}
			for _, impl := range []struct {
				name string
				fn   func([]byte, bool) ([]byte, bool, error)
			}{{"upstream", upstreamReferenceEnsureOpenAIOAuthWebSearchToolForHistoryBody}, {"adapted", ensureOpenAIOAuthWebSearchToolForHistoryBody}} {
				b.Run(size.name+"/"+scenario+"/"+impl.name, func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						_, _, err := impl.fn(body, true)
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		}
	}
}
