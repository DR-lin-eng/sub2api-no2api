#!/usr/bin/env python3
"""Reproduce the no-tool Unicode regression against the frozen draft adapter."""
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
BEFORE = '4e8c9908748f3920b37771f10c4af97ac484ee1c'
fixture = Path(tempfile.mkdtemp(prefix='sub2api-unicode-guard-'))
benchmark = r'''package service
import("strings";"testing")
func BenchmarkUnicodeNoTool(b *testing.B){body:=[]byte(`{"input":"`+strings.Repeat(`\u4e2d\u00e9\ud83d\ude80`,43690)+`","tools":[]}`);b.ReportAllocs();for b.Loop(){_,_,_=sanitizeOpenAIResponsesToolSchemaPatterns(body)}}
'''
summary = {'before_ref': BEFORE, 'scope': '1 MiB escaped Chinese, Latin-1 and emoji prompt, no tools; regex pass only', 'sources': {}}
for phase in ['before', 'after']:
    dest = fixture / phase
    dest.mkdir()
    sources = {}
    for name in ['openai_responses_tool_schema.go', 'openai_tool_schema_patterns.go', 'gateway_json_view.go']:
        path = 'backend/internal/application/service/' + name
        raw = subprocess.check_output(['git', 'show', BEFORE + ':' + path], cwd=REPO) if phase == 'before' else (REPO / path).read_bytes()
        (dest / name).write_bytes(raw)
        sources[path] = hashlib.sha256(raw).hexdigest()
    summary['sources'][phase] = sources
    for name in ['go.mod', 'go.sum']:
        (dest / name).write_bytes((REPO / 'backend' / name).read_bytes())
    (dest / 'platform.go').write_text('package service\nconst PlatformOpenAI="openai"\n')
    (dest / 'unicode_test.go').write_text(benchmark)
    command = ['docker', 'run', '--rm', '--network=none', '--cpus=1', '--memory=2g',
        '-v', str(dest) + ':/src:ro', '-v', '/private/tmp/sub2api-modcache:/go/pkg/mod',
        '-v', '/private/tmp/sub2api-gocache:/root/.cache/go-build',
        '-e', 'GOTOOLCHAIN=local', '-e', 'GOMAXPROCS=1', '-e', 'CGO_ENABLED=0',
        'golang:1.26.11-alpine', 'sh', '-c',
        'cp -a /src /work && cd /work && go version && go test -run=^$ -bench=BenchmarkUnicodeNoTool -benchmem -benchtime=200ms -count=3 -cpu=1']
    name = 'unicode-keyword-' + phase
    subprocess.run([sys.executable, str(ROOT / 'run_record.py'), name, *command], check=True)
    raw = gzip.decompress((ROOT / 'records' / (name + '.log.gz')).read_bytes()).decode()
    samples = [{'ns': float(ns), 'bytes': int(size), 'allocs': int(allocs)} for ns, size, allocs in
        re.findall(r'^BenchmarkUnicodeNoTool\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op$', raw, re.M)]
    assert len(samples) == 3
    summary[phase] = {'samples': samples, **{key: statistics.median(x[key] for x in samples) for key in ['ns', 'bytes', 'allocs']}}
(ROOT / 'UNICODE_GUARD.json').write_text(json.dumps(summary, indent=2) + '\n')
print(json.dumps(summary))
