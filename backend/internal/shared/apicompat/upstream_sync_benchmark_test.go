package apicompat

import (
	"encoding/json"
	"strings"
	"testing"
)

func BenchmarkSyncEffectiveResponsesTools(b *testing.B) {
	for _, discovery := range []bool{false, true} {
		name := "ordinary_history"
		suffix := ""
		if discovery {
			name = "discovery_history"
			suffix = `,{"type":"tool_search_output","call_id":"search_1","status":"completed","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`
		}
		req := &ResponsesRequest{Tools: []ResponsesTool{{Type: "tool_search"}}, Input: json.RawMessage(`[{"type":"message","role":"user","content":"` + strings.Repeat("x", 262144) + `"}` + suffix + `]`)}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(req.Input)))
			for b.Loop() {
				if _, err := EffectiveResponsesTools(req); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
