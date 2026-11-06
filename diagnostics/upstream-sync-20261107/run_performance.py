#!/usr/bin/env python3
"""Compare identical fixtures on the frozen main baseline and current source."""
from pathlib import Path
import json
import re
import statistics
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[2]
OUT = Path(__file__).resolve().parent
BASELINE = 'e0ecc7c85ce7c2f8e561e8da12f6cdcaae7c489c'


def run(source, role):
    command = ['docker', 'run', '--rm', '--network', 'none', '--cpus=2', '--memory=4g',
        '-v', str(source) + ':/src:ro', '-v', str(OUT / 'performance') + ':/fixture:ro',
        '-v', 'sub2api-gomod:/go/pkg/mod', '-v', 'sub2api-gocache:/root/.cache/go-build',
        '-e', 'CGO_ENABLED=0', '-e', 'GOMAXPROCS=1', '-e', 'GOMEMLIMIT=1400MiB',
        '-e', 'GOGC=50', '-e', 'GOFLAGS=-p=1', '-e', 'GOTOOLCHAIN=local', '-e', 'GOPROXY=off',
        'golang:1.26.10-alpine', 'sh', '-c',
        'cp -a /src /work && cp /fixture/compare_test.go /work/internal/application/service/zz_sync_compare_test.go && '
        'cp /fixture/bridge_compare_test.go /work/internal/shared/apicompat/zz_sync_compare_test.go && '
        'cd /work && go test -tags=unit ./internal/application/service ./internal/shared/apicompat '
        '-run "^$" -bench "^BenchmarkUpstreamSync" -benchmem -benchtime=200ms -count=3 -cpu=1']
    log = OUT / 'records' / (role + '-performance.log')
    with log.open('w') as stream:
        subprocess.run(command, cwd=ROOT, stdout=stream, stderr=subprocess.STDOUT, check=True)
    samples = {}
    for line in log.read_text().splitlines():
        if not line.startswith('BenchmarkUpstreamSync'):
            continue
        parts = line.split()
        def metric(unit):
            return float(parts[parts.index(unit)-1])
        samples.setdefault(parts[0], []).append({'ns_op':metric('ns/op'), 'bytes_op':metric('B/op'), 'allocs_op':metric('allocs/op')})
    return samples


with tempfile.TemporaryDirectory(prefix='sub2api-sync-20261107-bench-') as directory:
    directory = Path(directory)
    archive = directory / 'baseline.tar'
    with archive.open('wb') as stream:
        subprocess.run(['git', 'archive', BASELINE, 'backend'], cwd=ROOT, stdout=stream, check=True)
    with tarfile.open(archive) as stream:
        stream.extractall(directory, filter='data')
    baseline = run(directory / 'backend', 'baseline')
    candidate = run(ROOT / 'backend', 'candidate')
report = {'baseline':BASELINE, 'environment':{'docker':'linux/arm64','go':'1.26.10','cpus':2,'benchmark_cpu':1,'memory':'4GiB','gomemlimit':'1400MiB','gogc':50,'samples':3,'benchtime':'200ms'}, 'results':[]}
for name in baseline:
    old = {key:statistics.median(s[key] for s in baseline[name]) for key in baseline[name][0]}
    new = {key:statistics.median(s[key] for s in candidate[name]) for key in candidate[name][0]}
    report['results'].append({'name':name,'baseline_samples':baseline[name],'candidate_samples':candidate[name], 'baseline_median':old,'candidate_median':new})
(OUT / 'PERFORMANCE.json').write_text(json.dumps(report, indent=2) + '\n')
for row in report['results']:
    print(row['name'], row['baseline_median'], '=>', row['candidate_median'], flush=True)
