#!/usr/bin/env python3
from pathlib import Path
import hashlib,json,subprocess
E=Path(__file__).resolve().parent;T=E.parent/'rollback'
p=T/'backend/internal/application/service/upstream_20261015_observed_test.go'
if p.exists():p.unlink()
subprocess.run(['git','apply',str(E/'DIFF_FILE.patch')],cwd=T,check=True)
manifest=json.loads((E/'SOURCE_HASHES.json').read_text())
for path,want in manifest['modified'].items():
 assert hashlib.sha256((T/path).read_bytes()).hexdigest()==want,path
 assert hashlib.sha256((E.parent/'repo'/path).read_bytes()).hexdigest()==want,path
print('GOAL_REAPPLIED_HASH_PASS',len(manifest['modified']))
for name in ['MODIFIED_FILE.tar.gz','DIFF_FILE.patch','VERIFICATION.txt','ROLLBACK.sh']:
 p=E/name;data=p.read_bytes();print(str(p),len(data),hashlib.sha256(data).hexdigest())
