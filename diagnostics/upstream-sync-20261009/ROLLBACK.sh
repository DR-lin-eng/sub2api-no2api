#!/bin/sh
set -eu
role_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
target=${1:?Usage: ROLLBACK.sh /absolute/path/to/independent/source/copy}
python3 - "$role_dir" "$target" <<'PYTHON'
import hashlib,json,pathlib,sys,tarfile
role=pathlib.Path(sys.argv[1]).resolve();target=pathlib.Path(sys.argv[2]).resolve()
if not target.is_dir() or target in [role.parents[1],pathlib.Path('/Users/lin/.codex/worktrees/b2e2/sub2api-no2api')]:
 raise SystemExit('Use an existing independent source copy')
manifest=json.loads((role/'SOURCE_HASHES.json').read_text())
with tarfile.open(role/'BASELINE_FILE','r:gz') as archive:
 for member in archive.getmembers():
  if member.name not in manifest['baseline'] or not member.isfile():raise SystemExit('Invalid baseline member')
  data=archive.extractfile(member).read()
  if hashlib.sha256(data).hexdigest()!=manifest['baseline'][member.name]:raise SystemExit('Corrupt baseline')
  output=target/member.name
  if not output.resolve().is_relative_to(target):raise SystemExit('Invalid target path')
  output.parent.mkdir(parents=True,exist_ok=True);output.write_bytes(data)
for path in manifest['added']:
 output=target/path
 if not output.resolve().is_relative_to(target):raise SystemExit('Invalid target path')
 output.unlink(missing_ok=True)
for path,sha in manifest['baseline'].items():
 if hashlib.sha256((target/path).read_bytes()).hexdigest()!=sha:raise SystemExit('Restored hash mismatch')
print('ROLLBACK_PASS baseline='+'03a9207f932378207bf07ad09ce57aa154b5e0cd')
PYTHON
