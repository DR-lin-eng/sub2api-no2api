#!/usr/bin/env python3
"""Deterministic native source transaction and a literal command ledger."""
import gzip
import hashlib
import io
import json
from pathlib import Path
import shlex
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parent
REPO = ROOT / 'repo' if (ROOT / 'repo/.git').exists() else ROOT.parents[1]
STATE = json.loads((ROOT / 'STATE.json').read_text())
BASE = STATE['baseline_sha']


def git(*args):
    return subprocess.check_output(['git', *args], cwd=REPO)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def archive(path, files):
    raw = io.BytesIO()
    with tarfile.open(fileobj=raw, mode='w') as tar:
        for name, data in sorted(files.items()):
            item = tarfile.TarInfo(name)
            item.size = len(data)
            item.mode = 0o755 if (REPO / name).stat().st_mode & 0o111 else 0o644
            tar.addfile(item, io.BytesIO(data))
    path.write_bytes(gzip.compress(raw.getvalue(), mtime=0))


paths = sorted(set(git('diff', '--name-only', BASE).decode().splitlines() + git('ls-files', '--others', '--exclude-standard').decode().splitlines()))
paths = [name for name in paths if not name.startswith('diagnostics/') and '/zz_upstream_sync_' not in name and (REPO / name).is_file()]
assert paths
subprocess.run(['git', 'add', '-N', '--', *paths], cwd=REPO, check=True)
modified = {name: (REPO / name).read_bytes() for name in paths}
baseline = {}
for name in paths:
    result = subprocess.run(['git', 'show', BASE + ':' + name], cwd=REPO, capture_output=True)
    if result.returncode == 0:
        baseline[name] = result.stdout
added = sorted(set(modified) - set(baseline))
archive(ROOT / 'BASELINE_SOURCE_FILES.tar.gz', baseline)
archive(ROOT / 'MODIFIED_FILE.tar.gz', modified)
patch = git('diff', '--binary', BASE, '--', *paths)
assert patch
(ROOT / 'DIFF_FILE.patch').write_bytes(patch)
rollback = '''#!/bin/sh
set -eu
if [ "$#" -ne 1 ] || [ ! -d "$1" ]; then
  echo "usage: ROLLBACK.sh TARGET_COPY_DIRECTORY" >&2
  exit 64
fi
role_dir=$(CDPATH= cd -P "$(dirname "$0")" && pwd)
target=$(CDPATH= cd -P "$1" && pwd)
case "$target" in
  /|''' + str(REPO) + '|' + STATE['TARGET'] + ''')
    echo "refusing original workspace or candidate checkout" >&2
    exit 64 ;;
esac
[ -f "$role_dir/BASELINE_SOURCE_FILES.tar.gz" ] || { echo "missing sibling baseline archive" >&2; exit 66; }
while IFS= read -r name; do
  [ -z "$name" ] || rm -f "$target/$name"
done <<'ADDED_SOURCE_PATHS'
''' + '\n'.join(added) + '''
ADDED_SOURCE_PATHS
tar -xzf "$role_dir/BASELINE_SOURCE_FILES.tar.gz" -C "$target"
echo "Restored pristine source bytes in $target"
'''
(ROOT / 'ROLLBACK.sh').write_text(rollback)
(ROOT / 'ROLLBACK.sh').chmod(0o755)
transaction = {'target': STATE['TARGET'], 'original_worktree_head': STATE['original_head'], 'work_copy': str(REPO),
    'baseline_sha': BASE, 'upstream_sha': STATE['upstream_sha'], 'source_paths': paths, 'added_source_paths': added,
    'baseline_hashes': {k: sha(v) for k, v in baseline.items()}, 'modified_hashes': {k: sha(v) for k, v in modified.items()},
    'diff_sha256': sha(patch), 'changed_symbols': ['ImageResultUploader.download Content-Type validation', 'extractGeminiUsage.InputTokens',
        'Antigravity InputTokens projections', 'Gemini ForwardResult.ClientDisconnect', 'Gemini interrupted ImageCount and cumulative usage']}
(ROOT / 'SOURCE_TRANSACTION.json').write_text(json.dumps(transaction, ensure_ascii=False, indent=2) + '\n')
ledger = ['TARGET=' + STATE['TARGET'], 'WORK_COPY=' + str(REPO), 'BASELINE=' + BASE, 'UPSTREAM=' + STATE['upstream_sha'],
    'Original linked checkout remains untouched. Baseline is freshly fetched current project main, not the old linked-worktree HEAD.',
    'Inputs: offline Gemini final/cached-only/SSE fixtures, public/non-image image fixtures and one disposable PostgreSQL/Redis database.',
    'Runtime random credentials, cookies and access tokens stay in process memory and are excluded from this ledger.',
    json.dumps(transaction, ensure_ascii=False, indent=2), '']
for path in sorted((ROOT / 'records').glob('*.record.json')):
    record = json.loads(path.read_text())
    ledger.extend(['RECORD ' + path.name, 'COMMAND ' + shlex.join(record['command']), 'CWD ' + record['cwd'],
        'EXIT ' + str(record['exit_status']), 'STDOUT BEGIN', record['stdout'], 'STDOUT END', 'STDERR BEGIN', record['stderr'], 'STDERR END', ''])
for name in ['INITIAL_ERRORS.txt', 'ROLLBACK_HASH_VERIFIED.json', 'REAPPLY_HASH_VERIFIED.json', 'runtime/result.json', 'FINAL_PUBLICATION.json']:
    path = ROOT / name
    if path.exists():
        ledger.extend(['SUPPLEMENT ' + name, path.read_text(), ''])
(ROOT / 'VERIFICATION.txt').write_text('\n'.join(ledger) + '\n')

# Reopen every native role and validate its parseable format.
roles = {}
for name in ['MODIFIED_FILE.tar.gz', 'DIFF_FILE.patch', 'VERIFICATION.txt', 'ROLLBACK.sh']:
    path = ROOT / name
    data = path.read_bytes()
    roles[name] = {'path': str(path), 'sha256': sha(data), 'bytes': len(data)}
    if name.endswith('.tar.gz'):
        with tarfile.open(path) as tar:
            assert sorted(tar.getnames()) == paths
    else:
        data.decode('utf-8')
subprocess.run(['/bin/sh', '-n', str(ROOT / 'ROLLBACK.sh')], check=True)
(ROOT / 'ROLES.json').write_text(json.dumps(roles, indent=2) + '\n')
print(json.dumps(roles, indent=2))
