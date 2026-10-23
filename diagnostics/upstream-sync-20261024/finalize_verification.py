#!/usr/bin/env python3
from pathlib import Path
import hashlib,json
EV=Path(__file__).resolve().parent;state=json.loads((EV/'STATE.json').read_text())
lines=['TARGET '+state['target'],'BASELINE '+state['baseline'],'UPSTREAM '+state['upstream'],'PUBLICATION_SUPPLEMENTS '+state['publication_supplements'],'CHANGED_SYMBOL UpstreamFailoverError.SameAccountRetryDelay; sameAccountRetryDelayFor','INPUT fixed offline Responses capacity/empty-stream/replay/identity fixtures; see probes/ and runtime/','']
for path in sorted((EV/'records').glob('*.record.json')):
 r=json.loads(path.read_text());lines+=['RECORD '+path.name,'COMMAND '+repr(r['command']),'CWD '+r['cwd'],'INPUT '+r['input'],'EXIT_STATUS '+str(r['exit_status']),'STDOUT_BEGIN',r['stdout'],'STDOUT_END','STDERR_BEGIN',r['stderr'],'STDERR_END','']
for name in ['SOURCE_HASHES.json','SOURCE_TRANSACTION.json','PERFORMANCE.json','runtime/result.json','FINAL_PUBLICATION.json','GHCR_FINAL.json','SETUP_ERRORS.json','EDIT_ERRORS.json','STATIC_REVIEW.json','BASELINE_IMAGE.json','history/first-parser/PERFORMANCE.json']:
 p=EV/name
 if p.exists():lines+=['SUPPLEMENT '+name,p.read_text(),'']
(EV/'VERIFICATION.txt').write_text('\n'.join(lines)+'\n')
roles={}
for role,p in state['roles'].items():
 raw=Path(p).read_bytes();roles[role]={'path':p,'sha256':hashlib.sha256(raw).hexdigest(),'bytes':len(raw)}
(EV/'ROLE_HASHES.json').write_text(json.dumps(roles,indent=2)+'\n')
print(json.dumps(roles,indent=2))
