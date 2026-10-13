#!/usr/bin/env python3
from pathlib import Path
import hashlib,json,subprocess,tarfile
E=Path(__file__).resolve().parent;ROOT=E.parent;T=ROOT/'rollback'
manifest=json.loads((E/'SOURCE_HASHES.json').read_text())
probe=T/'backend/internal/application/service/upstream_20261013_observed_test.go'
if probe.exists():probe.unlink()
for path,want in json.loads((E/'BASELINE_HASHES.json').read_text()).items():
 assert hashlib.sha256((T/path).read_bytes()).hexdigest()==want,path
subprocess.run(['git','apply','--check',str(E/'DIFF_FILE.patch')],cwd=T,check=True)
subprocess.run(['git','apply',str(E/'DIFF_FILE.patch')],cwd=T,check=True)
for path,want in manifest['modified'].items():assert hashlib.sha256((T/path).read_bytes()).hexdigest()==want,path
with tarfile.open(E/'MODIFIED_FILE.tar.gz','r:gz') as archive:
 for member in archive:
  assert member.isfile()
  assert hashlib.sha256(archive.extractfile(member).read()).hexdigest()==manifest['modified'][member.name]
print('PATCH_AND_NATIVE_ARCHIVE_HASH_PASS',len(manifest['modified']))
subprocess.run([str(E/'ROLLBACK.sh'),str(T)],check=True)
for path,want in json.loads((E/'BASELINE_HASHES.json').read_text()).items():assert hashlib.sha256((T/path).read_bytes()).hexdigest()==want,path
for path in manifest['added']:assert not (T/path).exists(),path
print('ROLLBACK_ALL_ORIGINAL_HASH_PASS',6521)
probe.write_bytes((E/'behavior_probe_test.go').read_bytes())
print('ROLLBACK_READY_FOR_MATCHING_COMMAND')
