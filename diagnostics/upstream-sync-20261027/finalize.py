#!/usr/bin/env python3
from pathlib import Path
import hashlib,json,subprocess,tarfile
D=Path(__file__).resolve().parent
s=json.loads((D/'STATE.json').read_text());h=json.loads((D/'SOURCE_HASHES.json').read_text())
parts=['ACTION=VERIFIED_TRANSACTION','TARGET='+s['target'],'ISOLATED_COPY='+s['repo'],'BASELINE='+s['baseline'],'BASELINE_TREE='+s['baseline_tree'],'UPSTREAM='+s['upstream'],'CHANGED_SYMBOL='+h['changed_symbol'],'INPUT: frozen upstream diffs and deterministic offline fixtures; production credentials were not used.','SOURCE_HASHES:\n'+json.dumps(h,indent=2)]
for name in ['SETUP_ERRORS.json','ORIGINAL_WORKSPACE.json','BROWSER_VERIFICATION.json','PREEXISTING_LAYOUT.json','SOURCE_TRANSACTION.json','PERFORMANCE.json','runtime/result.json','PUBLICATION.json']:
 p=D/name
 if p.exists():parts.append(name+'\n'+p.read_text())
for p in sorted((D/'records').glob('*.record.json')):
 record=json.loads(p.read_text());parts.append('RECORD='+p.name+'\nCOMMAND='+json.dumps(record['command'],ensure_ascii=False)+'\nCWD='+record['cwd']+'\nSTDIN='+record['input']+'\nEXIT_STATUS='+str(record['exit_status'])+'\nSTDOUT_BEGIN\n'+record['stdout']+'\nSTDOUT_END\nSTDERR_BEGIN\n'+record['stderr']+'\nSTDERR_END')
(D/'VERIFICATION.txt').write_text('\n\n'.join(parts)+'\n')
roles={}
for key,path in s['roles'].items():
 p=Path(path);raw=p.read_bytes();assert raw
 if p.name.endswith('.tar.gz'):
  with tarfile.open(p) as tf:
   for member in tf.getmembers():assert tf.extractfile(member).read()==(D.parents[1]/member.name).read_bytes(),member.name
 if p.name=='ROLLBACK.sh':assert p.stat().st_mode&0o111;subprocess.run(['sh','-n',str(p)],check=True)
 roles[key]={'absolute_path':str(p.resolve()),'sha256':hashlib.sha256(raw).hexdigest(),'bytes':len(raw),'reopened':True}
(D/'ROLE_HASHES.json').write_text(json.dumps(roles,indent=2)+'\n');print(json.dumps(roles,indent=2))
