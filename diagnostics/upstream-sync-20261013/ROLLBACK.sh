#!/bin/sh
set -eu
role_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
target=${1:?Usage: ROLLBACK.sh /absolute/path/to/independent/source/copy}
python3 - "$role_dir" "$target" <<'PYTHON'
import hashlib,json,pathlib,sys,tarfile
role=pathlib.Path(sys.argv[1]).resolve();target=pathlib.Path(sys.argv[2]).resolve()
source = role.parents[1] if role.parent.name == 'diagnostics' else role.parent/'repo'
if not target.is_dir() or not (target/'backend/go.mod').is_file() or target == source or target == pathlib.Path('/') or target == pathlib.Path('/Users/lin/.codex/worktrees/e3d4/sub2api-no2api'): raise SystemExit('Use an independent source copy')
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
