#!/usr/bin/env python3
from pathlib import Path
import hashlib,json,re,shutil,statistics,subprocess
D=Path(__file__).resolve().parent;R=D.parents[1];ROOT=R.parent
probe='backend/internal/application/service/upstream_sync_20261028_benchmark_test.go'
command=['go','test','-tags=unit','./internal/application/service','-run','^$','-bench','^BenchmarkUpstreamSync20261028$','-benchmem','-benchtime=200ms','-count=5']
summary={};h=json.loads((D/'SOURCE_HASHES.json').read_text())
for stage,copy in [('BASELINE',ROOT/'baseline'),('MODIFIED',R),('ROLLBACK',ROOT/'rollback-copy')]:
 if stage=='ROLLBACK':
  subprocess.run([str(D/'ROLLBACK.sh'),str(copy)],check=True)
  for p,sha in h['baseline_hashes'].items():
   assert (not(copy/p).exists()) if sha is None else hashlib.sha256((copy/p).read_bytes()).hexdigest()==sha,p
 shutil.copyfile(D/'probes/benchmark_test.go',copy/probe)
 args=['docker','run','--rm','--cpus=2','--memory=4g','-e','GOGC=100','-v',str(copy)+':/src','-v','sub2api-go-mod:/go/pkg/mod','-v','sub2api-go-build:/root/.cache/go-build','-w','/src/backend','sub2api-sync-tests:20261026',*command]
 subprocess.run(['python3',str(D/'record.py'),'performance-'+stage.lower(),*args],cwd=R,check=True)
 rows={}
 for line in (D/'records'/('performance-'+stage.lower()+'.stdout')).read_text().splitlines():
  m=re.search(r'BenchmarkUpstreamSync20261028/(\w+)-\d+\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',line)
  if m:rows.setdefault(m[1],[]).append([float(m[2]),int(m[3]),int(m[4])])
 assert len(rows)==5,rows
 summary[stage]={name:{'median_ns':statistics.median(x[0] for x in values),'bytes':statistics.median(x[1] for x in values),'allocations':statistics.median(x[2] for x in values),'samples':values}for name,values in rows.items()}
 (copy/probe).unlink()
subprocess.run(['git','apply',str(D/'DIFF_FILE.patch')],cwd=ROOT/'rollback-copy',check=True)
for p,sha in h['modified_hashes'].items():assert hashlib.sha256((ROOT/'rollback-copy'/p).read_bytes()).hexdigest()==sha,p
summary['environment']={'image':'sub2api-sync-tests:20261026','go':'1.26.10','platform':'linux/arm64','cpus':2,'memory':'4 GiB','order':['BASELINE','MODIFIED','ROLLBACK'],'same_input':'ASCII and large Unicode fingerprints, Gemini text and inline image, unrelated 1 MiB Chat body with model last','scope':'isolated fixed function fixture; not production gateway throughput'}
(D/'PERFORMANCE.json').write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps(summary,indent=2))
