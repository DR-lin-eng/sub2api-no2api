#!/usr/bin/env python3
import gzip, hashlib, io, json, pathlib, subprocess, tarfile
ROOT = (pathlib.Path(__file__).resolve().parent.parent / "repo") if (pathlib.Path(__file__).resolve().parent.parent / "STATE.json").exists() else pathlib.Path(__file__).resolve().parents[2]
ROLE = pathlib.Path(__file__).resolve().parent
BASELINE = (ROLE/'BASELINE_SHA.txt').read_text().strip() if (ROLE/'BASELINE_SHA.txt').exists() else json.loads((ROLE.parent/'STATE.json').read_text())['baseline_sha']
tracked = [p for p in subprocess.check_output(['git','diff','--name-only',BASELINE,'--'], cwd=ROOT, text=True).splitlines() if not p.startswith('diagnostics/')]
untracked = [p for p in subprocess.check_output(['git','ls-files','--others','--exclude-standard'], cwd=ROOT, text=True).splitlines() if not p.startswith('diagnostics/')]
paths = sorted(set(p for p in tracked + untracked if p and not p.startswith('diagnostics/')))
(ROLE/'BASELINE_SHA.txt').write_text(BASELINE+'\n')
(ROLE/'SOURCE_PATHS.txt').write_text('\n'.join(paths)+'\n')
manifest={'baseline':{},'modified':{},'added':[]}
def sha(data): return hashlib.sha256(data).hexdigest()
base_bytes = io.BytesIO()
mod_bytes = io.BytesIO()
with tarfile.open(fileobj=base_bytes,mode='w') as base, tarfile.open(fileobj=mod_bytes,mode='w') as mod:
    for path in paths:
        current=(ROOT/path).read_bytes()
        manifest['modified'][path]=sha(current)
        info=tarfile.TarInfo(path);info.size=len(current);info.mtime=0;info.mode=0o644
        mod.addfile(info,io.BytesIO(current))
        old=subprocess.run(['git','show',f'{BASELINE}:{path}'],cwd=ROOT,capture_output=True)
        if old.returncode==0:
            manifest['baseline'][path]=sha(old.stdout)
            info=tarfile.TarInfo(path);info.size=len(old.stdout);info.mtime=0;info.mode=0o644
            base.addfile(info,io.BytesIO(old.stdout))
        else:
            manifest['added'].append(path)
(ROLE/'BASELINE_FILES.tar.gz').write_bytes(gzip.compress(base_bytes.getvalue(),mtime=0))
(ROLE/'MODIFIED_FILE.tar.gz').write_bytes(gzip.compress(mod_bytes.getvalue(),mtime=0))
(ROLE/'SOURCE_HASHES.json').write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
patch=subprocess.check_output(['git','diff','--binary',BASELINE,'--',*tracked],cwd=ROOT) if tracked else b''
for path in untracked:
    result=subprocess.run(['git','diff','--no-index','--binary','/dev/null',path],cwd=ROOT,capture_output=True)
    patch += result.stdout
(ROLE/'DIFF_FILE.patch').write_bytes(patch)
rollback='''#!/bin/sh
set -eu
role_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
target=${1:?Usage: ROLLBACK.sh /absolute/path/to/independent/source/copy}
python3 - "$role_dir" "$target" <<'PYTHON'
import hashlib,json,pathlib,sys,tarfile
role=pathlib.Path(sys.argv[1]).resolve();target=pathlib.Path(sys.argv[2]).resolve()
source = role.parents[1] if role.parent.name == 'diagnostics' else role.parent/'repo'
if not target.is_dir() or not (target/'backend/go.mod').is_file() or target == source or target == pathlib.Path('/') or target == pathlib.Path('/Users/lin/.codex/worktrees/7d88/sub2api-no2api'): raise SystemExit('Use an independent source copy')
manifest=json.loads((role/'SOURCE_HASHES.json').read_text())
with tarfile.open(role/'BASELINE_FILES.tar.gz','r:gz') as archive:
    for member in archive.getmembers():
        if not member.isfile(): continue
        data=archive.extractfile(member).read()
        if hashlib.sha256(data).hexdigest()!=manifest['baseline'].get(member.name): raise SystemExit('Corrupt baseline')
        out=(target/member.name).resolve()
        if not out.is_relative_to(target): raise SystemExit('Invalid target path')
        out.parent.mkdir(parents=True,exist_ok=True);out.write_bytes(data)
for path in manifest['added']:
    out=(target/path).resolve()
    if not out.is_relative_to(target): raise SystemExit('Invalid target path')
    if out.exists(): out.unlink()
for path,expected in manifest['baseline'].items():
    if hashlib.sha256((target/path).read_bytes()).hexdigest()!=expected: raise SystemExit('Restored hash mismatch: '+path)
print('ROLLBACK_PASS')
PYTHON
'''
(ROLE/'ROLLBACK.sh').write_text(rollback);(ROLE/'ROLLBACK.sh').chmod(0o755)
for name in ['BASELINE_FILES.tar.gz','MODIFIED_FILE.tar.gz','DIFF_FILE.patch','ROLLBACK.sh']:
 p=ROLE/name;print(name,sha(p.read_bytes()),p.stat().st_size)
print('BASELINE',BASELINE,'PATHS',len(paths),'ADDED',len(manifest['added']))
