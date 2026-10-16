#!/usr/bin/env python3
"""Create native source roles without changing the original linked checkout."""
import gzip
import hashlib
import io
import json
from pathlib import Path
import shlex
import subprocess
import tarfile

REPO=Path(__file__).resolve().parents[2]
ROOT=Path(__file__).resolve().parent
BASE='eb130c485520fa27536e33064f39894972205362'
ORIGINAL=Path('/Users/lin/.codex/worktrees/2ca9/sub2api-no2api')

def run(*args):
    return subprocess.check_output(args,cwd=REPO,text=True)

def digest(data): return hashlib.sha256(data).hexdigest()

paths=set(run('git','diff','--name-only',BASE).splitlines()+run('git','ls-files','--others','--exclude-standard').splitlines())
paths=sorted(p for p in paths if not p.startswith('diagnostics/') and (REPO/p).is_file())
assert paths
subprocess.run(['git','add','-N','--',*paths],cwd=REPO,check=True)
original={}
modified={p:(REPO/p).read_bytes() for p in paths}
for p in paths:
    result=subprocess.run(['git','show',BASE+':'+p],cwd=REPO,capture_output=True)
    if result.returncode==0: original[p]=result.stdout

# Deterministic gzip/tar with relative source paths, no git metadata or caches.
def archive(name,files):
    raw=io.BytesIO()
    with tarfile.open(fileobj=raw,mode='w') as tar:
        for p,data in files.items():
            info=tarfile.TarInfo(p)
            info.size=len(data)
            info.mode=0o755 if (REPO/p).stat().st_mode&0o111 else 0o644
            info.mtime=0
            tar.addfile(info,io.BytesIO(data))
    (ROOT/name).write_bytes(gzip.compress(raw.getvalue(),mtime=0))

archive('BASELINE_SOURCE_FILES.tar.gz',original)
archive('MODIFIED_FILE.tar.gz',modified)
patch=subprocess.check_output(['git','diff','--binary',BASE,'--',*paths],cwd=REPO)
assert patch
(ROOT/'DIFF_FILE.patch').write_bytes(patch)
added=sorted(set(modified)-set(original))
rollback='''#!/bin/sh
# Restore this transaction only; retain audit records and unrelated copy files.
set -eu
if [ "$#" -ne 1 ] || [ ! -d "$1" ]; then
  echo "usage: ROLLBACK.sh TARGET_COPY_DIRECTORY" >&2
  exit 64
fi
role_dir=$(CDPATH= cd -P "$(dirname "$0")" && pwd)
target=$(CDPATH= cd -P "$1" && pwd)
case "$target" in
  /|/Users/lin/.codex/worktrees/2ca9/sub2api-no2api|/private/tmp/sub2api-upstream-sync-20261016)
    echo "refusing original workspace or candidate checkout" >&2; exit 64 ;;
esac
baseline="$role_dir/BASELINE_SOURCE_FILES.tar.gz"
[ -f "$baseline" ] || { echo "missing sibling baseline archive" >&2; exit 66; }
while IFS= read -r name; do
  [ -n "$name" ] && rm -f "$target/$name"
done <<'ADDED_SOURCE_PATHS'
'''+ '\n'.join(added)+'''
ADDED_SOURCE_PATHS
tar -xzf "$baseline" -C "$target"
echo "Restored pristine source bytes in $target"
'''
(ROOT/'ROLLBACK.sh').write_text(rollback)
(ROOT/'ROLLBACK.sh').chmod(0o755)
transaction={'target':str(ORIGINAL),'work_copy':str(REPO),'baseline_sha':BASE,'source_paths':paths,'added_source_paths':added,
 'baseline_hashes':{p:digest(data) for p,data in original.items()},
 'modified_hashes':{p:digest(data) for p,data in modified.items()},'diff_sha256':digest(patch)}
(ROOT/'SOURCE_TRANSACTION.json').write_text(json.dumps(transaction,indent=2)+'\n')
ledger=['TARGET='+str(ORIGINAL),'WORK_COPY='+str(REPO),'BASELINE='+BASE,
 'UPSTREAM=b0a02a3304a0a78cdd003ddda1dd23ba02c03084',
 'Changed fields: response.created_at; accounts.extra.web_search_emulation; actual upstream GPT-5.5 eligibility; generationConfig.thinkingConfig and streamed thinking/reasoning_content.',
 'Original linked checkout is preserved. Source roles contain only this batch, based on the freshly fetched project main.',
 'Inputs: immutable local HTTP/SQL/search configuration fixtures; all provider credentials are test fixtures. Runtime random credentials are excluded from logs.',
 json.dumps(transaction,indent=2),'']
for p in sorted((ROOT/'records').glob('*.record.json')):
    record=json.loads(p.read_text())
    ledger.extend(['RECORD '+p.name,'COMMAND '+shlex.join(record.get('command',[])),
       'CWD '+record.get('cwd',''),'EXIT '+str(record['exit_status'])])
    if 'literal_record_gzip' in record:
        ledger.extend(['FULL LITERAL STDOUT/STDERR JSON: records/'+record['literal_record_gzip'],
            'UNCOMPRESSED SHA256 '+record['literal_record_sha256'],
            'PACKAGE OUTCOMES '+json.dumps(record.get('package_outcomes',[]))])
    else:
        ledger.extend(['STDOUT BEGIN',record.get('stdout',''),'STDOUT END','STDERR BEGIN',record.get('stderr',''),'STDERR END'])
    ledger.append('')
(ROOT/'VERIFICATION.txt').write_text('\n'.join(ledger)+'\n')
roles={}
for name in ['MODIFIED_FILE.tar.gz','DIFF_FILE.patch','VERIFICATION.txt','ROLLBACK.sh']:
    p=ROOT/name
    data=p.read_bytes()
    roles[name]={'path':str(p),'bytes':len(data),'sha256':digest(data)}
    if name.endswith('.tar.gz'):
        with tarfile.open(p) as tar:
            assert len(tar.getnames())==len(paths)
    if name.endswith('.sh'):subprocess.run(['/bin/sh','-n',str(p)],check=True)
(ROOT/'ROLES.json').write_text(json.dumps(roles,indent=2)+'\n')
print(json.dumps(roles,indent=2))
