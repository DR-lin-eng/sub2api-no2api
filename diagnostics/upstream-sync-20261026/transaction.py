#!/usr/bin/env python3
from pathlib import Path
import hashlib,json,shutil,subprocess,tarfile
D=Path(__file__).resolve().parent;R=D.parents[1];ROOT=R.parent
h=json.loads((D/'SOURCE_HASHES.json').read_text())
probes=['backend/internal/application/service/image_count_billing_test.go','backend/internal/transport/http/server/middleware/admin_api_key_rate_limit_test.go']
command=['go','test','-tags=unit','./internal/application/service','./internal/transport/http/server/middleware','-run','TestImageCountBillingTransaction|TestAdminAPIKeyRateLimitTransaction','-count=1','-v']
results=[]
for stage,copy,want in [('BASELINE',ROOT/'baseline',1),('MODIFIED',R,0),('ROLLBACK',ROOT/'rollback-copy',1)]:
 if stage=='ROLLBACK':
  subprocess.run([str(D/'ROLLBACK.sh'),str(copy)],check=True)
  for p,sha in h['baseline_hashes'].items():
   if sha is None:assert not (copy/p).exists(),p
   else:assert hashlib.sha256((copy/p).read_bytes()).hexdigest()==sha,p
 for p in probes:
  if stage!='MODIFIED':shutil.copyfile(D/'probes'/Path(p).name,copy/p)
 args=['docker','run','--rm','--cpus=2','--memory=4g','-v',str(copy)+':/src','-v','sub2api-go-mod:/go/pkg/mod','-v','sub2api-go-build:/root/.cache/go-build','-w','/src/backend','sub2api-sync-go:20261024',*command]
 label='transaction-'+stage.lower()
 result=subprocess.run(['python3',str(D/'record.py'),label,*args],cwd=R)
 assert result.returncode==want,(stage,result.returncode,want)
 record=json.loads((D/'records'/(label+'.record.json')).read_text())
 lines=[line.strip() for line in record['stdout'].splitlines() if 'route=' in line or 'key=' in line]
 results.append({'stage':stage,'exit_status':result.returncode,'observed_behavior':lines,'same_command_input':command})
for p in probes:(ROOT/'rollback-copy'/p).unlink()
subprocess.run(['git','apply',str(D/'DIFF_FILE.patch')],cwd=ROOT/'rollback-copy',check=True)
for p,sha in h['modified_hashes'].items():assert hashlib.sha256((ROOT/'rollback-copy'/p).read_bytes()).hexdigest()==sha,p
with tarfile.open(D/'MODIFIED_FILE.tar.gz') as tf:
 for member in tf.getmembers():assert tf.extractfile(member).read()==(ROOT/'rollback-copy'/member.name).read_bytes(),member.name
result={'baseline_tree':h['baseline_tree'],'affected_paths':len(h['paths']),'baseline_hashes_restored':'PASS','patch_reconstruction':'PASS','reapplied_source_equals_modified_archive':'PASS','results':results}
(D/'SOURCE_TRANSACTION.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result,indent=2))
