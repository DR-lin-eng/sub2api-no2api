#!/usr/bin/env python3
"""Record literal commands and outputs; never pass live credentials as argv."""
import datetime,json,pathlib,subprocess,sys,time
root=pathlib.Path(__file__).resolve().parent
label,*command=sys.argv[1:]
print('RUN '+label,flush=True); start=time.monotonic()
p=subprocess.run(command,capture_output=True,text=True)
record={'command':command,'cwd':str(pathlib.Path.cwd()),'input':'','stdout':p.stdout,'stderr':p.stderr,'exit_status':p.returncode,'duration_seconds':round(time.monotonic()-start,3),'utc':datetime.datetime.now(datetime.timezone.utc).isoformat()}
for suffix,value in [('stdout',p.stdout),('stderr',p.stderr),('record.json',json.dumps(record,ensure_ascii=False,indent=2)+'\n')]:
 (root/'records'/f'{label}.{suffix}').write_text(value)
print(json.dumps({'label':label,'exit_status':p.returncode,'duration_seconds':record['duration_seconds'],'stdout_tail':p.stdout[-3600:],'stderr_tail':p.stderr[-1000:]},ensure_ascii=False),flush=True)
sys.exit(p.returncode)
