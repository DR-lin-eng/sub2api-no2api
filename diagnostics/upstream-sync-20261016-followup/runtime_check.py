#!/usr/bin/env python3
"""One disposable database: baseline -> modified -> rollback -> restored.
Secrets are ephemeral, held only in memory, and excluded from reported output.
"""
import hashlib
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parent
COMPOSE = ['docker','compose','-f',str(ROOT/'compose.yaml'),'-p','sub2api-followup-20261016-2ca9']
BASE = 'http://127.0.0.1:18126'
BASELINE = 'ghcr.io/dr-lin-eng/sub2api-no2api:sha-eb130c4'
CANDIDATE = os.environ.get('SYNC_CANDIDATE_IMAGE', 'sub2api:upstream-sync-20261016-followup')
ENV = {**os.environ,'SYNC_DB_PASSWORD':secrets.token_urlsafe(24),'SYNC_ADMIN_EMAIL':'admin@fixture.invalid',
       'SYNC_ADMIN_PASSWORD':secrets.token_urlsafe(24),'SYNC_JWT_SECRET':secrets.token_urlsafe(48),
       'SYNC_ENCRYPTION_KEY':secrets.token_hex(32),'SYNC_IMAGE':BASELINE}
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}),urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
REPORT = []

def compose(*args):
    result = subprocess.run(COMPOSE+list(args),env=ENV,capture_output=True,text=True)
    if result.returncode:
        raise RuntimeError('compose '+str(args)+': '+result.stderr)
    return result.stdout.strip()

def sql(statement):
    return compose('exec','-T','postgres','psql','-U','sub2api','-d','upgrade','-v','ON_ERROR_STOP=1','-qAtc',statement)

def request(path, body=None, token=None):
    headers={'Content-Type':'application/json'}
    if token: headers['Authorization']='Bearer '+token
    req=urllib.request.Request(BASE+path, data=None if body is None else json.dumps(body).encode(),headers=headers)
    try: response=OPENER.open(req,timeout=20)
    except urllib.error.HTTPError as error: response=error
    with response:
        raw=response.read().decode()
        try: content=json.loads(raw)
        except ValueError: content=raw
        return response.code,content

def start(image, installed=False):
    ENV['SYNC_IMAGE']=image
    old_email,old_password=ENV['SYNC_ADMIN_EMAIL'],ENV['SYNC_ADMIN_PASSWORD']
    if installed: ENV['SYNC_ADMIN_EMAIL'],ENV['SYNC_ADMIN_PASSWORD']='invalid-install-email','short'
    compose('rm','-sf','provider','app')
    compose('up','-d','app')
    compose('up','-d','--no-deps','provider')
    ENV['SYNC_ADMIN_EMAIL'],ENV['SYNC_ADMIN_PASSWORD']=old_email,old_password
    for _ in range(80):
        try:
            if request('/ready')[0]==200: return
        except OSError: pass
        time.sleep(.5)
    raise AssertionError('app failed readiness')

def login():
    status,key=request('/api/v1/auth/credential-key')
    assert status==200, ('credential key',status)
    envelope=json.loads(subprocess.check_output(['docker','run','--rm','-i','-v',str(ROOT)+':/fixture:ro',
        'node:24-alpine','node','/fixture/credential_envelope_fixture.mjs'],
        input=json.dumps({'server':key['data'],'email':ENV['SYNC_ADMIN_EMAIL'],'password':ENV['SYNC_ADMIN_PASSWORD']}).encode()))
    status,result=request('/api/v1/auth/login',{'credential_envelope':envelope})
    assert status==200,('encrypted login',status)
    return result['data']['access_token'],result['data']['user']['id']

def snapshot(uid,key_id):
    return sql(f"""SELECT json_build_object('user',(SELECT jsonb_build_array(id,email,password_hash) FROM users WHERE id={uid}),
       'key',(SELECT jsonb_build_array(id,name,quota) FROM api_keys WHERE id={key_id}),
       'subscriptions',(SELECT jsonb_agg(jsonb_build_array(user_id,group_id,expires_at,status)) FROM user_subscriptions),
       'migrations',(SELECT COUNT(*) FROM schema_migrations))""")

