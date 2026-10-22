from pathlib import Path
import subprocess,json,re,statistics,shutil,sys
R=Path(__file__).resolve().parent
stages=[('BASELINE','BASELINE'),('MODIFIED','MODIFIED_FILE')]
if len(sys.argv)>1:stages=[('ROLLBACK',sys.argv[1])]
result=json.loads((R/'PERFORMANCE.json').read_text()) if (R/'PERFORMANCE.json').exists() else {}
for stage,name in stages:
 d=Path(name) if name.startswith('/') else R/name
 shutil.copy2(R/'performance_bench_test.go',d/'backend/internal/application/service/zz_upstream_sync_20261022_bench_test.go')
 cmd=['docker','run','--rm','--cpus','2','--memory','3g','-e','GOMAXPROCS=2','-e','GOMEMLIMIT=2300MiB','-e','CGO_ENABLED=0','-e','GOFLAGS=-p=1','-v',str(d/'backend')+':/app','-v','sub2api-upstream-go-mod-20261016:/go/pkg/mod','-v','sub2api-upstream-go-build-20261016:/root/.cache/go-build','-w','/app','sub2api-sync-go:20261022','go','test','-tags','unit','-run','^$','-bench','^BenchmarkUpstreamSync20261022$','-benchmem','-benchtime=300ms','-count=3','./internal/application/service']
 label='performance-v3-'+stage.lower();subprocess.run([sys.executable,str(R/'record.py'),label,str(d),*cmd],check=True)
 record=json.loads((R/'records'/(label+'.record.json')).read_text());groups={}
 for line in record['stdout'].splitlines():
  m=re.match(r'BenchmarkUpstreamSync20261022/(\w+)-2\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',line)
  if m:groups.setdefault(m[1],[]).append([float(m[2]),int(m[3]),int(m[4])])
 assert len(groups)==8 and all(len(v)==3 for v in groups.values()),groups
 result[stage]={n:{'samples':v,'median_ns':statistics.median(x[0] for x in v),'median_bytes':statistics.median(x[1] for x in v),'median_allocs':statistics.median(x[2] for x in v)} for n,v in groups.items()}
 (R/'PERFORMANCE.json').write_text(json.dumps(result,indent=2)+'\n')
if 'MODIFIED' in result:
 result['change_percent']={n:round(100*(result['MODIFIED'][n]['median_ns']/v['median_ns']-1),2) for n,v in result['BASELINE'].items()}
 result['scope']='Same Go 1.26.10 linux/arm64 Docker image, CGO=0, CPU limit 2, GOMAXPROCS=2, memory 3GiB, 300ms x3; sequential after other jobs finish. Mapping, schema and 32-delta stream fixtures; Timeout cases include 180s read timeout and a cancellable request context. Not production gateway throughput.'
 (R/'PERFORMANCE.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result['change_percent']))
