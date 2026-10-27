#!/usr/bin/env python3
"""Same-database upgrade/rollback with ephemeral credentials kept in memory."""
import hashlib
import http.client
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import sys
import subprocess
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parent
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'compose.yaml'), '-p', 'sub2api-sync-20261027']
BASELINE = 'ghcr.io/dr-lin-eng/sub2api-no2api:sha-a8ced32'
CANDIDATE = 'sub2api:upstream-sync-20261027'
ENV = {**os.environ, 'SYNC_DB_PASSWORD': secrets.token_urlsafe(24), 'SYNC_ADMIN_EMAIL': 'admin@fixture.invalid',
       'SYNC_ADMIN_PASSWORD': secrets.token_urlsafe(24), 'SYNC_JWT_SECRET': secrets.token_urlsafe(48),
       'SYNC_ENCRYPTION_KEY': secrets.token_hex(32), 'SYNC_IMAGE': BASELINE}
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))


def compose(*args):
    result = subprocess.run(COMPOSE + list(args), env=ENV, capture_output=True, text=True)
    if result.returncode:
        raise RuntimeError('compose ' + str(args) + ': ' + result.stderr)
    return result.stdout.strip()


def sql(statement):
    return compose('exec', '-T', 'postgres', 'psql', '-U', 'sub2api', '-d', 'upgrade', '-v', 'ON_ERROR_STOP=1', '-qAtc', statement)


