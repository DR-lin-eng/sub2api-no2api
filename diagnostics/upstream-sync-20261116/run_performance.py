#!/usr/bin/env python3
"""Run the same bounded fixture sequentially on baseline/candidate/rollback."""
from pathlib import Path
import gzip
import json
import re
import statistics
import subprocess
import sys

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parents[1]
BASE = Path('/private/tmp/sub2api-sync-20261116-baseline/backend')
summary = {}
for stage, source in [('baseline', BASE), ('candidate', REPO / 'backend'), ('rollback', BASE)]:
    name = 'performance-final-' + stage
    command = ['docker', 'run', '--rm', '--network=none', '--cpus=2', '--memory=4g',
        '-v', str(source) + ':/src:ro', '-v', str(ROOT / 'performance') + ':/fixture:ro',
        '-v', '/private/tmp/sub2api-modcache:/go/pkg/mod',
        '-v', '/private/tmp/sub2api-gocache:/root/.cache/go-build',
        '-e', 'CGO_ENABLED=0', '-e', 'GOMAXPROCS=1', '-e', 'GOFLAGS=-p=1',
        '-e', 'GOMEMLIMIT=1400MiB', '-e', 'GOTOOLCHAIN=local',
        'golang:1.26.11-alpine', 'sh', '-c',
        'mkdir /work && cp -a /src /work/backend && '
        'cp /fixture/upstream_sync_benchmark_test.go /work/backend/internal/application/service/ && '
        'cd /work/backend && go version && '
        'go test -tags=unit ./internal/application/service -run=^$ -bench="^BenchmarkUpstreamSync(Metadata|CacheControlNoop|AnthropicStream)$" -benchmem -benchtime=300ms -count=3 -cpu=1']
    subprocess.run([sys.executable, str(ROOT / 'run_record.py'), name, *command], check=True)
    raw = gzip.decompress((ROOT / 'records' / (name + '.log.gz')).read_bytes()).decode()
    samples = {}
    pattern = r'^(BenchmarkUpstreamSync\S+)\s+\d+\s+([\d.]+) ns/op(?:\s+[\d.]+ MB/s)?\s+(\d+) B/op\s+(\d+) allocs/op$'
    for line in raw.splitlines():
        match = re.match(pattern, line)
        if match:
            label, ns, size, allocs = match.groups()
            samples.setdefault(label, []).append({'ns_per_op': float(ns), 'bytes_per_op': int(size), 'allocs_per_op': int(allocs)})
    assert len(samples) == 4 and all(len(values) == 3 for values in samples.values()), samples
    summary[stage] = {label: {'samples': values,
        'median_ns_per_op': statistics.median(v['ns_per_op'] for v in values),
        'median_bytes_per_op': statistics.median(v['bytes_per_op'] for v in values),
        'median_allocs_per_op': statistics.median(v['allocs_per_op'] for v in values)} for label, values in samples.items()}
(ROOT / 'PERFORMANCE.json').write_text(json.dumps(summary, indent=2) + '\n')
print(json.dumps(summary))
