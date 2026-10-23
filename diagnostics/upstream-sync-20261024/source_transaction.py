#!/usr/bin/env python3
"""Reconstruct, rollback, hash-check and reapply the same changed-source artifact."""
from pathlib import Path
import hashlib,io,json,shutil,subprocess,tarfile
EV=Path(__file__).resolve().parent;REPO=EV.parents[1];ROOT=REPO.parent
manifest=json.loads((EV/'SOURCE_HASHES.json').read_text());baseline=manifest['baseline']
copy=ROOT/'ROLLBACK_TARGET'
if copy.exists():
 subprocess.run([str(EV/'ROLLBACK.sh'),str(copy)],check=True)
else:copy.mkdir()
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',baseline],cwd=REPO))) as tf:tf.extractall(copy,filter='data')
def hashes():return {p:hashlib.sha256((copy/p).read_bytes()).hexdigest() if (copy/p).exists() else None for p in manifest['paths']}
original=hashes();assert original==manifest['baseline_hashes']
patch=str(EV/'DIFF_FILE.patch')
subprocess.run(['git','apply','--check',patch],cwd=copy,check=True)
subprocess.run(['git','apply',patch],cwd=copy,check=True)
assert hashes()==manifest['modified_hashes']
# The changed copy archive must reconstruct identical bytes independently.
for p in manifest['paths']:
 if (copy/p).exists():(copy/p).unlink()
with tarfile.open(EV/'MODIFIED_FILE.tar.gz') as tf:tf.extractall(copy,filter='data')
assert hashes()==manifest['modified_hashes']
result=subprocess.run([str(EV/'ROLLBACK.sh'),str(copy)],capture_output=True,text=True)
assert result.returncode==0,result.stderr
assert hashes()==original
for src,dst in [('service_probe_test.go','backend/internal/application/service/upstream_sync_20261024_probe_test.go'),('handler_probe_test.go','backend/internal/transport/http/handler/upstream_sync_20261024_probe_test.go')]:shutil.copy(EV/'probes'/src,copy/dst)
report={'target_copy':str(copy),'paths':len(manifest['paths']),'baseline_hashes':original,'modified_hashes':manifest['modified_hashes'],'rollback_hashes':hashes(),'restored_equals_original':True,'patch_reconstruction':'PASS','archive_reconstruction':'PASS','rollback_stdout':result.stdout,'rollback_stderr':result.stderr,'rollback_exit_status':result.returncode,'original_workspace_head':subprocess.check_output(['git','rev-parse','HEAD'],cwd='/Users/lin/.codex/worktrees/3998/sub2api-no2api',text=True).strip(),'original_workspace_status':subprocess.check_output(['git','status','--porcelain'],cwd='/Users/lin/.codex/worktrees/3998/sub2api-no2api',text=True)}
(EV/'SOURCE_TRANSACTION.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({'baseline_and_restored_sha256':hashlib.sha256(json.dumps(original,sort_keys=True).encode()).hexdigest(),'paths':report['paths'],'patch_reconstruction':'PASS','archive_reconstruction':'PASS','restored_equals_original':True,'rollback_stdout':result.stdout,'original_workspace_status':report['original_workspace_status']},indent=2))
