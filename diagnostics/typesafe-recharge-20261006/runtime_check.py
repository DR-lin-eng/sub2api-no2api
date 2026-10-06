#!/usr/bin/env python3
import json, pathlib, sys, urllib.request, http.cookiejar, subprocess, urllib.error
OPENER=urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
BASE='http://127.0.0.1:18088'
STATE=pathlib.Path('/private/tmp/sub2api-adapt-runtime-state-20261006.json')
stage=sys.argv[1]
def request(path, body=None, token=None):
 headers={'Content-Type':'application/json'}
 if token: headers['Authorization']='Bearer '+token
 req=urllib.request.Request(BASE+path, data=None if body is None else json.dumps(body).encode(), headers=headers)
 try:
  res=OPENER.open(req,timeout=60)
 except urllib.error.HTTPError as e:
  print('HTTP_FAILURE',path,e.code,e.read().decode(),file=sys.stderr);raise
 with res:
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
_,order=request('/api/v1/payment/public/orders/verify', {'out_trade_no':'typesafe-recharge-20261006-legacy'})
assert order['data']['status']=='PENDING' and order['data']['paid'] is False
print(stage.upper()+': legacy_order_state PASS')
if stage in ['modified','restored','native-setup','native']:
 _,settings=request('/api/v1/admin/settings',token=token)
 assert settings['data']['payment_recharge_bonus_enabled'] is False
 assert settings['data']['payment_recharge_bonus_tiers']==[{'min_amount':10,'bonus_percent':20}]
 print(stage.upper()+': dormant_tiers_default_off PASS')
if stage=='native-setup':
 _,chat=request('/api/v1/admin/groups',{'name':'native-ordered-empty-fixture','platform':'typesafe','rate_multiplier':1},token)
 _,native=request('/api/v1/admin/groups',{'name':'native-typesafe-fixture','platform':'typesafe','rate_multiplier':1},token)
 gid=native['data']['id']; chatid=chat['data']['id']
 sql="UPDATE users SET balance=100 WHERE id="+str(uid)+"; INSERT INTO accounts (name,platform,type,credentials,egress_mode) VALUES ('native-fixture','typesafe','apikey','{\"api_key\":\"typesafe-fixture-key\",\"base_url\":\"http://upstream:8080\"}','direct'); INSERT INTO account_groups (account_id,group_id) SELECT id,"+str(gid)+" FROM accounts WHERE name='native-fixture';"
 subprocess.run(['docker','compose','-f',str(pathlib.Path(__file__).with_name('compose.yaml').resolve()),'-p','sub2api-adapt-20261006','exec','-T','postgres','psql','-U','sub2api','-d','sub2api','-v','ON_ERROR_STOP=1'],input=sql.encode(),check=True,stdout=subprocess.DEVNULL)
 state.update({'native_group_id':gid,'chat_group_id':chatid});STATE.write_text(json.dumps(state))
 print('NATIVE_SETUP: group_and_account_fixture PASS')
if stage=='probe':
 _,probe=request('/api/v1/admin/accounts/1/upstream-billing-probe',{},token)
 assert probe['data']['snapshot']['status']=='ok',probe
 print('PROBE: typesafe_relay_manual_billing_probe PASS')
if stage=='native':
 _,created=request('/api/v1/keys',{'name':'typesafe-ordered-fixture','group_bindings':[{'group_id':state['chat_group_id']},{'group_id':state['native_group_id']}]},token)
 nativeKey=created['data'];keyToken=nativeKey['key']
 _,catalog=request('/v1/models',token=keyToken)
 assert any(x['id']=='jev-latest' for x in catalog['data'])
 payload={'model':'jev-latest','state':'synthetic sample','questions':{'q':{'type':'noul','instructions':'Evaluate'}}}
 _,result=request('/v1/systemone',payload,keyToken)
 assert result['usage']['input_tokens']==123 and result['extension']['kept'] is True
 print('NATIVE: ordered_group_dispatch_raw_json PASS')
 import urllib.error,time
 payload['stream']=True
 try: request('/v1/systemone',payload,keyToken);raise AssertionError('stream unexpectedly accepted')
 except urllib.error.HTTPError as e: assert e.code==400
 del payload['stream'];payload['state']='force-failover'
 try: request('/v1/systemone',payload,keyToken);raise AssertionError('outage unexpectedly accepted')
 except urllib.error.HTTPError as e: assert e.code in [502,503]
 payload['state']='post-failure sample'
 _,result=request('/v1/systemone',payload,keyToken)
 assert result['usage']['input_tokens']==123
 print('NATIVE: stream_rejection_and_failure_slot_release PASS')
 sql='SELECT COUNT(*),COALESCE(SUM(input_tokens),0),COALESCE(SUM(actual_cost),0) FROM usage_logs WHERE api_key_id='+str(nativeKey['id'])
 command=['docker','compose','-f',str(pathlib.Path(__file__).with_name('compose.yaml').resolve()),'-p','sub2api-adapt-20261006','exec','-T','postgres','psql','-U','sub2api','-d','sub2api','-Atc',sql]
 for _ in range(20):
  counts=subprocess.check_output(command,text=True).strip().split('|')
  if counts[0]=='2':break
  time.sleep(0.2)
 assert counts[0]=='2' and counts[1]=='246',counts
 assert abs(float(counts[2])-246*0.042/1e6)<1e-9,counts
 print('NATIVE: reliable_usage_246_input_tokens_free_output PASS')
print(stage.upper()+': RUNTIME_PASS')
