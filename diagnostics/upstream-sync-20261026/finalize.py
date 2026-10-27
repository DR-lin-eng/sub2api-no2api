#!/usr/bin/env python3
from pathlib import Path
import hashlib,json,os,subprocess,tarfile
D=Path(__file__).resolve().parent;R=D.parents[1]
state=json.loads((D/'STATE.json').read_text())
required=['backend-final-unit','integration','frontend-required','backend-final-lint','race','build-accepted-source','runtime-final-health-and-protocol','runtime-key-limits','docs-final','performance-final-baseline','performance-final-modified','performance-final-rollback','transaction-modified','rollback-source','reapply-probe']
records=[]
for p in sorted((D/'records').glob('*.record.json')):
 record=json.loads(p.read_text());record['label']=p.name.removesuffix('.record.json');records.append(record)
by={v['label']:v for v in records}
for label in required:assert by[label]['exit_status']==0,label
for label in ['transaction-baseline','transaction-rollback']:assert by[label]['exit_status']==1,label
transaction=json.loads((D/'SOURCE_TRANSACTION.json').read_text());assert transaction['baseline_hashes_restored']=='PASS' and transaction['reapplied_source_equals_modified_archive']=='PASS'
original=state['target'];assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=original,text=True).strip()==state['original_head'];assert subprocess.check_output(['git','status','--porcelain'],cwd=original)==b''
manifest=json.loads((D/'SOURCE_HASHES.json').read_text())
for p,sha in manifest['modified_hashes'].items():assert hashlib.sha256((R/p).read_bytes()).hexdigest()==sha,p
with tarfile.open(D/'MODIFIED_FILE.tar.gz') as tf:
 for member in tf.getmembers():assert tf.extractfile(member).read()==(R/member.name).read_bytes(),member.name
assert os.access(D/'ROLLBACK.sh',os.X_OK)
lines=['Sub2API upstream synchronization — 2026-10-26','TARGET: '+original,'Original workspace unchanged: HEAD '+state['original_head']+'; clean at final verification.','BASELINE: '+state['baseline']+' tree '+state['baseline_tree'],'FROZEN UPSTREAM: 5c185c5c5d4f5e409e2139806e0964ae944e03e8..'+state['upstream'],'Changed fields: Group.ImageCountBilling / image_count_billing; OpenAIForwardResult.ImageEndpoint; PanelRateLimiter.Authenticated per-Key-ID buckets.','Acceptance: native source reconstruction, pristine rollback hashes, canonical same-command probes, Docker unit/integration/race/lint/image/runtime/frontend and docs.','Baseline/candidate/rollback: dedicated enabled images $0.004500 token / $0.600000 image / $0.004500 token. Mixed and disabled stay $0.004500 token.','Baseline/candidate/rollback: Key A, Key B, Key A HTTP [200,200,200] / [200,200,429] / [200,200,200]; JWT exemption and machine lifecycle 403 retained.','Canonical probe exits: BASELINE=1, MODIFIED=0, ROLLBACK=1. Failures are intentional detection of missing new behavior, not build failures.','HTTP four phases: 32 protocol cases, migration counts 306/307/307/307, one settlement row per successful request, 1000/100 tokens, identities/keys/subscriptions/balance equation preserved.','Source layout: both baseline and candidate exit 2 for the same two unchanged 1201-line files. PREEXISTING_LAYOUT.json records exact hashes; no allowlist changes.','#8319 resource-level authorization remains an explicit specialized follow-up (fully_closed=false). Closed existing lifecycle/rate-limit parts are recorded separately.','Initial tooling/type/contract/lint failures are retained below alongside corrected passing commands.','Publication: exact PR/main CI and registry observations are supplemental records; local success alone does not prove remote publication.','', 'ROLES:']
for role,path in state['roles'].items():lines.append(role+': '+path)
lines+=['','CANONICAL OBSERVED TRANSACTION:',json.dumps(transaction,ensure_ascii=False,indent=2),'','LITERAL EXECUTION LEDGER:']
for record in records:
 lines+=['\n===== '+record['label']+' =====','COMMAND (argv JSON): '+json.dumps(record['command'],ensure_ascii=False),'CWD: '+record['cwd'],'INPUT: '+record['input'],'EXIT STATUS: '+str(record['exit_status']),'DURATION SECONDS: '+str(record['duration_seconds']),'STDOUT (literal):',record['stdout'],'STDERR (literal):',record['stderr']]
(D/'VERIFICATION.txt').write_text('\n'.join(lines)+'\n')
roles={}
for role,path in state['roles'].items():
 p=Path(path);raw=p.read_bytes();roles[role]={'absolute_path':str(p),'bytes':len(raw),'sha256':hashlib.sha256(raw).hexdigest()}
(D/'ROLE_HASHES.json').write_text(json.dumps(roles,indent=2)+'\n')
state['last_confirmed_result']='Final source/native rollback, Docker local gates, 32-case runtime upgrade/rollback, machine-key HTTP matrix and fixed performance accepted.';state['next_executable_action']='Commit, reviewed-ancestry tracking merge, push, create/attach PR, exact PR CI, merge, exact main CI and registry verification.'
(D/'STATE.json').write_text(json.dumps(state,indent=2)+'\n')
(D/'STATUS.json').write_text(json.dumps({'active_object':'Sub2API selective upstream sync through '+state['upstream'],'last_confirmed_result':state['last_confirmed_result'],'next_executable_action':state['next_executable_action'],'input_paths':['STATE.json','FROZEN_RANGE.json','UPSTREAM_PRS.json','SOURCE_HASHES.json','VERIFICATION.txt'],'acceptance_event':'Exact pushed/merged revision and successful CI plus image publication; all four native role paths reopened.'},indent=2)+'\n')
print(json.dumps({'accepted_checks':len(required),'source_paths':len(manifest['paths']),'original_workspace':'unchanged','roles':roles},indent=2))
