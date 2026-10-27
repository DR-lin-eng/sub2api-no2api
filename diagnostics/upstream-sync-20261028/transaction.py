#!/usr/bin/env python3
from pathlib import Path
import hashlib,json,shutil,subprocess,tarfile
D=Path(__file__).resolve().parent;R=D.parents[1];ROOT=R.parent
h=json.loads((D/'SOURCE_HASHES.json').read_text())
copy=ROOT/'rollback-copy'
if copy.exists():shutil.rmtree(copy)
# Exact source with no Git metadata, independent of the unchanged user tree.
shutil.copytree(ROOT/'baseline',copy)
subprocess.run(['git','apply',str(D/'DIFF_FILE.patch')],cwd=copy,check=True)
probe='backend/internal/application/service/upstream_sync_20261028_probe_test.go'
command=['go','test','-tags=unit','./internal/application/service','-run','^TestUpstreamSync20261028Transaction$','-count=1','-v']
results=[]
for stage,source in [('BASELINE',ROOT/'baseline'),('MODIFIED',R),('ROLLBACK',copy)]:
 if stage=='ROLLBACK':
  subprocess.run([str(D/'ROLLBACK.sh'),str(copy)],check=True)
  for p,sha in h['baseline_hashes'].items():
   assert (not(copy/p).exists()) if sha is None else hashlib.sha256((copy/p).read_bytes()).hexdigest()==sha,p
 (source/probe).write_bytes((D/'probes/transaction_test.go').read_bytes())
 args=['docker','run','--rm','--cpus=2','--memory=4g','-v',str(source)+':/src','-v','sub2api-go-mod:/go/pkg/mod','-v','sub2api-go-build:/root/.cache/go-build','-w','/src/backend','sub2api-sync-tests:20261026',*command]
 subprocess.run(['python3',str(D/'record.py'),'transaction-'+stage.lower(),*args],cwd=R,check=True)
 record=json.loads((D/'records'/('transaction-'+stage.lower()+'.record.json')).read_text())
 results.append({'stage':stage,'exit_status':record['exit_status'],'observed_behavior':[line.strip() for line in record['stdout'].splitlines() if 'OBS ' in line],'same_command_input':command})
(copy/probe).unlink()
subprocess.run(['git','apply',str(D/'DIFF_FILE.patch')],cwd=copy,check=True)
for p,sha in h['modified_hashes'].items():assert hashlib.sha256((copy/p).read_bytes()).hexdigest()==sha,p
with tarfile.open(D/'MODIFIED_FILE.tar.gz') as tf:
 for member in tf.getmembers():assert tf.extractfile(member).read()==(copy/member.name).read_bytes(),member.name
result={'baseline_tree':h['baseline_tree'],'affected_paths':len(h['paths']),'baseline_hashes_restored':'PASS','patch_reconstruction':'PASS','reapplied_source_equals_modified_archive':'PASS','results':results}
(D/'SOURCE_TRANSACTION.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result,indent=2))