try:
    compose('up','-d','postgres','redis')
    start(BASELINE)
    token,uid=login()
    gid=int(sql("INSERT INTO groups(name,platform,status,rate_multiplier) VALUES ('followup-fixture','openai','active',1) RETURNING id"))
    sql(f"UPDATE users SET balance=100 WHERE id={uid}")
    aid=int(sql("""INSERT INTO accounts(name,platform,type,credentials,extra,status,schedulable,concurrency,egress_mode)
        VALUES ('offline-responses','openai','apikey',
        '{"api_key":"fixture-local","base_url":"http://127.0.0.1:8000","model_mapping":{"gpt-5.4":"gpt-5.4"}}',
        '{"openai_responses_supported":true}', 'active',true,3,'direct') RETURNING id"""))
    sql(f"INSERT INTO account_groups(account_id,group_id) VALUES ({aid},{gid})")
    status,key=request('/api/v1/keys',{'name':'followup-runtime-key','group_id':gid,'quota':12.34},token)
    assert status==200, ('create key',status)
    key_id,api_key=key['data']['id'],key['data']['key']
    sql(f"INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status) VALUES ({uid},{gid},NOW(),NOW()+INTERVAL '1 day','active')")
    original_snapshot=snapshot(uid,key_id)
    for stage,image,fixed in [('BASELINE',BASELINE,False),('MODIFIED',CANDIDATE,True),('ROLLBACK',BASELINE,False),('RESTORED',CANDIDATE,True)]:
        if stage!='BASELINE': start(image,True)
        assert request('/health')[0]==200 and request('/ready')[0]==200 and request('/')[0]==200
        token,current_uid=login()
        assert current_uid==uid and original_snapshot==snapshot(uid,key_id),stage+' persisted invariant changed'
        status,key_info=request(f'/api/v1/keys/{key_id}',token=token)
        assert status==200 and key_info['data']['quota']==12.34
        status,models=request('/v1/models',token=api_key)
        assert status==200 and models['data'], (stage,'models',status)
        standard_models=all(all(field in model for field in ['object','created','owned_by']) for model in models['data'])
        assert standard_models==fixed,(stage,'OpenAI model wire fields',standard_models)
        assert all(model.get('created_at')=='2024-01-01T00:00:00Z' for model in models['data']), 'legacy model display field changed'
        flags=[]
        for stream in [False,True]:
            status,result=request('/v1/responses',{'model':'gpt-5.4','stream':stream,'input':'fixture hi'},api_key)
            assert status==200,(stage,'responses',stream,status,str(result)[:250])
            if stream:
                objects=[json.loads(line[6:]).get('response') for line in result.splitlines() if line.startswith('data: {')]
                objects=[obj for obj in objects if obj is not None]
                found=bool(objects) and all(isinstance(obj.get('created_at'),int) for obj in objects)
                if fixed: assert len(set(obj['created_at'] for obj in objects))==1,'unstable stream timestamp'
            else: found=isinstance(result.get('created_at'),int)
            flags.append(found)
            assert found==fixed,(stage,'created_at',stream,found)
        row={'stage':stage,'result':'PASS','responses_json_created_at':flags[0],'responses_stream_created_at':flags[1],
             'migration_count':int(sql('SELECT COUNT(*) FROM schema_migrations')),
             'persisted_invariants_sha256':hashlib.sha256(original_snapshot.encode()).hexdigest(),
             'models_standard_fields':standard_models,'health_ready_frontend_encrypted_login_key_subscription':'PASS'}
        REPORT.append(row)
        print(json.dumps(row),flush=True)
    (ROOT/'runtime-result.json').write_text(json.dumps(REPORT,indent=2)+'\n')
finally:
    compose('down','--volumes','--remove-orphans')
