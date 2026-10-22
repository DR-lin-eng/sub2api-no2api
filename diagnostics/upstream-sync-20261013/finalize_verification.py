#!/usr/bin/env python3
from pathlib import Path
import hashlib,json
E=Path(__file__).resolve().parent
state=json.loads((E.parent/'STATE.json').read_text()) if (E.parent/'STATE.json').exists() else {}
head=['VERIFIED TRANSACTION: upstream selective adaptation 2026-10-13',
'TARGET: /Users/lin/.codex/worktrees/e3d4/sub2api-no2api (original preserved)',
'BASELINE_SHA: 8773001a188f0b1fed3dff5d23b1700c70e3cf5e',
'UPSTREAM_RANGE: ab80abc6d1f929a6864c8804fd077b509d21fda2..4d9ac3e482106d2da3a39fe3e546d8d6801f1cc1',
'CHANGED_FIELD: TempUnschedulableRule.Scope / credentials.temp_unschedulable_rules[].scope',
'CHANGED_BEHAVIORS: Antigravity image family isolation; Gemini tool-result base64 images; Kimi international official URL; narrow one-shot converted Responses input fallback and original startTime.',
'INPUTS: exact committed tests and behavior_probe_test.go (literal model, base64, scope and HTTP validation fixtures).',
'BASELINE: provider=""; image=true,text=true; images=0,parts=1; account_writes=0,model_writes=1.',
'MODIFIED: provider="kimi"; image=false,text=true; images=1,parts=2; account_writes=1,model_writes=0.',
'ROLLBACK: provider=empty; image=true,text=true; images=0,parts=1; account_writes=0,model_writes=1; exit=0. All 6521 pristine file hashes restored. GOAL reapplied and modified hashes matched.',
'ROLES:']
for name in ['MODIFIED_FILE.tar.gz','DIFF_FILE.patch','VERIFICATION.txt','ROLLBACK.sh']:
 head.append(str(E/name))
for name in ['behavior_probe_test.go','SOURCE_HASHES.json','BASELINE_HASHES.json','UPSTREAM_PRS.json']:
 p=E/name
 if p.exists():head.append(f'INPUT_FILE: {p} bytes={p.stat().st_size} sha256={hashlib.sha256(p.read_bytes()).hexdigest()}')
for record in sorted((E/'records').glob('*.record.json')):
 d=json.loads(record.read_text());stem=record.name.removesuffix('.record.json')
 head += ['',f'EVENT: {stem}','COMMAND_JSON: '+json.dumps(d['command'],ensure_ascii=False),'CWD: '+d['cwd'],'INPUT: '+d.get('input','exact frozen fixture'),'EXIT_STATUS: '+str(d['exit_status']),'ELAPSED_SECONDS: '+str(d['elapsed_s'])]
 for extension in ['stdout','stderr']:
  p=E/'records'/(stem+'.'+extension);data=p.read_bytes();head += [extension.upper()+'_SHA256: '+hashlib.sha256(data).hexdigest(),extension.upper()+'_BEGIN',data.decode(errors='replace'),extension.upper()+'_END']
head += ['','SUPPLEMENTAL_RECORDS (including corrected command, compile, timeout and patch failures):']
for record in sorted((E/'records').glob('*.json')):
 if record.name.endswith('.record.json'):continue
 head.extend([str(record),record.read_text()])
for name in ['MODIFIED_FILE.tar.gz','DIFF_FILE.patch','ROLLBACK.sh']:
 p=E/name
 if p.exists():head.append(f'ROLE_HASH {p} bytes={p.stat().st_size} sha256={hashlib.sha256(p.read_bytes()).hexdigest()}')
(E/'VERIFICATION.txt').write_text('\n'.join(head)+'\n')
roles={}
for name in ['MODIFIED_FILE.tar.gz','DIFF_FILE.patch','VERIFICATION.txt','ROLLBACK.sh']:
 p=E/name;data=p.read_bytes();roles[name]={'path':str(p),'bytes':len(data),'sha256':hashlib.sha256(data).hexdigest()}
(E/'ROLE_HASHES.json').write_text(json.dumps(roles,indent=2)+'\n')
print(json.dumps(roles,indent=2))
