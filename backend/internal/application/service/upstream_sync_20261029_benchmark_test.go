package service

import (
	"strings"
	"testing"
)

func BenchmarkUpstreamSyncResponsesTerminalExtraction(b *testing.B) {
	progress := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"" + strings.Repeat("x", 1024) + "\"}\n\n"
	body := strings.Repeat(progress, 128) + "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"fixture\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":20}}}\n\n"
	for _, entry := range []struct {
		name    string
		extract func(string) ([]byte, bool)
	}{
		{"existing_completed", extractCodexFinalResponse},
		{"all_terminal_statuses", extractOpenAINonStreamingFinalResponse},
	} {
		b.Run(entry.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for i := 0; i < b.N; i++ {
				if _, ok := entry.extract(body); !ok {
					b.Fatal("missing terminal")
				}
			}
		})
	}
}
