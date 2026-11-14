#!/usr/bin/env python3
"""Same-database upgrade/rollback with ephemeral credentials kept in memory."""
from concurrent.futures import ThreadPoolExecutor
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
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'compose.yaml'), '-p', 'sub2api-sync-20261112']
BASELINE = 'sub2api:sync-baseline-20261112'
CANDIDATE = 'sub2api:sync-candidate-20261112'
ENV = {**os.environ, 'SYNC_DB_PASSWORD': secrets.token_urlsafe(24), 'SYNC_ADMIN_EMAIL': 'admin@fixture.invalid',
       'SYNC_ADMIN_PASSWORD': 'FixtureSync20261112!', 'SYNC_JWT_SECRET': secrets.token_urlsafe(48),
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
    req = urllib.request.Request('http://127.0.0.1:18562' + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
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


def login_data():
    status, key = request('/api/v1/auth/credential-key')
    assert status == 200, ('credential key', status)
    envelope = json.loads(subprocess.check_output(['docker', 'run', '--rm', '-i', '-v', str(ROOT) + ':/fixture:ro',
        'node:24-alpine', 'node', '/fixture/credential_envelope_fixture.mjs'],
        input=json.dumps({'server': key['data'], 'email': ENV['SYNC_ADMIN_EMAIL'], 'password': ENV['SYNC_ADMIN_PASSWORD']}).encode()))
    status, result = request('/api/v1/auth/login', {'credential_envelope': envelope})
    assert status == 200, ('encrypted login', status)
    return result['data']


def login():
    data = login_data()
    return data['access_token'], data['user']['id']


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
    req=urllib.request.Request('http://127.0.0.1:18562'+path,data=None if body is None else json.dumps(body).encode(),headers={'Content-Type':'application/json','x-api-key':key})
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



def update_extra(aid, value):
    sql(f"UPDATE accounts SET extra='{json.dumps(value)}'::jsonb WHERE id={aid}")


def check_stage(stage, token, uid, fixed):
    cases = []
    gid, low = create_fixture('images-' + stage, 'openai', 'gpt-image-1', [{'platform':'openai','models':['gpt-image-1'],'billing_mode':'image','per_request_price':.1}])
    low_extra = {'openai_responses_supported': True, 'openai_image_size_tiers': ['1K']}
    update_extra(low, low_extra)
    sql(f"UPDATE accounts SET credentials=jsonb_set(credentials,'{{api_key}}','\"fixture-low\"'),priority=0 WHERE id={low}")
    high = int(sql(f"INSERT INTO accounts(name,platform,type,credentials,extra,status,schedulable,concurrency,priority,egress_mode) SELECT 'high-{stage}',platform,type,jsonb_set(credentials,'{{api_key}}','\"fixture-high\"'),'{{\"openai_responses_supported\":true,\"openai_image_size_tiers\":[\"4K\"]}}',status,schedulable,concurrency,9,egress_mode FROM accounts WHERE id={low} RETURNING id"))
    sql(f'INSERT INTO account_groups(account_id,group_id) VALUES ({high},{gid})')
    kid, key = create_key(token, gid, stage + '-images')
    code, body = request('/v1/images/generations', {'model':'gpt-image-1','prompt':'fixture','size':'3840x2160','n':1}, key)
    assert code == 200, (stage, 'image generation', code, body)
    sent = provider('/fixture/last')
    assert sent['label'] == ('fixture-high' if fixed else 'fixture-low'), (stage, 'tier routing', sent['label'])
    rows = usage_row(kid)
    assert len(rows) == 1, (stage, 'image duplicate settlement', rows)
    cases.append({'case':'image-4k', 'selected_fixture':sent['label'], 'settlement':rows})
    for stream in (False, True):
        kid, key = create_key(token, gid, stage + '-responses-' + str(stream))
        code, body = request('/v1/responses', {'model':'gpt-5.4','stream':stream,'instructions':'client guidance','input':'hello'}, key)
        assert code == 200, (stage, 'responses', stream, code, str(body)[:200])
        if stream:
            events = [json.loads(line[6:]) for line in body.splitlines() if line.startswith('data: {')]
            completed = [event['response'] for event in events if event.get('type') == 'response.completed']
            assert completed and completed[-1]['instructions'] == 'client guidance'
        else:
            assert body['instructions'] == 'client guidance', (stage, 'instructions', body)
        rows = usage_row(kid)
        assert len(rows) == 1 and rows[0]['input'] == 1000 and rows[0]['output'] == 100, (stage, 'responses settlement', rows)
        cases.append({'case':'responses', 'stream':stream, 'settlement':rows})
    kid, key = create_key(token,gid,stage+'-buffered-reasoning')
    code, body = request('/v1/chat/completions',{'model':'gpt-5.4','stream':False,'messages':[{'role':'user','content':'reasoning-fixture'}]},key)
    assert code == 200,(stage,'buffered reasoning',code,str(body)[:200])
    reasoning=body['choices'][0]['message'].get('reasoning_content','')
    expected='think more'
    assert reasoning == expected,(stage,'reasoning fallback',reasoning)
    assert len(usage_row(kid)) == 1,(stage,'buffered reasoning settlement')
    cases.append({'case':'buffered-reasoning-fallback','plaintext_present':bool(reasoning),'settled_once':True})
    grok_gid, grok_aid = create_fixture('grok-' + stage, 'grok', 'grok-4.5', [])
    update_extra(grok_aid, {'openai_responses_supported':True})
    kid, key = create_key(token, grok_gid, stage + '-grok')
    code, body = request('/v1/responses', {'model':'grok-4.5','stream':False,'input':[{'type':'agent_message','content':[{'type':'input_text','text':'Task:'},{'type':'encrypted_content','encrypted_content':'plaintext body'}]}]}, key)
    assert code == 200, (stage, 'grok', code, str(body)[:200])
    sent = provider('/fixture/last')['body']['input']
    if fixed:
        assert sent[0]['role'] == 'user' and sent[0]['content'] == 'Task:plaintext body', (stage, 'grok delegation', sent)
    else:
        assert sent[0]['type'] == 'agent_message', (stage, 'baseline grok', sent)
    assert len(usage_row(kid)) == 1
    cases.append({'case':'grok-delegation', 'normalized':fixed})
    kimi_gid, kimi_aid = create_fixture('kimi-' + stage, 'kimi', 'kimi-k2.5', [])
    kid, key = create_key(token, kimi_gid, stage + '-kimi')
    code, _ = request('/v1/chat/completions', {'model':'kimi-k2.5','stream':False,'messages':[{'role':'user','content':'hello'}]}, key)
    state = json.loads(sql(f"SELECT json_build_object('status',status,'rate_limited',rate_limit_reset_at > NOW(),'temporary',temp_unschedulable_until > NOW()) FROM accounts WHERE id={kimi_aid}"))
    assert state['status'] == ('active' if fixed else 'error'), (stage, 'kimi state', code, state)
    if fixed: assert state['rate_limited'] is True, (stage, 'short cooldown', state)
    cases.append({'case':'kimi-concurrency-403', 'http':code, 'state':state})
    # The same old Redis token format must remain readable after either binary switch.
    session = login_data()
    code, rotated = request('/api/v1/auth/refresh', {'refresh_token': session['refresh_token']})
    assert code == 200 and rotated['data']['refresh_token'] != session['refresh_token'], (stage, 'refresh rotation', code)
    code, _ = request('/api/v1/auth/refresh', {'refresh_token': session['refresh_token']})
    assert code == 401, (stage, 'used refresh token rejected', code)
    cases.append({'case': 'refresh-rotation-and-reuse', 'single_use': True})
    if stage in ('MODIFIED','RESTORED'):
        session = login_data()
        raw = json.dumps({'refresh_token': session['refresh_token']}).encode()
        def concurrent_refresh(_):
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            req = urllib.request.Request('http://127.0.0.1:18562/api/v1/auth/refresh', data=raw,
                headers={'Content-Type':'application/json'})
            try:
                response = opener.open(req, timeout=15)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                response.read()
                return response.code
        with ThreadPoolExecutor(max_workers=8) as pool:
            codes = list(pool.map(concurrent_refresh, range(8)))
        assert codes.count(200) == 1 and codes.count(401) == 7, (stage, 'atomic concurrent refresh', codes)
        cases.append({'case':'atomic-concurrent-refresh', 'callers':8, 'successes':1, 'rejected':7})

    code, _ = request('/api/v1/admin/payment/config', {'balance_recharge_multiplier':0.140845}, token, 'PUT')
    assert code == 200, (stage, 'save multiplier', code)
    code, cfg = request('/api/v1/admin/payment/config', token=token)
    assert code == 200, (stage, 'read multiplier', code)
    precision = 0.140845 if stage in ('MODIFIED','RESTORED') else 0.14
    assert cfg['data']['balance_recharge_multiplier'] == precision, (stage, 'multiplier precision', cfg)
    cases.append({'case':'multiplier-save-readback', 'value':precision})
    # Restore the old supported value before switching binaries.
    assert request('/api/v1/admin/payment/config', {'balance_recharge_multiplier':1}, token, 'PUT')[0] == 200

    native_gid, _ = create_fixture('native-catalog-' + stage, 'anthropic', 'claude-sonnet-4-6', [])
    _, native_key = create_key(token, native_gid, stage + '-catalog')
    code, catalog = request('/v1/models', token=native_key)
    assert code == 200, (stage, 'native catalog', code)
    ids = {item['id'] for item in catalog['data']}
    for model_id in ['claude-fable-5-2','claude-mythos-5-2']:
        assert (model_id in ids) == (stage in ('MODIFIED','RESTORED')), (stage, 'model listing', model_id)
    cases.append({'case':'claude52-model-listing', 'visible':stage in ('MODIFIED','RESTORED')})

    # Page snapshots enter the current shared bootstrap before orphan tool validation.
    kid, key = create_key(token, gid, stage + '-page-context')
    page = {'type':'function_call_output', 'namespace':'agent', 'name':'codex_apps_open_page',
            'output':json.dumps({'page_id':'fixture-page','text':'untrusted fixture'})}
    code, body = request('/v1/responses', {'model':'gpt-5.4','stream':False,'input':[page]}, key)
    changed = stage in ('MODIFIED','RESTORED')
    if changed:
        assert code == 200, (stage, 'page context', code, str(body)[:120])
        sent = provider('/fixture/last')['body']['input'][0]
        assert sent['role'] == 'user' and '<external_codex_apps_open_page>' in sent['content'][0]['text']
        assert len(usage_row(kid)) == 1
    else:
        assert code == 400, (stage, 'old page-context behavior', code)
    cases.append({'case':'page-context', 'http':code, 'normalized':changed})
    return cases




report = []
try:
    compose('up', '-d', 'postgres', 'redis')
    start(BASELINE)
    token, uid = login()
    ack = json.dumps({'version':'v2026.06.10','admin_user_id':uid,'accepted_at':'2026-11-04T00:00:00Z'})
    sql(f"INSERT INTO settings(key,value) VALUES ('admin_compliance_acknowledgement:{uid}','{ack}')")
    sql(f'UPDATE users SET balance=1000 WHERE id={uid}')
    gid, _ = create_fixture('persisted', 'openai', 'gpt-5.4', [])
    kid, _ = create_key(token, gid, 'persisted-key')
    sql(f"INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status) VALUES ({uid},{gid},NOW(),NOW()+INTERVAL '1 day','active')")
    pristine = snapshot(uid, kid)
    migrations = int(sql('SELECT COUNT(*) FROM schema_migrations'))
    historical = None
    for stage, image, fixed in [('BASELINE',BASELINE,True),('MODIFIED',CANDIDATE,True),('ROLLBACK',BASELINE,True),('RESTORED',CANDIDATE,True)]:
        if stage != 'BASELINE':
            # Old nodes ignore tier configuration: remove restrictions before rollback.
            if not fixed: sql("UPDATE accounts SET extra=extra-'openai_image_size_tiers'")
            start(image, True)
        token, uid = login()
        if historical:
            upper, digest = historical
            assert sql(f"SELECT md5(COALESCE(string_agg(row_to_json(u)::text,',' ORDER BY id),'')) FROM usage_logs u WHERE id<={upper}") == digest
        cases = check_stage(stage, token, uid, True)
        assert snapshot(uid, kid) == pristine, (stage, 'persistent identity/key/subscription')
        expected_migrations = migrations + (0 if stage == 'BASELINE' else 1)
        assert int(sql('SELECT COUNT(*) FROM schema_migrations')) == expected_migrations, (stage, 'migration drift')
        if stage != 'BASELINE':
            assert sql("SELECT indisvalid FROM pg_index WHERE indexrelid='idx_usage_logs_api_key_created_cost'::regclass") == 't'
        assert sql(f'SELECT ABS(balance+(SELECT COALESCE(SUM(actual_cost),0) FROM usage_logs WHERE user_id={uid})-1000)<0.00000001 FROM users WHERE id={uid}') == 't', (stage, 'balance equation')
        upper = int(sql('SELECT COALESCE(MAX(id),0) FROM usage_logs'))
        historical = (upper, sql(f"SELECT md5(COALESCE(string_agg(row_to_json(u)::text,',' ORDER BY id),'')) FROM usage_logs u WHERE id<={upper}"))
        item = {'stage':stage, 'result':'PASS', 'cases':cases, 'migrations':expected_migrations, 'persistent_identity_key_subscription':'PASS', 'historical_usage_unchanged':'PASS', 'balance_settlement_identity':'PASS'}
        report.append(item)
        print(json.dumps(item), flush=True)
    (ROOT / 'result.json').write_text(json.dumps(report, indent=2) + '\n')
    if os.environ.get('SYNC_KEEP_UI') == '1':
        # Local disposable fixture credentials only. JWT/encryption keys remain in memory.
        print('READY_FOR_UI http://127.0.0.1:18562 admin@fixture.invalid', flush=True)
        finish = ROOT / '.ui-finished'
        deadline = time.monotonic() + 1800
        while not finish.exists() and time.monotonic() < deadline:
            time.sleep(1)
        finish.unlink(missing_ok=True)
finally:
    compose('down', '--volumes', '--remove-orphans')
