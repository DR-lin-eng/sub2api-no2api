#!/usr/bin/env python3
from pathlib import Path
import hashlib,json,re,shutil,statistics,subprocess
D=Path(__file__).resolve().parent;R=D.parents[1];ROOT=R.parent
probe='backend/internal/application/service/image_count_billing_benchmark_test.go'
shutil.copyfile(D/'probes'/Path(probe).name,ROOT/'baseline'/probe)
command=['go','test','-tags=unit','./internal/application/service','-run','^$','-bench','^BenchmarkImageCountBillingPricing$','-benchmem','-benchtime=200ms','-count=5']
summary={}
for label,copy in [('baseline',ROOT/'baseline'),('modified',R),('rollback',ROOT/'rollback-copy')]:
 if label=='rollback':
  subprocess.run([str(D/'ROLLBACK.sh'),str(copy)],check=True)
  h=json.loads((D/'SOURCE_HASHES.json').read_text())
  for p,sha in h['baseline_hashes'].items():
   assert (not (copy/p).exists()) if sha is None else hashlib.sha256((copy/p).read_bytes()).hexdigest()==sha,p
  shutil.copyfile(D/'probes'/Path(probe).name,copy/probe)
 args=['docker','run','--rm','--cpus=2','--memory=4g','-v',str(copy)+':/src','-v','sub2api-go-mod:/go/pkg/mod','-v','sub2api-go-build:/root/.cache/go-build','-w','/src/backend','sub2api-sync-go:20261024',*command]
 subprocess.run(['python3',str(D/'record.py'),'performance-final-'+label,*args],cwd=R,check=True)
 rows={}
 for line in (D/'records'/('performance-final-'+label+'.stdout')).read_text().splitlines():
  m=re.search(r'BenchmarkImageCountBillingPricing/(\w+)-\d+\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',line)
  if m:rows.setdefault(m[1],[]).append([float(m[2]),int(m[3]),int(m[4])])
 summary[label]={name:{'median_ns':statistics.median(v[0] for v in values),'bytes':statistics.median(v[1] for v in values),'allocations':statistics.median(v[2] for v in values),'samples':values} for name,values in rows.items()}
# Reapply the accepted goal after restoring pristine source and measuring it.
(ROOT/'rollback-copy'/probe).unlink()
subprocess.run(['git','apply',str(D/'DIFF_FILE.patch')],cwd=ROOT/'rollback-copy',check=True)
h=json.loads((D/'SOURCE_HASHES.json').read_text())
for p,sha in h['modified_hashes'].items():assert hashlib.sha256((ROOT/'rollback-copy'/p).read_bytes()).hexdigest()==sha,p
summary['environment']={'docker_image':'sub2api-sync-go:20261024','go':'1.26.10','arch':'linux/arm64','cpus':2,'memory':'4 GiB','order':['baseline','modified','rollback'],'scope':'fixed cost-resolution fixture, not gateway throughput','initial_exploratory_runs':'benchmark-baseline and benchmark-candidate ran concurrently with other checks; retained but not used for final median comparison'}
(D/'PERFORMANCE.json').write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps(summary,indent=2))
