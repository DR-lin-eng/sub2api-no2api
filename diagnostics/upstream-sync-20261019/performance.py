#!/usr/bin/env python3
"""Run consecutive baseline/candidate benchmarks after other checks finish."""
import json
from pathlib import Path
import re
import statistics
import subprocess
import sys

ROOT = Path(__file__).resolve().parent
results = {}
for stage, directory in [('baseline', 'baseline'), ('modified', 'repo')]:
    label = 'performance-' + stage + '-isolated'
    command = ['docker', 'run', '--rm', '-e', 'GOMAXPROCS=2', '-e', 'GOMEMLIMIT=3GiB', '-e', 'GOFLAGS=-p=1',
        '-v', str(ROOT / directory / 'backend') + ':/app', '-v', 'sub2api-upstream-go-mod-20261016:/go/pkg/mod',
        '-v', 'sub2api-upstream-go-build-20261016:/root/.cache/go-build', '-w', '/app', 'golang:1.26.9-alpine', 'sh', '-c',
        'export PATH=/usr/local/go/bin:$PATH; go test -tags unit -run ^$ -bench ^BenchmarkUpstreamSyncGeminiStream$ -benchtime=500ms -benchmem -count=3 ./internal/application/service']
    subprocess.run([sys.executable, str(ROOT / 'record.py'), label, str(ROOT / directory), *command], check=True)
    record = json.loads((ROOT / 'records' / (label + '.record.json')).read_text())
    groups = {}
    for line in record['stdout'].splitlines():
        match = re.match(r'BenchmarkUpstreamSyncGeminiStream/(.+)-2\s+\d+\s+(\d+) ns/op\s+[\d.]+ MB/s\s+(\d+) B/op\s+(\d+) allocs/op', line)
        if match:
            groups.setdefault(match[1], []).append([int(value) for value in match.groups()[1:]])
    assert len(groups) == 4 and all(len(samples) == 3 for samples in groups.values())
    results[stage] = {name: {'samples': values, 'median_ns': statistics.median(v[0] for v in values),
        'median_bytes': statistics.median(v[1] for v in values), 'median_allocs': statistics.median(v[2] for v in values)}
        for name, values in groups.items()}
results['change_percent'] = {name: round((results['modified'][name]['median_ns'] / values['median_ns'] - 1) * 100, 2)
    for name, values in results['baseline'].items()}
results['scope'] = 'Consecutive Docker Go 1.26.9 linux/arm64 GOMAXPROCS=2, 3 samples/500ms; completed after other compile/test workloads. Sink implements StringWriter as Gin/net/http do. Successful stream microbenchmarks, not production throughput.'
results['excluded_record'] = 'performance-modified-final.record.json: ran concurrently with compiler/linter processes and is not comparable.'
(ROOT / 'PERFORMANCE.json').write_text(json.dumps(results, indent=2) + '\n')
print(json.dumps(results['change_percent']))
