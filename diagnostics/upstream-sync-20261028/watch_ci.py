#!/usr/bin/env python3
"""Observe exact revision workflows, with bounded polls and persisted snapshots."""
import json,subprocess,sys,time
from pathlib import Path
D=Path(__file__).resolve().parent
phase,sha=sys.argv[1:3]
required={'CI','Security Scan','Docker Image'}
last=None
for attempt in range(160):
 raw=subprocess.check_output(['gh','api',f'repos/DR-lin-eng/sub2api-no2api/actions/runs?head_sha={sha}&per_page=100'],text=True)
 data=json.loads(raw);runs=[{k:item[k] for k in ['id','name','event','head_sha','status','conclusion','html_url','run_attempt']}for item in data['workflow_runs']]
 (D/(phase.upper()+'_CI.json')).write_text(json.dumps({'sha':sha,'runs':runs},indent=2)+'\n')
 snapshot=[(x['id'],x['status'],x['conclusion'])for x in runs]
 if snapshot!=last:
  print(json.dumps({'phase':phase,'sha':sha,'workflows':[{k:x[k]for k in ['id','name','event','status','conclusion']}for x in runs]}),flush=True);last=snapshot
 seen={x['name']for x in runs}
 if required.issubset(seen) and all(x['status']=='completed'for x in runs):
  failures=[x for x in runs if x['conclusion']not in ['success','skipped','neutral']]
  if failures:print(json.dumps({'failed':failures}),flush=True);raise SystemExit(1)
  print('EXACT_REVISION_CI_PASSED '+phase+' '+sha,flush=True);raise SystemExit(0)
 time.sleep(30)
raise SystemExit('CI did not finish within the bounded observation window')
