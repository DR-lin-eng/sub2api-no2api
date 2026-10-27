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
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'compose.yaml'), '-p', 'sub2api-sync-20261026']
BASELINE = 'sub2api:sync-baseline-20261026'
CANDIDATE = 'sub2api:upstream-sync-20261026'
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
    req = urllib.request.Request('http://127.0.0.1:18260' + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
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
    req=urllib.request.Request('http://127.0.0.1:18260'+path,data=None if body is None else json.dumps(body).encode(),headers={'Content-Type':'application/json','x-api-key':key})
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

report=[]
try:
    compose('up','-d','postgres','redis')
    start(BASELINE)
    token,uid=login()
    ack=json.dumps({'version':'v2026.06.10','admin_user_id':uid,'accepted_at':'2026-10-26T00:00:00Z'})
    sql(f"INSERT INTO settings(key,value) VALUES ('admin_compliance_acknowledgement:{uid}','{ack}')")
    sql(f'UPDATE users SET balance=1000 WHERE id={uid}')
    pricing=json.dumps([{'platform':'openai','models':['gpt-image-2','gpt-4.1'],'billing_mode':'token','input_price':.000003,'output_price':.000015}])
    gid=int(sql(f"INSERT INTO groups(name,platform,status,rate_multiplier,allow_image_generation,image_price_1k,model_pricing) VALUES ('image-fixture','openai','active',1,true,0.3,'{pricing}') RETURNING id"))
    aid=int(sql('''INSERT INTO accounts(name,platform,type,credentials,extra,status,schedulable,concurrency,egress_mode) VALUES ('offline-images','openai','apikey','{"api_key":"fixture","base_url":"http://127.0.0.1:8000"}','{}','active',true,3,'direct') RETURNING id'''))
    sql(f'INSERT INTO account_groups(account_id,group_id) VALUES ({aid},{gid})')
    kid,_=create_key(token,gid,'persisted-image-key')
    sql(f"INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status) VALUES ({uid},{gid},NOW(),NOW()+INTERVAL '1 day','active')")
    pristine=snapshot(uid,kid);migrations=int(sql('SELECT COUNT(*) FROM schema_migrations'))
    for stage,image,fixed in [('BASELINE',BASELINE,False),('MODIFIED',CANDIDATE,True),('ROLLBACK',BASELINE,False),('RESTORED',CANDIDATE,True)]:
        if stage!='BASELINE':start(image,True)
        token,current=login();assert current==uid and snapshot(uid,kid)==pristine
        assert all(request(path)[0]==200 for path in ['/health','/ready','/']), (stage,'health/ready/frontend')
        current_migrations=int(sql('SELECT COUNT(*) FROM schema_migrations'))
        assert current_migrations==migrations+(stage!='BASELINE'),(stage,current_migrations,migrations)
        if stage=='MODIFIED':assert sql(f'SELECT image_count_billing FROM groups WHERE id={gid}')=='f'
        outcomes=[]
        for enabled in [False,True]:
            code,_=request('/api/v1/admin/groups/'+str(gid),{'image_count_billing':enabled,'allow_image_generation':True},token,method='PUT')
            assert code==200,('update group',stage,code)
            for mixed,stream in [(False,False),(False,True),(True,False),(True,True)]:
                name=f'{stage}-{enabled}-{mixed}-{stream}'
                case_kid,key=create_key(token,gid,name)
                body={'model':'gpt-4.1' if mixed else 'gpt-image-2','stream':stream}
                if mixed:body.update(input='fixture-text',tools=[{'type':'image_generation'}])
                else:body.update(prompt='fixture image',n=7,size='1024x1024',response_format='b64_json')
                code,response=request('/v1/responses' if mixed else '/v1/images/generations',body,key)
                assert code==200,(stage,enabled,mixed,stream,code,str(response)[:180])
                rows=usage_row(case_kid);assert len(rows)==1,(stage,name,len(rows))
                row=rows[0];want=.6 if fixed and enabled and not mixed else .0045
                assert abs(row['total']-want)<1e-8,(stage,name,row,want)
                assert (row['input'],row['output'])==(1000,100),(stage,name,row)
                if not mixed:assert row['count']==2,(stage,name,row)
                outcomes.append({'enabled':enabled,'mixed':mixed,'stream':stream,'http':code,'requested_images':7 if not mixed else None,'reported_images':row['count'],'cost':row['total'],'mode':row['mode'],'settlement_rows':1})
        # A pre-existing source field stays in the database when an old writer updates the group.
        if stage=='ROLLBACK':assert sql(f'SELECT image_count_billing FROM groups WHERE id={gid}')=='t'
        assert sql(f'SELECT ABS(balance+(SELECT COALESCE(SUM(actual_cost),0) FROM usage_logs WHERE user_id={uid})-1000)<0.00000001 FROM users WHERE id={uid}')=='t'
        assert sql(f"SELECT status || '/' || schedulable FROM accounts WHERE id={aid}")=='active/true'
        assert snapshot(uid,kid)==pristine
        item={'stage':stage,'result':'PASS','cases':outcomes,'migrations':current_migrations,'persisted_identity_key_subscription_sha256':hashlib.sha256(pristine.encode()).hexdigest(),'health_ready_frontend_encrypted_login':'PASS','balance_and_account_invariants':'PASS'}
        report.append(item);print(json.dumps(item),flush=True)
    (ROOT/'result.json').write_text(json.dumps(report,indent=2)+'\n')
finally:
    compose('down','--volumes','--remove-orphans')
