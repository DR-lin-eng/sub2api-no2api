#!/usr/bin/env python3
"""Sample the isolated model matcher with frozen upstream source, offline."""
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parents[1]
UPSTREAM = 'b0fa9f3e6eeab1968a53e1289b69daa4bb40ca2b'
source = subprocess.check_output(['git', 'show', UPSTREAM + ':backend/internal/service/billing_service.go'], cwd=REPO, text=True)
block = source[source.index('\t// 硅基流动 Qwen3-Embedding 系列（纯文本向量化）。'):]
block = block[:block.index('\t// 火山方舟 豆包 Embedding')]
block = re.sub(r'return s\.fallbackPrices\["([^"]+)"\]', r'return "\1"', block)
upstream = 'package service\nimport "strings"\nfunc upstreamQwen3InputPricingID(modelLower string) string {\n' + block + '\nreturn ""\n}\n'
adapted = (REPO / 'backend/internal/application/service/billing_qwen3_input.go').read_text()
benchmark = '''package service
import "testing"
// Only the card field used by the isolated production source is needed here.
type ModelPricing struct { InputPricePerToken float64 }
type BillingService struct { fallbackPrices map[string]*ModelPricing }
var sink string
func BenchmarkQwenMatcher(b *testing.B) {
    for _, impl := range []struct { name string; fn func(string) string }{
        {"upstream", upstreamQwen3InputPricingID}, {"adapted", qwen3InputPricingID},
    } {
        for _, item := range []struct { name, model string }{
            {"embedding", "qwen/qwen3-embedding-8b"},
            {"reranker", "qwen/qwen3-reranker-0-6b"},
            {"unrelated", "gpt-6.1-sol"},
        } {
            b.Run(impl.name+"/"+item.name, func(b *testing.B) {
                b.ReportAllocs()
                for i := 0; i < b.N; i++ { sink = impl.fn(item.model) }
            })
        }
    }
}
'''
with tempfile.TemporaryDirectory(prefix='sub2api-qwen-bench-', dir='/private/tmp') as path:
    work = Path(path)
    (work / 'adapted.go').write_text(adapted)
    (work / 'upstream.go').write_text(upstream)
    (work / 'matcher_test.go').write_text(benchmark)
    command = ['docker', 'run', '--rm', '--network=none', '--cpus=1', '--memory=512m',
               '-v', str(work) + ':/bench:ro', '-w', '/bench', '-e', 'GO111MODULE=off',
               '-e', 'GOTOOLCHAIN=local', '-e', 'GOMAXPROCS=1', 'golang:1.26.11-alpine',
               'go', 'test', '-run=^$', '-bench=.', '-benchmem', '-benchtime=300ms', '-count=3']
    raw = subprocess.check_output(command)
    import gzip
    (ROOT / 'records' / 'matcher-benchmark.log.gz').write_bytes(gzip.compress(raw))
    report = {'upstream': UPSTREAM, 'upstream_source_sha256': hashlib.sha256(source.encode()).hexdigest(),
              'extracted_matcher_sha256': hashlib.sha256(upstream.encode()).hexdigest(),
              'production_source_sha256': hashlib.sha256(adapted.encode()).hexdigest(),
              'command': command, 'exit_code': 0,
              'output_sha256': hashlib.sha256(raw).hexdigest(),
              'scope': 'Isolated lowercase model-ID matcher only; no gateway throughput or network claim.',
              'samples': raw.decode().splitlines()}
    (ROOT / 'PERFORMANCE.json').write_text(json.dumps(report, indent=2) + '\n')
    print(raw.decode())
