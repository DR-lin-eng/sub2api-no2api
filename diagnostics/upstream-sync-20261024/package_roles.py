#!/usr/bin/env python3
"""Package current-owner source, a native patch, literal evidence and standalone rollback."""
from pathlib import Path
import base64,gzip,hashlib,io,json,shlex,subprocess,tarfile
EV=Path(__file__).resolve().parent;REPO=EV.parents[1]
state=json.loads((EV/'STATE.json').read_text());BASE=state['baseline']
def git(*args):return subprocess.check_output(['git',*args],cwd=REPO)
def sha(data):return hashlib.sha256(data).hexdigest()
paths=set(git('diff','--name-only',BASE,'--','backend','frontend','docs').decode().splitlines())
paths.update(git('ls-files','--others','--exclude-standard','--','backend','frontend','docs').decode().splitlines())
paths=sorted(p for p in paths if 'upstream_sync_20261024_probe_test.go' not in p and '__pycache__' not in p and not p.endswith('.tsbuildinfo'))
subprocess.run(['git','add','--',*paths],cwd=REPO,check=True)
patch=git('diff','--binary',BASE,'--',*paths);(EV/'DIFF_FILE.patch').write_bytes(patch)
old={};current={};new=[]
for p in paths:
 current[p]=(REPO/p).read_bytes() if (REPO/p).exists() else None
 proc=subprocess.run(['git','show',BASE+':'+p],cwd=REPO,capture_output=True)
 old[p]=proc.stdout if proc.returncode==0 else None
 if old[p] is None:new.append(p)
def archive(values):
 out=io.BytesIO()
 with tarfile.open(fileobj=out,mode='w') as tf:
  for p,raw in values.items():
   if raw is None:continue
   info=tarfile.TarInfo(p);info.size=len(raw);info.mode=0o644;info.mtime=0
   tf.addfile(info,io.BytesIO(raw))
 return gzip.compress(out.getvalue(),mtime=0)
candidate=archive(current);baseline=archive(old)
(EV/'MODIFIED_FILE.tar.gz').write_bytes(candidate);(EV/'BASELINE_FILE.tar.gz').write_bytes(baseline)
encoded=base64.encodebytes(baseline).decode()
script='''#!/bin/sh
set -eu
[ "$#" -eq 1 ] || { echo 'usage: ROLLBACK.sh TARGET_COPY' >&2; exit 2; }
target_copy=$(CDPATH= cd "$1" && pwd)
[ -f "$target_copy/backend/go.mod" ] || { echo 'target must be a source copy' >&2; exit 2; }
case "$target_copy" in /|/Users/lin/.codex/worktrees/3998/sub2api-no2api) echo 'refusing original workspace' >&2; exit 2;; esac
rollback_payload=$(mktemp "${TMPDIR:-/tmp}/sub2api-rollback.XXXXXX")
trap 'rm -f "$rollback_payload" "$rollback_payload.b64"' EXIT HUP INT TERM
cat > "$rollback_payload.b64" <<'BASELINE_BYTES'
'''+encoded+'''BASELINE_BYTES
if base64 -d < "$rollback_payload.b64" > "$rollback_payload" 2>/dev/null; then :
else base64 -D < "$rollback_payload.b64" > "$rollback_payload"; fi
tar -xzf "$rollback_payload" -C "$target_copy"
'''
for p in new:script+='rm -f "$target_copy"/'+shlex.quote(p)+'\n'
script+='printf \'ROLLBACK_RESTORED=%s\\n\' "$target_copy"\n'
(EV/'ROLLBACK.sh').write_text(script);(EV/'ROLLBACK.sh').chmod(0o755)
manifest={'baseline':BASE,'baseline_tree':state['baseline_tree'],'changed_symbol':'UpstreamFailoverError.SameAccountRetryDelay; sameAccountRetryDelayFor','paths':paths,'baseline_hashes':{p:None if v is None else sha(v) for p,v in old.items()},'modified_hashes':{p:None if v is None else sha(v) for p,v in current.items()},'diff_sha256':sha(patch),'candidate_sha256':sha(candidate),'rollback_embedded_baseline_sha256':sha(baseline)}
(EV/'SOURCE_HASHES.json').write_text(json.dumps(manifest,indent=2)+'\n')
# Reopen every native role before reporting it.
with tarfile.open(EV/'MODIFIED_FILE.tar.gz') as tf:assert sorted(tf.getnames())==sorted(p for p,v in current.items() if v is not None)
assert (EV/'DIFF_FILE.patch').read_bytes()==patch
assert (EV/'ROLLBACK.sh').read_text()==script
print(json.dumps({'source_paths':len(paths),'diff_bytes':len(patch),'candidate_bytes':len(candidate),'roles':state['roles']},indent=2))
