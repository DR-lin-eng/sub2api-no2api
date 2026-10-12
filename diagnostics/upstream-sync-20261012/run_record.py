#!/usr/bin/env python3
import datetime,hashlib,json,pathlib,subprocess,sys,time
name=sys.argv[1]
cmd=sys.argv[2:]
records=pathlib.Path(__file__).resolve().parent/'records'
records.mkdir(exist_ok=True)
begin=time.monotonic()
with (records/(name+'.stdout')).open('wb') as out, (records/(name+'.stderr')).open('wb') as err:
 p=subprocess.run(cmd,stdout=out,stderr=err)
p.stdout=(records/(name+'.stdout')).read_bytes()
p.stderr=(records/(name+'.stderr')).read_bytes()
r={'command':cmd,'cwd':str(pathlib.Path.cwd()),'exit_status':p.returncode,'elapsed_s':time.monotonic()-begin,'utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'input':'exact repository sources and committed fixtures','stdout_sha256':hashlib.sha256(p.stdout).hexdigest(),'stderr_sha256':hashlib.sha256(p.stderr).hexdigest()}
(records/(name+'.record.json')).write_text(json.dumps(r,ensure_ascii=False,indent=2)+'\n')
print(json.dumps(r,ensure_ascii=False))
print(p.stdout.decode(errors='replace')[-3500:])
print(p.stderr.decode(errors='replace')[-3500:])
sys.exit(p.returncode)
