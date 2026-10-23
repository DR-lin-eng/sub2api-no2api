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
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'compose.yaml'), '-p', 'sub2api-sync-20261024-3998']
BASELINE = 'ghcr.io/dr-lin-eng/sub2api-no2api:sha-7161a54'
CANDIDATE = 'sub2api:upstream-sync-20261024'
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
    req = urllib.request.Request('http://127.0.0.1:18240' + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
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

report=[]
try:
    compose('up','-d','postgres','redis')
    start(BASELINE)
    token,uid=login()
    ack=json.dumps({'version':'v2026.06.10','admin_user_id':uid,'accepted_at':'2026-10-24T00:00:00Z'})
    sql(f"INSERT INTO settings(key,value) VALUES ('admin_compliance_acknowledgement:{uid}','{ack}')")
    sql(f'UPDATE users SET balance=1000 WHERE id={uid}')
    gid=int(sql("INSERT INTO groups(name,platform,status,rate_multiplier) VALUES ('capacity-fixture','openai','active',1) RETURNING id"))
    aid=int(sql('''INSERT INTO accounts(name,platform,type,credentials,extra,status,schedulable,concurrency,egress_mode) VALUES ('offline-responses','openai','apikey','{"api_key":"fixture","base_url":"http://127.0.0.1:8000","pool_mode":true,"pool_mode_retry_count":1}','{}','active',true,3,'direct') RETURNING id'''))
    sql(f'INSERT INTO account_groups(account_id,group_id) VALUES ({aid},{gid})')
    kid,_=create_key(token,gid,'persisted-capacity-key')
    sql(f"INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status) VALUES ({uid},{gid},NOW(),NOW()+INTERVAL '1 day','active')")
    pristine=snapshot(uid,kid);migrations=int(sql('SELECT COUNT(*) FROM schema_migrations'))
    plan=[('BASELINE',BASELINE,False),('MODIFIED',CANDIDATE,True),('ROLLBACK',BASELINE,False),('RESTORED',CANDIDATE,True)]
    if '--baseline-only' in sys.argv:plan=plan[:1]
    for stage,image,fixed in plan:
        if stage!='BASELINE':start(image,True)
        token,current=login();assert current==uid and snapshot(uid,kid)==pristine
        assert all(request(p)[0]==200 for p in ['/health','/ready','/'])
        assert int(sql('SELECT COUNT(*) FROM schema_migrations'))==migrations
        outcomes=[]
        for mode,stream in [('http',False),('http',True),('sse',False),('sse',True),('visible',True),('invalid',False)]:
            name='fixture-'+stage+'-'+mode+('-stream' if stream else '-buffered')
            case_kid,key=create_key(token,gid,name)
            body={'model':'gpt-4.1','stream':stream,'input':name}
            code,response=request('/v1/responses',body,key)
            stats=json.loads(compose('exec','-T','provider','python','-c',"import urllib.request; print(urllib.request.urlopen('http://127.0.0.1:8000/stats').read().decode())"))
            assert name in stats,(stage,mode,code,str(response)[:150],stats)
            attempts=stats[name]
            if mode in ['http','sse']:
                assert code==200 and 'fixture-end' in str(response),(stage,mode,stream,code,str(response)[:150])
                assert len(attempts)==2,(stage,mode,attempts)
                gap=attempts[1]-attempts[0]
                assert gap>=(1.15 if fixed else .4),(stage,mode,gap)
                if not fixed:assert gap<1.15,(stage,mode,gap)
                rows=settled(case_kid);assert len(rows)==1 and (rows[0]['input'],rows[0]['output'])==(10,5),(stage,rows)
                outcomes.append({'case':mode,'stream':stream,'http':code,'attempts':2,'retry_gap_seconds':round(gap,4),'settlement_rows':1})
            elif mode=='visible':
                assert code==200 and str(response).count('visible-once')==1 and len(attempts)==1,(stage,code,len(attempts))
                outcomes.append({'case':mode,'stream':stream,'http':code,'attempts':1,'visible_output_count':1})
            else:
                assert code==400 and len(attempts)==1,(stage,code,len(attempts))
                outcomes.append({'case':mode,'stream':stream,'http':code,'attempts':1})
        assert sql(f"SELECT status || '/' || schedulable FROM accounts WHERE id={aid}")=='active/true'
        assert sql(f'SELECT ABS(balance+(SELECT COALESCE(SUM(actual_cost),0) FROM usage_logs WHERE user_id={uid})-1000)<0.00000001 FROM users WHERE id={uid}')=='t'
        item={'stage':stage,'result':'PASS','cases':outcomes,'migrations':migrations,'persisted_identity_key_subscription_sha256':hashlib.sha256(pristine.encode()).hexdigest(),'health_ready_frontend_encrypted_login':'PASS','account_and_balance_invariants':'PASS'}
        report.append(item);print(json.dumps(item),flush=True)
    (ROOT/('baseline-fixture.json' if '--baseline-only' in sys.argv else 'result.json')).write_text(json.dumps(report,indent=2)+'\n')
finally:
    compose('down','--volumes','--remove-orphans')