def request(path, body=None, token=None, method=None):
    headers = {'Content-Type': 'application/json'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    req = urllib.request.Request('http://127.0.0.1:18270' + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
    try:
        response = OPENER.open(req, timeout=20)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        raw = response.read().decode()
        try:
            body = json.loads(raw)
        except ValueError:
            body = raw
        return response.code, body


def start(image, installed=False):
    ENV['SYNC_IMAGE'] = image
    email, password = ENV['SYNC_ADMIN_EMAIL'], ENV['SYNC_ADMIN_PASSWORD']
    if installed:
        ENV['SYNC_ADMIN_EMAIL'], ENV['SYNC_ADMIN_PASSWORD'] = 'invalid-install-email', 'short'
    compose('rm', '-sf', 'provider', 'app')
    compose('up', '-d', 'app')
    compose('up', '-d', '--no-deps', 'provider')
    ENV['SYNC_ADMIN_EMAIL'], ENV['SYNC_ADMIN_PASSWORD'] = email, password
    for _ in range(100):
        try:
            if request('/ready')[0] == 200:
                # Provider startup is independent of application readiness.
                time.sleep(.5)
                return
        except OSError:
            pass
        time.sleep(.5)
    raise AssertionError('app failed readiness')


def login():
    status, key = request('/api/v1/auth/credential-key')
    assert status == 200, ('credential key', status)
    envelope = json.loads(subprocess.check_output(['docker', 'run', '--rm', '-i', '-v', str(ROOT) + ':/fixture:ro',
        'node:24-alpine', 'node', '/fixture/credential_envelope_fixture.mjs'],
        input=json.dumps({'server': key['data'], 'email': ENV['SYNC_ADMIN_EMAIL'], 'password': ENV['SYNC_ADMIN_PASSWORD']}).encode()))
    status, result = request('/api/v1/auth/login', {'credential_envelope': envelope})
    assert status == 200, ('encrypted login', status)
    return result['data']['access_token'], result['data']['user']['id']


def snapshot(uid, kid):
    return sql(f"""SELECT json_build_object('user',(SELECT jsonb_build_array(id,email,password_hash) FROM users WHERE id={uid}),
       'key',(SELECT jsonb_build_array(id,name,quota) FROM api_keys WHERE id={kid}),
       'subscriptions',(SELECT jsonb_agg(jsonb_build_array(user_id,group_id,expires_at,status)) FROM user_subscriptions))""")


def create_key(token, group_id, name):
    status, key = request('/api/v1/keys', {'name': name, 'group_id': group_id, 'quota': 12.34}, token)
    assert status == 200, ('create key', status)
    return key['data']['id'], key['data']['key']


def settled(kid):
    for _ in range(50):
        rows=json.loads(sql(f"SELECT COALESCE(json_agg(json_build_object('input',input_tokens,'output',output_tokens,'actual',actual_cost)),'[]'::json) FROM usage_logs WHERE api_key_id={kid}"))
        if rows:return rows
        time.sleep(.15)
    raise AssertionError('usage not persisted')

def admin_key_request(path, key, body=None):
    req=urllib.request.Request('http://127.0.0.1:18270'+path,data=None if body is None else json.dumps(body).encode(),headers={'Content-Type':'application/json','x-api-key':key})
    try:response=OPENER.open(req,timeout=20)
    except urllib.error.HTTPError as error:response=error
    with response:return response.code

def new_admin_key(token,name):
    code,value=request('/api/v1/admin/settings/admin-api-keys',{'name':name,'scopes':['admin.read','admin.write']},token)
    assert code==201,('create scoped key',code)
    return value['data']['key']

def usage_row(kid):
    for _ in range(60):
        rows=json.loads(sql(f"SELECT COALESCE(json_agg(json_build_object('input',input_tokens,'output',output_tokens,'count',image_count,'total',total_cost,'actual',actual_cost,'mode',billing_mode)),'[]'::json) FROM usage_logs WHERE api_key_id={kid}"))
        if rows:return rows
        time.sleep(.1)
    raise AssertionError('usage not persisted')

def provider(path,body=None):
    script='import urllib.request,json; r=urllib.request.Request("http://127.0.0.1:8000"+'+repr(path)+',data='+('None' if body is None else repr(json.dumps(body).encode()))+',headers={"Content-Type":"application/json"});print(urllib.request.urlopen(r).read().decode())'
    return json.loads(compose('exec','-T','provider','python','-c',script))

def usage(kid):
    for _ in range(80):
        rows=json.loads(sql(f"SELECT COALESCE(json_agg(json_build_object('input',input_tokens,'output',output_tokens,'cost',total_cost,'actual',actual_cost,'response_model',upstream_response_model)),'[]'::json) FROM usage_logs WHERE api_key_id={kid}"))
        if rows:return rows
        time.sleep(.1)
    raise AssertionError('missing reliable settlement')
report=[]
try:
    compose('up','-d','postgres','redis')
    start(BASELINE)
    token,uid=login()
    ack=json.dumps({'version':'v2026.06.10','admin_user_id':uid,'accepted_at':'2026-10-27T00:00:00Z'})
    sql(f"INSERT INTO settings(key,value) VALUES ('admin_compliance_acknowledgement:{uid}','{ack}')")
    sql(f'UPDATE users SET balance=1000 WHERE id={uid}')
    fixtures={}
    for platform,model in [('grok','grok-4.5'),('openai','gpt-6.1-terra')]:
        pricing=json.dumps([{'platform':platform,'models':[model,'gpt-6.1-terra-max','grok-4.6'],'billing_mode':'token','input_price':.000003,'output_price':.000015}])
        gid=int(sql(f"INSERT INTO groups(name,platform,status,rate_multiplier,model_pricing,allow_messages_dispatch) VALUES ('{platform}-fixture','{platform}','active',1,'{pricing}',true) RETURNING id"))
        credentials=json.dumps({'api_key':'fixture','base_url':'http://127.0.0.1:8000'})
        extra=json.dumps({'responses_supported':True})
        aid=int(sql(f"INSERT INTO accounts(name,platform,type,credentials,extra,status,schedulable,concurrency,egress_mode) VALUES ('{platform}-fixture','{platform}','apikey','{credentials}','{extra}','active',true,3,'direct') RETURNING id"))
        sql(f'INSERT INTO account_groups(account_id,group_id) VALUES ({aid},{gid})')
        fixtures[platform]=(gid,aid)
    gid,aid=fixtures['grok'];kid,_=create_key(token,gid,'persisted-key')
    sql(f"INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status) VALUES ({uid},{gid},NOW(),NOW()+INTERVAL '1 day','active')")
    pristine=snapshot(uid,kid);migrations=int(sql('SELECT COUNT(*) FROM schema_migrations'))
    for stage,image,fixed in [('BASELINE',BASELINE,False),('MODIFIED',CANDIDATE,True),('ROLLBACK',BASELINE,False),('RESTORED',CANDIDATE,True)]:
        if stage!='BASELINE':start(image,True)
        token,current=login();assert current==uid and snapshot(uid,kid)==pristine
        assert all(request(path)[0]==200 for path in ['/health','/ready','/'])
        assert int(sql('SELECT COUNT(*) FROM schema_migrations'))==migrations
        provider('/fixture/mode',{'mode':'valid'})
        code,catalog=request(f'/api/v1/admin/accounts/{aid}/models/sync-upstream',{},token)
        assert code==200,(stage,'valid sync',code)
        diagnostics=catalog['data'].get('grok_model_diagnostics')
        assert bool(diagnostics)==fixed,(stage,'optional diagnostics',diagnostics)
        if fixed:assert diagnostics['state']=='fresh'
        before=sql(f"SELECT extra->'upstream_model_metadata' FROM accounts WHERE id={aid}")
        provider('/fixture/mode',{'mode':'invalid'})
        bad_code,_=request(f'/api/v1/admin/accounts/{aid}/models/sync-upstream',{},token)
        assert bad_code==(502 if fixed else 200),(stage,'invalid envelope',bad_code)
        after=sql(f"SELECT extra->'upstream_model_metadata' FROM accounts WHERE id={aid}")
        assert (before==after)==fixed,(stage,'snapshot preservation')
        provider('/fixture/mode',{'mode':'valid'})
        cases=[]
        for platform,path,stream in [('grok','/v1/responses',False),('grok','/v1/responses',True),('openai','/v1/messages',False),('openai','/v1/messages',True)]:
            case_gid,_=fixtures[platform];case_id,key=create_key(token,case_gid,stage+'-'+platform+'-'+str(stream))
            if platform=='grok':body={'model':'grok-4.5','input':'fixture','stream':stream}
            else:body={'model':'gpt-6.1-terra-max','max_tokens':1000,'messages':[{'role':'user','content':'fixture'}],'stream':stream}
            code,value=request(path,body,key)
            assert code==200,(stage,platform,stream,code,str(value)[:160])
            rows=usage(case_id);assert len(rows)==1,(stage,platform,'duplicate settlement')
            row=rows[0];assert (row['input'],row['output'])==(1000,100),(stage,row)
            if platform=='grok':assert row['response_model']==('grok-4.6' if fixed else None),(stage,row)
            else:
                sent=provider('/fixture/last')
                if fixed:assert sent['model']=='gpt-6.1-terra' and sent['reasoning']['effort']=='max',(stage,sent['model'],sent.get('reasoning'))
            cases.append({'platform':platform,'path':path,'stream':stream,'http':code,'settlement':row})
        assert sql(f'SELECT ABS(balance+(SELECT COALESCE(SUM(actual_cost),0) FROM usage_logs WHERE user_id={uid})-1000)<0.00000001 FROM users WHERE id={uid}')=='t'
        assert snapshot(uid,kid)==pristine
        item={'stage':stage,'result':'PASS','cases':cases,'migrations':migrations,'invalid_catalog_http':bad_code,'failed_refresh_preserved_success':before==after,'diagnostic_state':diagnostics['state'] if diagnostics else 'absent','persisted_identity_key_subscription_sha256':hashlib.sha256(pristine.encode()).hexdigest(),'health_ready_frontend_encrypted_login':'PASS','balance_settlement_identity':'PASS'}
        report.append(item);print(json.dumps(item),flush=True)
    costs=[[x['settlement']['cost'] for x in stage['cases']] for stage in report]
    assert all(cost==costs[0] for cost in costs),costs
    (ROOT/'result.json').write_text(json.dumps(report,indent=2)+'\n')
finally:
    compose('down','--volumes','--remove-orphans')
