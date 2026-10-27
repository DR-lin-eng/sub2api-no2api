from pathlib import Path
import shutil,subprocess
D=Path(__file__).resolve().parent;R=D.parents[1]
def run(*a):subprocess.run(list(a),cwd=R,check=True)
run('python3',str(D/'record.py'),'integrated-lint','docker','run','--rm','--cpus=2','--memory=4g','-e','GOGC=50','-e','GOLANGCI_LINT_CACHE=/lint-cache','-v','sub2api-sync-lint-20261026:/lint-cache','-v',str(R)+':/src','-v','sub2api-go-mod:/go/pkg/mod','-v','sub2api-go-build:/root/.cache/go-build','-w','/src/backend','sub2api-sync-tests:20261026','golangci-lint','run','--timeout=20m','./...')
run('python3',str(D/'package_roles.py'))
run('python3',str(D/'record.py'),'source-transaction-final','python3',str(D/'transaction.py'))
run('python3',str(D/'record.py'),'runtime-integrated-final','python3',str(D/'runtime/check.py'))
shutil.copyfile(R/'backend/internal/application/service/upstream_sync_20261027_benchmark_test.go',D/'probes/benchmark_test.go')
run('python3',str(D/'record.py'),'performance-final','python3',str(D/'performance.py'))
probe=R/'backend/internal/application/service/upstream_sync_20261027_probe_test.go'
if probe.exists():probe.unlink()
run('python3',str(D/'finalize.py'))
print('FINAL_LOCAL_GATES_ACCEPTED',flush=True)
