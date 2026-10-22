from pathlib import Path
import hashlib,json,shutil,subprocess,tarfile
r=Path('/private/tmp/sub2api-upstream-sync-20261022-ieZnRf/MODIFIED_FILE');role=Path('/Users/lin/.codex/visualizations/2026/10/22/01a16146-dab6-7463-bd83-4e067aea805a/roles');d=r/'diagnostics/upstream-sync-20261022-pr8170';state=json.loads((role/'SOURCE_TRANSACTION.json').read_text());copy=r.parent/'rollback-target'
for p in state['source_paths']:
 target=copy/p;target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(r/p,target)
res=subprocess.run([str(role/'ROLLBACK.sh'),str(copy)],capture_output=True,text=True,check=True)
base=state['baseline_sha'];paths=[p for p in subprocess.check_output(['git','ls-tree','-rz','--name-only',base],cwd=r).decode().split('\0') if p and not p.startswith('diagnostics/')]
files={}
for p in paths:
 if (copy/p).is_symlink():files[p]=hashlib.sha256((copy/p).readlink().as_posix().encode()).hexdigest()
 elif (copy/p).is_file():files[p]=hashlib.sha256((copy/p).read_bytes()).hexdigest()
 else:raise AssertionError('missing '+p)
pristine={}
with tarfile.open(r.parent/'BASELINE.tar.gz') as archive:
 for member in archive:
  if member.name in files:
   if member.isfile():pristine[member.name]=hashlib.sha256(archive.extractfile(member).read()).hexdigest()
   elif member.issym():pristine[member.name]=hashlib.sha256(member.linkname.encode()).hexdigest()
assert files==pristine,[(p,files[p],pristine.get(p)) for p in files if files[p]!=pristine.get(p)][:5]
assert all(not (copy/p).exists() for p in state['added_source_paths'])
subprocess.run(['git','apply','--check',str(role/'DIFF_FILE.patch')],cwd=copy,check=True)
subprocess.run(['git','apply',str(role/'DIFF_FILE.patch')],cwd=copy,check=True)
assert all(hashlib.sha256((copy/p).read_bytes()).hexdigest()==h for p,h in state['modified_hashes'].items())
aggregate=lambda m:hashlib.sha256(json.dumps(m,sort_keys=True,separators=(',',':')).encode()).hexdigest()
proof={'copy':str(copy),'baseline_sha':base,'all_baseline_source_hashes_match':True,'verified_source_files':len(files),'baseline_source_manifest_sha256':aggregate(pristine),'restored_source_manifest_sha256':aggregate(files),'all_added_source_paths_removed':True,'all_modified_hashes_match_after_reapply':True,'final_diff_sha256':state['diff_sha256'],'rollback_command':[str(role/'ROLLBACK.sh'),str(copy)],'rollback_stdout':res.stdout,'rollback_stderr':res.stderr,'rollback_exit':res.returncode}
(role/'ROLLBACK_HASH_VERIFIED.json').write_text(json.dumps(proof,indent=2)+'\n');(role/'REAPPLY_HASH_VERIFIED.json').write_text(json.dumps({'all_modified_hashes_match':True,'diff_sha256':state['diff_sha256']},indent=2)+'\n')
for name in ['SOURCE_TRANSACTION.json','ROLLBACK_HASH_VERIFIED.json','REAPPLY_HASH_VERIFIED.json']:shutil.copy2(role/name,d/name)
print(json.dumps(proof,indent=2))
