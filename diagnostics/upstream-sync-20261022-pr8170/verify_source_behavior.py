from pathlib import Path
import hashlib,json,shutil,subprocess
P=Path(__file__).resolve().parent;R=Path('/private/tmp/sub2api-upstream-sync-20261022-ieZnRf');copy=R/'rollback-target';role=P.parent/'roles';state=json.loads((role/'SOURCE_TRANSACTION.json').read_text())
def probe(label):
 fixtures=[(P/'probe_test.go','zz_pr8170_probe_test.go'),(R/'probe_test.go','zz_upstream_sync_20261022_probe_test.go')]
 for src,name in fixtures:shutil.copy2(src,copy/'backend/internal/application/service'/name)
 try:
  cmd=['docker','run','--rm','--cpus','2','--memory','3g','-e','GOMAXPROCS=2','-e','GOMEMLIMIT=2300MiB','-e','GOFLAGS=-p=1','-v',str(copy/'backend')+':/app','-v','sub2api-upstream-go-mod-20261016:/go/pkg/mod','-v','sub2api-upstream-go-build-20261016:/root/.cache/go-build','-w','/app','sub2api-sync-go:20261022','go','test','-tags','unit','-run','^Test(PR8170Probe|UpstreamSync20261022Probe)$','-count=1','-v','./internal/application/service']
  subprocess.run(['python3',str(P/'record.py'),label,str(copy),*cmd],check=True)
 finally:
  for src,name in fixtures:(copy/'backend/internal/application/service'/name).unlink(missing_ok=True)
subprocess.run([str(role/'ROLLBACK.sh'),str(copy)],check=True)
assert all(hashlib.sha256((copy/p).read_bytes()).hexdigest()==h for p,h in state['baseline_hashes'].items())
probe('pr8170-source-rollback')
subprocess.run(['git','apply','--check',str(role/'DIFF_FILE.patch')],cwd=copy,check=True)
subprocess.run(['git','apply',str(role/'DIFF_FILE.patch')],cwd=copy,check=True)
assert all(hashlib.sha256((copy/p).read_bytes()).hexdigest()==h for p,h in state['modified_hashes'].items())
probe('pr8170-source-reapplied')
print('Cumulative source rollback/reapply: original baseline bytes and Fast/Gemini/Anthropic probe results observed.')
