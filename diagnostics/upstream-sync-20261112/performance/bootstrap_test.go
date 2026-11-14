package apicompat

import (
	"strings"
	"testing"
)

var syncBootstrapResult []byte

func BenchmarkSyncBootstrapOrdinary1MiB(b *testing.B) {
	body := []byte(`{"model":"gpt-5.4","input":[{"role":"user","content":"` + strings.Repeat("x", 1<<20) + `"}]}`)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for range b.N {
		syncBootstrapResult, _, _ = NormalizeCodexCallOutputBootstrap(body)
	}
}
