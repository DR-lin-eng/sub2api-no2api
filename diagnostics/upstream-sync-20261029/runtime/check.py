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
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'compose.yaml'), '-p', 'sub2api-sync-20261029']
BASELINE = 'ghcr.io/dr-lin-eng/sub2api-no2api:sha-c050e6c'
CANDIDATE = 'sub2api:upstream-sync-20261029'
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
    req = urllib.request.Request('http://127.0.0.1:18380' + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
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
    req=urllib.request.Request('http://127.0.0.1:18380'+path,data=None if body is None else json.dumps(body).encode(),headers={'Content-Type':'application/json','x-api-key':key})
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

def detailed_usage(kid):
    for _ in range(100):
        rows=json.loads(sql(f"SELECT COALESCE(json_agg(json_build_object('input',input_tokens,'output',output_tokens,'cache',cache_read_tokens,'images',image_count,'cost',total_cost,'actual',actual_cost,'mode',billing_mode)),'[]'::json) FROM usage_logs WHERE api_key_id={kid}"))
        if rows:return rows
        time.sleep(.1)
    raise AssertionError('missing reliable settlement')


def create_fixture(name, platform, model, pricing):
    card=json.dumps(pricing)
    gid=int(sql(f"INSERT INTO groups(name,platform,status,rate_multiplier,model_pricing,allow_messages_dispatch) VALUES ('{name}','{platform}','active',1,'{card}',true) RETURNING id"))
    credentials=json.dumps({'api_key':'fixture','base_url':'http://127.0.0.1:8000','api_protocol':'chat_completions','account_mode':'payg'})
    extra=json.dumps({'openai_responses_supported':False})
    aid=int(sql(f"INSERT INTO accounts(name,platform,type,credentials,extra,status,schedulable,concurrency,egress_mode) VALUES ('{name}','{platform}','apikey','{credentials}','{extra}','active',true,3,'direct') RETURNING id"))
    sql(f'INSERT INTO account_groups(account_id,group_id) VALUES ({aid},{gid})')
    if platform=='openai':sql(f'UPDATE groups SET allow_image_generation=true WHERE id={gid}')
    return gid,aid


def raw_upload(key):
    boundary='sub2api-fixture-boundary'
    raw=(f'--{boundary}\r\nContent-Disposition: form-data; name="model"\r\n\r\ngpt-image-1\r\n--{boundary}\r\nContent-Disposition: form-data; name="prompt"\r\n\r\nfixture\r\n--{boundary}\r\nContent-Disposition: form-data; name="image"; filename="fixture.png"\r\nContent-Type: image/png\r\n\r\n'.encode()+b'x'*(21<<20)+f'\r\n--{boundary}--\r\n'.encode())
    req=urllib.request.Request('http://127.0.0.1:18380/v1/images/edits',data=raw,headers={'Authorization':'Bearer '+key,'Content-Type':'multipart/form-data; boundary='+boundary})
    try:response=OPENER.open(req,timeout=30)
    except urllib.error.HTTPError as error:response=error
    with response:return response.code,len(raw),response.read().decode()



def sync_regressions(stage,token,uid):
    candidate = stage in ('MODIFIED','RESTORED')
    channel=int(sql(f"INSERT INTO channels(name,status,features_config) VALUES ('sync-{stage}','active','{{\"web_search_emulation\":{{\"anthropic\":true,\"openai\":true}}}}') RETURNING id"))
    status,value=request(f'/api/v1/admin/channels/{channel}', {'features_config':{'web_search_emulation':{'anthropic':False}}},token,method='PUT')
    assert status==200,(stage,'channel update',status,value)
    stored=json.loads(sql(f'SELECT features_config FROM channels WHERE id={channel}'))
    assert bool(stored['web_search_emulation'].get('openai'))==candidate,(stage,'platform dimension',stored)
    guard=int(sql(f"INSERT INTO users(email,password_hash,role,status,balance,concurrency) VALUES ('guard-{stage}@fixture.invalid','unused','admin','active',0,1) RETURNING id"))
    guard_status,guard_body=request(f'/api/v1/admin/users/{guard}', {'status':'disabled'},token,method='PUT')
    assert guard_status==(400 if candidate else 500),(stage,'admin guard',guard_status,guard_body)
    if candidate: assert guard_body['reason']=='ADMIN_DISABLE_FORBIDDEN',guard_body
    gid,aid=create_fixture('mapped-'+stage,'gemini','gemini-alpha',[])
    creds={'api_key':'fixture','base_url':'http://127.0.0.1:8000','model_mapping':{'gemini-alpha':'gemini-3.8-flash','gemini-beta*':'gemini-3.8-flash'}}
    sql(f"UPDATE accounts SET credentials='{json.dumps(creds)}' WHERE id={aid}")
    metadata_id,metadata_key=create_key(token,gid,'metadata-'+stage)
    metadata_status,metadata=request('/v1beta/models',token=metadata_key)
    assert metadata_status==200,(stage,'metadata',metadata_status,metadata)
    model_names=[model['name'] for model in metadata['models']]
    assert model_names==(['models/gemini-alpha','models/gemini-beta*'] if candidate else ['models/upstream-only']),(stage,'declarations',model_names)
    response_gid,response_aid=create_fixture('incomplete-'+stage,'openai','fixture-incomplete',[{'platform':'openai','models':['fixture-incomplete'],'billing_mode':'token','input_price':.000001,'output_price':.000002}])
    response_extra={'openai_responses_supported':True,'openai_passthrough':True}
    sql(f"UPDATE accounts SET extra='{json.dumps(response_extra)}' WHERE id={response_aid}")
    response_id,response_key=create_key(token,response_gid,'incomplete-'+stage)
    response_status,response=request('/v1/responses',{'model':'fixture-incomplete','instructions':'fixture','input':'hello','stream':False},response_key)
    assert response_status==200,(stage,'nonstream',response_status,response)
    if candidate: assert isinstance(response,dict) and response['status']=='incomplete',(stage,'incomplete json',response)
    else: assert isinstance(response,str) and 'response.incomplete' in response,(stage,'baseline sse',response)
    settled_rows=detailed_usage(response_id)
    assert len(settled_rows)==1 and settled_rows[0]['input']==1000 and settled_rows[0]['output']==100,(stage,'nonstream settlement',settled_rows)
    return {'admin_guard_http':guard_status,'channel_openai_preserved':candidate,'declared_models':model_names,'incomplete_json':candidate,'incomplete_settlement':settled_rows}

report=[]
try:
    compose('up','-d','postgres','redis')
    start(BASELINE)
    token,uid=login()
    ack=json.dumps({'version':'v2026.06.10','admin_user_id':uid,'accepted_at':'2026-10-28T00:00:00Z'})
    sql(f"INSERT INTO settings(key,value) VALUES ('admin_compliance_acknowledgement:{uid}','{ack}')")
    sql(f'UPDATE users SET balance=1000 WHERE id={uid}')
    fixtures={}
    fixtures['default']=create_fixture('deepseek-default','deepseek','deepseek-v4-pro',[])
    fixtures['custom']=create_fixture('deepseek-custom','deepseek','deepseek-v4-pro',[{'platform':'deepseek','models':['deepseek-v4-pro'],'billing_mode':'token','input_price':.000001,'output_price':.000002,'cache_read_price':.0000004}])
    fixtures['gemini']=create_fixture('gemini-files','gemini','custom-gemini-fixture',[{'platform':'gemini','models':['custom-gemini-fixture'],'billing_mode':'image','per_request_price':.25}])
    fixtures['upload']=create_fixture('openai-upload','openai','gpt-image-1',[{'platform':'openai','models':['gpt-image-1'],'billing_mode':'image','per_request_price':.1}])
    plaza_gid,_=create_fixture('plaza-fixed','openai','fixture-image',[{'platform':'openai','models':['fixture-image'],'billing_mode':'image','per_request_price':.12}])
    sql(f'UPDATE groups SET image_price_1k=.9 WHERE id={plaza_gid}')
    cid=int(sql("INSERT INTO channels(name,status) VALUES ('plaza-fixture','active') RETURNING id"))
    sql(f"INSERT INTO channel_groups(channel_id,group_id) VALUES ({cid},{plaza_gid})")
    sql(f"INSERT INTO channel_model_pricing(channel_id,platform,models,billing_mode,per_request_price) VALUES ({cid},'openai','[\"fixture-image\"]','image',.5)")
    sql("INSERT INTO settings(key,value) VALUES ('model_plaza_enabled','true') ON CONFLICT(key) DO UPDATE SET value='true'")
    gid,aid=fixtures['default'];kid,_=create_key(token,gid,'persisted-key')
    sql(f"INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status) VALUES ({uid},{gid},NOW(),NOW()+INTERVAL '1 day','active')")
    pristine=snapshot(uid,kid);migrations=int(sql('SELECT COUNT(*) FROM schema_migrations'))
    historical=None
    for stage,image,fixed in [('BASELINE',BASELINE,True),('MODIFIED',CANDIDATE,True),('ROLLBACK',BASELINE,True),('RESTORED',CANDIDATE,True)]:
        if stage!='BASELINE':start(image,True)
        token,current=login();assert current==uid and snapshot(uid,kid)==pristine
        assert all(request(path)[0]==200 for path in ['/health','/ready','/'])
        assert int(sql('SELECT COUNT(*) FROM schema_migrations'))==migrations
        if historical:
            upper,digest=historical
            assert sql(f"SELECT md5(COALESCE(string_agg(row_to_json(u)::text,',' ORDER BY id),'')) FROM usage_logs u WHERE id<={upper}")==digest,'historical usage was changed'
        pages={}
        for path in ['/api/v1/admin/users','/api/v1/keys']:
            code,_=request(path+'?page=9223372036854775807',token=token)
            assert code==(200 if fixed else 500),(stage,'pagination',path,code)
            pages[path]=code
        plaza_code,plaza=request('/api/v1/model-plaza',token=token)
        assert plaza_code==200,(stage,'plaza',plaza_code,plaza)
        cards=[m['pricing'] for g in plaza['data']['groups'] if g['id']==plaza_gid for m in g['models'] if m['name']=='fixture-image']
        assert len(cards)==1,(stage,'plaza fixed card absent',plaza)
        card=cards[0];display=card['intervals'][0]['per_request_price'] if card.get('intervals') else card['per_request_price']
        assert abs(display-(.12 if fixed else .9))<1e-9,(stage,'plaza fixed price',card)
        cases=[]
        for name in ['default','custom']:
            for stream in [False,True]:
                case_gid,_=fixtures[name];case_id,key=create_key(token,case_gid,stage+'-'+name+'-'+str(stream))
                code,value=request('/v1/chat/completions',{'model':'deepseek-v4-pro','messages':[{'role':'developer','content':'fixture'},{'role':'user','content':'fixture'}],'stream':stream},key)
                assert code==200,(stage,name,stream,code,str(value)[:200])
                sent=provider('/fixture/last');assert sent['messages'][0]['role']=='system'
                rows=detailed_usage(case_id);assert len(rows)==1,(stage,name,'duplicate settlement')
                row=rows[0];assert (row['input'],row['output'],row['cache'])==((1000,500,1000) if fixed else (2000,500,0)),(stage,name,row)
                if name=='custom':expected=.0024 if fixed else .003
                else:expected=.001672 if fixed else .0006
                assert abs(row['cost']-expected)<1e-9,(stage,name,row,expected)
                cases.append({'fixture':name,'stream':stream,'http':code,'settlement':row})
        for stream in [False,True]:
            case_gid,_=fixtures['gemini'];case_id,key=create_key(token,case_gid,stage+'-gemini-'+str(stream))
            method='streamGenerateContent' if stream else 'generateContent'
            code,value=request('/v1beta/models/custom-gemini-fixture:'+method,{'contents':[{'role':'user','parts':[{'text':'fixture'}]}]},key)
            assert code==200,(stage,'gemini',stream,code,str(value)[:200])
            rows=detailed_usage(case_id);assert len(rows)==1,(stage,'gemini duplicate settlement')
            row=rows[0];assert row['images']==(2 if fixed else 0),(stage,'gemini',stream,row)
            assert abs(row['cost']-(.5 if fixed else .25))<1e-9,(stage,'gemini cost',row)
            cases.append({'fixture':'gemini-files','stream':stream,'http':code,'settlement':row})
        case_gid,_=fixtures['upload'];case_id,key=create_key(token,case_gid,stage+'-upload')
        code,sent_bytes,value=raw_upload(key)
        assert code==(200 if fixed else 400),(stage,'upload',code,value[:200])
        if fixed:
            sent=provider('/fixture/last');assert sent['image_bytes']==21<<20,(stage,'truncated upload',sent)
            assert sent['image_sha256']==hashlib.sha256(b'x'*(21<<20)).hexdigest(),(stage,'changed image bytes',sent)
            rows=detailed_usage(case_id);assert len(rows)==1
            assert abs(rows[0]['cost']-.1)<1e-9,rows
        else:rows=[]
        cases.append({'fixture':'upload-21MiB','http':code,'bytes':sent_bytes,'settlement':rows})
        regression=sync_regressions(stage,token,uid)
        assert sql(f'SELECT ABS(balance+(SELECT COALESCE(SUM(actual_cost),0) FROM usage_logs WHERE user_id={uid})-1000)<0.00000001 FROM users WHERE id={uid}')=='t'
        assert snapshot(uid,kid)==pristine
        upper=int(sql('SELECT COALESCE(MAX(id),0) FROM usage_logs'))
        historical=(upper,sql(f"SELECT md5(COALESCE(string_agg(row_to_json(u)::text,',' ORDER BY id),'')) FROM usage_logs u WHERE id<={upper}"))
        item={'stage':stage,'sync_regressions':regression,'image_id':subprocess.check_output(['docker','inspect','-f','{{.Image}}',compose('ps','-q','app')],text=True).strip(),'result':'PASS','cases':cases,'pagination':pages,'plaza_fixed_price':display,'migrations':migrations,'persisted_identity_key_subscription_sha256':hashlib.sha256(pristine.encode()).hexdigest(),'historical_usage_unchanged':'PASS','health_ready_frontend_encrypted_login':'PASS','balance_settlement_identity':'PASS'}
        report.append(item);print(json.dumps(item),flush=True)
    (ROOT/'result.json').write_text(json.dumps(report,indent=2)+'\n')
finally:
    compose('down','--volumes','--remove-orphans')
