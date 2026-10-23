#!/usr/bin/env python3
from pathlib import Path
import json,statistics,subprocess,re
EV=Path(__file__).resolve().parent;REPO=EV.parents[1];ROOT=REPO.parent
common=['docker','run','--rm','--cpus','2','--memory','3g','-e','GOMAXPROCS=2','-e','GOMEMLIMIT=2300MiB','-e','GOFLAGS=-p=1','-e','CGO_ENABLED=0']
def run(label,path):
 probe=common+['-v',str(path/'backend')+':/app','-v','sub2api-upstream-go-mod-20261016:/go/pkg/mod','-v','sub2api-upstream-go-build-20261016:/root/.cache/go-build','-w','/app','sub2api-sync-go:20261024','go','test','-tags','unit','-count=1','-run','^TestUpstreamSync20261024Probe$','-v','./internal/application/service','./internal/transport/http/handler']
 subprocess.run(['python3',str(EV/'record.py'),label+'-matching-probe',*probe],cwd=REPO,check=True)
 cmd=common+['-v',str(path/'backend')+':/app','-v','sub2api-upstream-go-mod-20261016:/go/pkg/mod','-v','sub2api-upstream-go-build-20261016:/root/.cache/go-build','-w','/app','sub2api-sync-go:20261024','go','test','-tags','unit','-run','^$','-bench','^BenchmarkUpstreamSync20261024$','-benchmem','-benchtime=300ms','-count=5','./internal/application/service']
 subprocess.run(['python3',str(EV/'record.py'),label,*cmd],cwd=REPO,check=True)
 values={}
 for line in (EV/'records'/f'{label}.stdout').read_text().splitlines():
  m=re.match(r'BenchmarkUpstreamSync20261024/(\S+)-\d+\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',line)
  if m:values.setdefault(m[1],[]).append([float(m[2]),int(m[3]),int(m[4])])
 return {k:{'median_ns':statistics.median(x[0] for x in v),'bytes':statistics.median(x[1] for x in v),'allocs':statistics.median(x[2] for x in v),'samples':v} for k,v in values.items()}
results={'BASELINE':run('performance-baseline',ROOT/'BASELINE'),'MODIFIED':run('performance-modified',ROOT/'ROLLBACK_TARGET')}
subprocess.run([str(EV/'ROLLBACK.sh'),str(ROOT/'ROLLBACK_TARGET')],check=True)
results['ROLLBACK']=run('performance-rollback',ROOT/'ROLLBACK_TARGET')
subprocess.run(['git','apply',str(EV/'DIFF_FILE.patch')],cwd=ROOT/'ROLLBACK_TARGET',check=True)
for name,v in results['MODIFIED'].items():v['change_percent']=(v['median_ns']/results['BASELINE'][name]['median_ns']-1)*100
report={'resource_limits':'2 CPUs / 3 GiB; GOMAXPROCS=2; same fixed fixtures; serial after CPU-heavy checks','scope':'Error construction and unchanged semantic text classification only; does not estimate live provider or gateway throughput. Retry waits are intentional server-directed latency.','states':results}
(EV/'PERFORMANCE.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({k:{n:{s:v for s,v in x.items() if s!='samples'} for n,x in values.items()} for k,values in results.items()},indent=2))
