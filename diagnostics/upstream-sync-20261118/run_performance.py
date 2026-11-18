#!/usr/bin/env python3
"""Compare actual raw usage owners in the same Docker toolchain and CPU budget."""
import gzip
import hashlib
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile

root = Path(__file__).resolve().parent
repo = root.parents[1]
baseline = json.loads((root / 'UPSTREAM_REVIEW_INPUT.json').read_text())['baseline']
reports = []
with tempfile.TemporaryDirectory(prefix='sub2api-sync-perf-20261118-', dir='/private/tmp') as name:
    checkout = Path(name)
    archive = subprocess.Popen(['git', 'archive', baseline, 'backend'], cwd=repo, stdout=subprocess.PIPE)
    with tarfile.open(fileobj=archive.stdout, mode='r|') as source:
        source.extractall(checkout, filter='data')
    assert archive.wait() == 0
    for label, source in [('baseline', checkout), ('candidate', repo)]:
        command = ['docker', 'run', '--rm', '--network=none', '--cpus=1', '--memory=4g',
                   '-v', str(source / 'backend') + ':/src:ro',
                   '-v', str(root / 'usage_benchmark_test.go') + ':/bench.go:ro',
                   '-v', '/private/tmp/sub2api-modcache:/go/pkg/mod',
                   '-v', '/private/tmp/sub2api-gocache:/root/.cache/go-build',
                   '-e', 'CGO_ENABLED=0', '-e', 'GOMAXPROCS=1', '-e', 'GOMEMLIMIT=1800MiB',
                   '-e', 'GOGC=50', '-e', 'GOFLAGS=-p=1', '-e', 'GOTOOLCHAIN=local',
                   'golang:1.26.11-alpine', 'sh', '-c',
                   'mkdir /work && cp -a /src /work/backend && '
                   'cp /bench.go /work/backend/internal/application/service/sync_benchmark_test.go && '
                   'cd /work/backend && go test -run=^$ -bench=BenchmarkSyncAnthropicUsage '
                   '-benchmem -benchtime=500ms -count=5 ./internal/application/service']
        result = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        raw = result.stdout
        (root / 'records' / ('performance-' + label + '.log.gz')).write_bytes(gzip.compress(raw))
        reports.append({'label': label, 'command': command, 'exit_code': result.returncode,
                        'output_sha256': hashlib.sha256(raw).hexdigest(),
                        'samples': [line for line in raw.decode(errors='replace').splitlines()
                                    if line.startswith('Benchmark')]})
        print(label, raw.decode(errors='replace')[-2000:], flush=True)
        if result.returncode:
            raise SystemExit(result.returncode)
(root / 'PERFORMANCE.json').write_text(json.dumps({
    'baseline': baseline, 'scope': 'Raw usage parser only; no production throughput claim.',
    'runs': reports,
}, indent=2) + '\n')
