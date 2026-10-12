#!/usr/bin/env python3
import hashlib,io,json,pathlib,subprocess,tarfile
root=pathlib.Path(__file__).resolve().parents[2];role=pathlib.Path(__file__).resolve().parent
baseline=(role/'BASELINE_SHA.txt').read_text().strip()
paths=(role/'SOURCE_PATHS.txt').read_text().splitlines()
manifest={'baseline':{},'modified':{},'added':[]}
def h(b):return hashlib.sha256(b).hexdigest()
archives={name:tarfile.open(role/name,'w:gz') for name in ['BASELINE_FILE','MODIFIED_FILE']}
for path in paths:
 b=(root/path).read_bytes();manifest['modified'][path]=h(b)
 info=tarfile.TarInfo(path);info.size=len(b);info.mtime=0;info.mode=0o644
 archives['MODIFIED_FILE'].addfile(info,io.BytesIO(b))
 r=subprocess.run(['git','show',baseline+':'+path],cwd=root,capture_output=True)
 if r.returncode==0:
  manifest['baseline'][path]=h(r.stdout);info=tarfile.TarInfo(path);info.size=len(r.stdout);info.mtime=0;info.mode=0o644
  archives['BASELINE_FILE'].addfile(info,io.BytesIO(r.stdout))
 else:manifest['added'].append(path)
for archive in archives.values():archive.close()
(role/'SOURCE_HASHES.json').write_text(json.dumps(manifest,indent=2)+'\n')
(role/'DIFF_FILE').write_bytes(subprocess.check_output(['git','diff','--binary','--no-ext-diff',baseline,'--',*paths],cwd=root))
rollback='''#!/bin/sh
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
print('ROLLBACK_PASS baseline='+'''+repr(baseline)+''')
PYTHON
'''
(role/'ROLLBACK.sh').write_text(rollback);(role/'ROLLBACK.sh').chmod(0o755)
for name in ['BASELINE_FILE','MODIFIED_FILE','DIFF_FILE','ROLLBACK.sh']:
 p=role/name;b=p.read_bytes();print(str(p),h(b),len(b))
