#!/usr/bin/env python3
import json, pathlib, sys, urllib.request, http.cookiejar, subprocess
OPENER=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
BASE='http://127.0.0.1:18085'
STATE=pathlib.Path('/private/tmp/sub2api-sync-runtime-state-20260930.json')
stage=sys.argv[1]
def request(path, body=None, token=None):
 headers={'Content-Type':'application/json'}
 if token: headers['Authorization']='Bearer '+token
 req=urllib.request.Request(BASE+path, data=None if body is None else json.dumps(body).encode(), headers=headers)
 with OPENER.open(req,timeout=20) as res:
  content=res.read(); status=res.status
  if path=='/': return status,content.decode()
  return status,json.loads(content)
for path in ['/health','/ready']:
 status,_=request(path);assert status==200;print(stage.upper()+': '+path+' HTTP_200')
status,html=request('/');assert status==200 and 'app' in html;print(stage.upper()+': embedded_frontend HTTP_200')
_,server_key=request('/api/v1/auth/credential-key')
script=pathlib.Path(__file__).with_name('credential_envelope.mjs').resolve()
envelope=json.loads(subprocess.check_output(['docker','run','--rm','-i','-v',str(script)+':/fixture.mjs:ro','node:24-alpine','node','/fixture.mjs'],input=json.dumps(server_key['data']).encode()))
_,login=request('/api/v1/auth/login',{'credential_envelope':envelope})
d=login['data']; token=d['access_token']; uid=d['user']['id'];print(stage.upper()+': admin_login PASS')
if stage=='baseline':
 _,created=request('/api/v1/keys',{'name':'upstream-sync-upgrade-fixture','quota':12.34},token)
 key=created['data']; state={'user_id':uid,'key_id':key['id'],'name':key['name'],'quota':key['quota']}
 STATE.write_text(json.dumps(state));STATE.chmod(0o600);print('BASELINE: api_key_fixture_created PASS')
state=json.loads(STATE.read_text());assert state['user_id']==uid
_,detail=request('/api/v1/keys/'+str(state['key_id']),token=token);key=detail['data']
assert key['id']==state['key_id'] and key['name']==state['name'] and key['quota']==state['quota']
print(stage.upper()+': persisted_key_name_quota PASS')
_,settings=request('/api/v1/settings/public');assert isinstance(settings['data'],dict)
print(stage.upper()+': public_settings_contract PASS')
print(stage.upper()+': RUNTIME_PASS')
