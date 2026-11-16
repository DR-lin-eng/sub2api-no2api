#!/usr/bin/env python3
"""Compare the frozen upstream regex parser with the current bounded adapter."""
from pathlib import Path
import gzip
import hashlib
import json
import re
import statistics
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parents[1]
fixture = Path(tempfile.mkdtemp(prefix='sub2api-followup-regex-'))
upstream = subprocess.check_output(['git', 'show',
    '3e8e3254d1a5c9f4e2bf66d05ca95ba57bdf6297:backend/internal/service/openai_responses_tool_schema.go'], cwd=REPO)
source_dir = REPO / 'backend/internal/application/service'
local_files = ['openai_responses_tool_schema.go', 'openai_tool_schema_patterns.go', 'gateway_json_view.go']
local = {name: (source_dir / name).read_bytes() for name in local_files}
benchmark = '''package service
import ("fmt"; "strings"; "testing")
func BenchmarkFollowupRegex(b *testing.B) {
 for _, count := range []int{0,100,1000} {
  tools := strings.TrimSuffix(strings.Repeat(`{"parameters":{"type":"object","pattern":"(?<name>abc)"}},`,count),",")
  body := []byte(`{"input":"`+strings.Repeat("x",1<<20)+`","tools":[`+tools+`]}`)
  b.Run(fmt.Sprint(count),func(b *testing.B){b.ReportAllocs();b.SetBytes(int64(len(body)))
   for b.Loop(){_,_,_ = sanitizeOpenAIResponsesToolSchemaPatterns(body)}
  })
 }
}
'''
stubs = '''package service
const (PlatformOpenAI="openai";PlatformAnthropic="anthropic";PlatformGrok="grok")
func IsCNProvider(string)bool{return false}
'''
for stage in ['upstream', 'adapted']:
    dest = fixture / stage
    dest.mkdir()
    (dest / 'platform.go').write_text(stubs)
    (dest / 'regex_test.go').write_text(benchmark)
    for name in ['go.mod', 'go.sum']:
        (dest / name).write_bytes((REPO / 'backend' / name).read_bytes())
    for name, raw in ({'upstream.go': upstream} if stage == 'upstream' else local).items():
        (dest / name).write_bytes(raw)
summary = {'scope': 'Regex compatibility pass only; fixed 1 MiB prompt and 0/100/1000 tool schemas. Not production throughput.',
    'source_sha256': {'upstream': hashlib.sha256(upstream).hexdigest(),
        **{name: hashlib.sha256(raw).hexdigest() for name, raw in local.items()}}}
for stage, source in [('upstream', 'upstream'), ('adapted', 'adapted'), ('upstream-repeat', 'upstream')]:
    name = 'performance-final-' + stage
    command = ['docker', 'run', '--rm', '--network=none', '--cpus=2', '--memory=2g',
        '-v', str(fixture / source) + ':/src:ro',
        '-v', '/private/tmp/sub2api-modcache:/go/pkg/mod',
        '-v', '/private/tmp/sub2api-gocache:/root/.cache/go-build',
        '-e', 'GOTOOLCHAIN=local', '-e', 'GOMAXPROCS=1', '-e', 'CGO_ENABLED=0',
        'golang:1.26.11-alpine', 'sh', '-c',
        'cp -a /src /work && cd /work && go version && go test -run=^$ -bench=^BenchmarkFollowupRegex$ -benchmem -benchtime=300ms -count=3 -cpu=1']
    subprocess.run([sys.executable, str(ROOT / 'run_record.py'), name, *command], check=True)
    log = gzip.decompress((ROOT / 'records' / (name + '.log.gz')).read_bytes()).decode()
    samples = {}
    for label, ns, size, allocs in re.findall(r'^(BenchmarkFollowupRegex/\d+)\s+\d+\s+([\d.]+) ns/op(?:\s+[\d.]+ MB/s)?\s+(\d+) B/op\s+(\d+) allocs/op$', log, re.M):
        samples.setdefault(label, []).append({'ns': float(ns), 'bytes': int(size), 'allocs': int(allocs)})
    assert len(samples) == 3 and all(len(values) == 3 for values in samples.values()), samples
    summary[stage] = {label: {'samples': values, **{key: statistics.median(v[key] for v in values)
        for key in ['ns', 'bytes', 'allocs']}} for label, values in samples.items()}
(ROOT / 'PERFORMANCE.json').write_text(json.dumps(summary, indent=2) + '\n')
print(json.dumps(summary))
