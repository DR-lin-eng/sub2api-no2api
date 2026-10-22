from pathlib import Path
import gzip
import hashlib
import json
import os
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parent
REPO = ROOT / 'repo'
BASE = ROOT / 'baseline'
subprocess.run(['git', 'add', '-N', 'backend/internal/transport/http/handler/admin/setting_handler_channel_monitor_test.go'], cwd=REPO, check=True)
names = subprocess.check_output(['git', 'diff', '--name-only', 'origin/main', '--', '.', ':!diagnostics'], cwd=REPO, text=True).splitlines()
assert names and not any(name.startswith('backend/migrations/') for name in names)
base_names = subprocess.check_output(['git', '-c', 'core.quotePath=false', 'ls-tree', '-r', '--name-only', 'origin/main'], cwd=REPO, text=True).splitlines()
added = [name for name in names if not (BASE / name).exists()]
removed = [name for name in names if not (REPO / name).exists()]
assert not removed


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def archive(path, source, selected):
    with path.open('wb') as raw, gzip.GzipFile(fileobj=raw, mode='wb', filename='', mtime=0) as stream, tarfile.open(fileobj=stream, mode='w') as output:
        for name in sorted(selected):
            info = output.gettarinfo(str(source / name), arcname=name)
            info.uid = info.gid = info.mtime = 0
            info.uname = info.gname = ''
            with (source / name).open('rb') as body:
                output.addfile(info, body)


archive(ROOT / 'MODIFIED_FILE.tar.gz', REPO, names)
archive(ROOT / 'BASELINE_SOURCE_FILES.tar.gz', BASE, [name for name in names if name not in added])
(ROOT / 'DIFF_FILE.patch').write_bytes(subprocess.check_output(['git', 'diff', '--binary', 'origin/main', '--', '.', ':!diagnostics'], cwd=REPO))
(ROOT / 'ADDED_FILES.txt').write_text(''.join(name + '\n' for name in added))
rollback = '''#!/bin/sh
set -eu
if [ "$#" -ne 1 ] || [ ! -d "$1" ]; then
  printf '%s\\n' 'usage: ROLLBACK.sh TARGET_COPY_DIRECTORY' >&2
  exit 64
fi
role_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
target_copy=$(CDPATH= cd -- "$1" && pwd)
while IFS= read -r path; do
  [ -n "$path" ] || continue
  case "$path" in /*|../*|*/../*) exit 65 ;; esac
  rm -f -- "$target_copy/$path"
done < "$role_dir/ADDED_FILES.txt"
tar -xzf "$role_dir/BASELINE_SOURCE_FILES.tar.gz" -C "$target_copy"
printf '%s\\n' 'ROLLBACK restored pristine source bytes'
'''
(ROOT / 'ROLLBACK.sh').write_text(rollback)
(ROOT / 'ROLLBACK.sh').chmod(0o755)
modified_hashes = {name: sha(REPO / name) for name in names}
baseline_hashes = {name: sha(BASE / name) for name in base_names}
original_digest = hashlib.sha256(json.dumps(baseline_hashes, sort_keys=True).encode()).hexdigest()
(ROOT / 'BASELINE_MANIFEST.json').write_text(json.dumps(baseline_hashes, indent=2) + '\n')
copy = ROOT / 'rollback'
copy.mkdir(exist_ok=True)
with tarfile.open(ROOT / 'BASELINE.tar') as source:
    source.extractall(copy, filter='data')
with tarfile.open(ROOT / 'MODIFIED_FILE.tar.gz') as source:
    source.extractall(copy, filter='data')
assert all(sha(copy / name) == value for name, value in modified_hashes.items())
result = subprocess.run([str(ROOT / 'ROLLBACK.sh'), str(copy)], capture_output=True, text=True)
assert result.returncode == 0, result.stderr
restored_hashes = {name: sha(copy / name) for name in base_names}
assert restored_hashes == baseline_hashes
assert all(not (copy / name).exists() for name in added)
with (ROOT / 'VERIFICATION.txt').open('a') as log:
    log.write('\nCOMMAND: ' + str(ROOT / 'ROLLBACK.sh') + ' ' + str(copy) + '\nSTDOUT:\n' + result.stdout + '\nSTDERR:\n' + result.stderr + '\nEXIT_STATUS: ' + str(result.returncode) + '\n')
transaction = {'original_files': len(base_names), 'changed_files': len(names), 'added_files': added,
    'baseline_source_manifest_sha256': original_digest,
    'restored_source_manifest_sha256': hashlib.sha256(json.dumps(restored_hashes, sort_keys=True).encode()).hexdigest(),
    'modified_hashes': modified_hashes, 'modified_archive_sha256': sha(ROOT / 'MODIFIED_FILE.tar.gz'),
    'patch_sha256': sha(ROOT / 'DIFF_FILE.patch'), 'rollback_exit_status': result.returncode,
    'rollback_stdout': result.stdout, 'rollback_stderr': result.stderr, 'restored_equals_original': True}
(ROOT / 'SOURCE_TRANSACTION.json').write_text(json.dumps(transaction, indent=2) + '\n')
print(json.dumps({key: value for key, value in transaction.items() if key != 'modified_hashes'}, indent=2))
for source, destination in [('probe.spec.ts', 'pr153.transaction.spec.ts'), ('performance.spec.ts', 'pr153.performance.spec.ts')]:
    (copy / 'frontend/src/features/channels-user/__tests__' / destination).write_bytes((ROOT / source).read_bytes())
