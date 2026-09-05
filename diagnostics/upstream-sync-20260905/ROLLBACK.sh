#!/bin/sh
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TARGET=${1:?usage: ROLLBACK.sh /absolute/path/to/independent/repository}
python3 - "$HERE" "$TARGET" <<'PYTHON'
import hashlib,json,pathlib,subprocess,sys
here,target=map(pathlib.Path,sys.argv[1:])
target=target.resolve()
manifest=json.loads((here/'MANIFEST.json').read_text())
def verify(state):
 for name,values in manifest.items():
  p=target/name
  actual=hashlib.sha256(p.read_bytes()).hexdigest() if p.exists() else None
  if actual!=values[state]:raise SystemExit(f'hash mismatch: {name} ({state}); no overwrite')
verify('modified')
patch=here/'DIFF_FILE'
subprocess.run(['git','-C',str(target),'apply','--check','--reverse',str(patch)],check=True)
subprocess.run(['git','-C',str(target),'apply','--reverse',str(patch)],check=True)
verify('original')
print(f'ROLLBACK restored={len(manifest)} files hash_match=true')
PYTHON
