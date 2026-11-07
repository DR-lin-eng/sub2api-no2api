#!/usr/bin/env python3
"""Frozen-main comparison plus the exact upstream per-image rewrite strategy."""
from pathlib import Path
import json
import statistics
import subprocess
import tarfile
import tempfile

ROOT=Path(__file__).resolve().parents[2]
OUT=Path(__file__).resolve().parent
BASELINE='1e629a9a75e043ae0a888a9231cf7c88ad8f37e8'

def run(source,role,native=False):
    script='cp -a /src /work && cp /fixture/compat_test.go /work/internal/shared/apicompat/zz_followup_compare_test.go && cd /work && go test -tags=unit ./internal/shared/apicompat -run "^$" -bench "^BenchmarkSyncBufferedReasoning$" -benchmem -benchtime=200ms -count=3 -cpu=1'
    if native:
        script+=' && cp /fixture/native_test.go /work/internal/application/service/zz_followup_compare_test.go && go test -tags=unit ./internal/application/service -run "^$" -bench "^BenchmarkSync(Native|Terra)" -benchmem -benchtime=200ms -count=3 -cpu=1'
    command=['docker','run','--rm','--network','none','--cpus=2','--memory=4g',
      '-v',str(source)+':/src:ro','-v',str(OUT/'performance')+':/fixture:ro',
      '-v','sub2api-followup-gomod:/go/pkg/mod','-v','sub2api-followup-gocache:/root/.cache/go-build',
      '-e','CGO_ENABLED=0','-e','GOMAXPROCS=1','-e','GOMEMLIMIT=1800MiB','-e','GOTOOLCHAIN=local','-e','GOPROXY=off','-e','GOFLAGS=-p=1','golang:1.26.10-alpine','sh','-c',script]
    log=OUT/'records'/(role+'-performance.log')
    with log.open('w') as stream:subprocess.run(command,stdout=stream,stderr=subprocess.STDOUT,check=True)
    samples={}
    for line in log.read_text().splitlines():
        if not line.startswith('BenchmarkSync'):continue
        parts=line.split()
        samples.setdefault(parts[0],[]).append({unit:float(parts[parts.index(unit)-1]) for unit in ['ns/op','B/op','allocs/op']})
    return {name:{'samples':values,'median':{unit:statistics.median(x[unit] for x in values) for unit in values[0]}} for name,values in samples.items()}

with tempfile.TemporaryDirectory(prefix='sub2api-followup-bench-') as temporary:
    temporary=Path(temporary)
    archive=temporary/'baseline.tar'
    with archive.open('wb') as stream:subprocess.run(['git','archive',BASELINE,'backend'],cwd=ROOT,stdout=stream,check=True)
    with tarfile.open(archive) as stream:stream.extractall(temporary,filter='data')
    before=run(temporary/'backend','baseline')
    after=run(ROOT/'backend','candidate',True)
report={'baseline':BASELINE,'upstream':'6b4b37db43443b5e88e424fe3c69602cc3810dfb','environment':{'docker':'linux/arm64','go':'1.26.10','benchmark_cpu':1,'memory':'4GiB','benchtime':'200ms','samples':3},'baseline_results':before,'candidate_results':after}
(OUT/'PERFORMANCE.json').write_text(json.dumps(report,indent=2)+'\n')
for role,data in [('baseline',before),('candidate',after)]:
    for name,row in data.items():print(role,name,row['median'])
