#!/usr/bin/env python3
import json, os, pathlib, subprocess, uuid
root=pathlib.Path(__file__).resolve().parents[2]
for file in ['deploy/docker-compose.yml','deploy/docker-compose.local.yml','deploy/docker-compose.dev.yml']:
 for index,password in enumerate(['',"sync$'\";$(echo nope)\\! test"]):
  env=dict(os.environ,DATABASE_PASSWORD='sync_test_password',POSTGRES_PASSWORD='sync_test_password',REDIS_PASSWORD=password)
  project='sub2api-sync-password-'+uuid.uuid4().hex[:10]
  compose=['docker','compose','-p',project,'-f',str(root/file)]
  subprocess.run([*compose,'up','-d','redis'],env=env,check=True,capture_output=True)
  try:
   cid=subprocess.check_output([*compose,'ps','-q','redis'],env=env,text=True).strip()
   cmd=json.loads(subprocess.check_output(['docker','inspect','--format','{{json .Config.Cmd}}',cid],text=True))
   assert cmd[0]=='redis-server' and cmd[-2:]==['--requirepass',password],cmd
   for attempt in range(30):
    r=subprocess.run([*compose,'exec','-T','-e','REDISCLI_AUTH='+password,'redis','redis-cli','ping'],env=env,capture_output=True,text=True)
    if r.returncode==0 and r.stdout.strip()=='PONG':break
   else:raise AssertionError('Redis fixture did not respond with PONG')
   print(file+' password_case='+str(index)+' REDIS_PONG')
  finally:subprocess.run([*compose,'down','-v'],env=env,check=True,capture_output=True)
print('REDIS_COMMAND_MATRIX_PASS')
