#!/usr/bin/env python3
"""Only synthetic data in the dedicated sub2api-sync-20261015-7d88 Docker project."""
import json
import http.cookiejar
import os
from pathlib import Path
import re
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parent
PARENT = ROOT.parent
COMPOSE = ['docker', 'compose', '-f', str(PARENT / 'compose.yaml'), '-p', 'sub2api-open-items-20261015-7d88']
BASE = 'http://127.0.0.1:18115'
BASELINE = 'sub2api-upstream-sync:20261015-candidate'
CANDIDATE = 'sub2api-upstream-sync:20261015-open-items'
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}),
                                   urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
REPORT = []


def compose(*args, env=None, capture=False):
    result = subprocess.run(COMPOSE + list(args), env={**os.environ, **(env or {})},
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True)
    return result.stdout.decode() if capture else None


def sql(database, statement):
    return compose('exec', '-T', 'postgres', 'psql', '-U', 'sub2api', '-d', database,
                   '-v', 'ON_ERROR_STOP=1', '-qAtc', statement, capture=True).strip()


def request(path, body=None, token=None, method=None, language=None):
    headers = {'Content-Type': 'application/json'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    if language:
        headers['Accept-Language'] = language
    req = urllib.request.Request(BASE + path, headers=headers,
        data=None if body is None else json.dumps(body).encode(), method=method)
    try:
        response = OPENER.open(req, timeout=20)
    except urllib.error.HTTPError as error:
        return error.code, json.loads(error.read())
    with response:
        content = response.read()
        return response.status, content.decode() if path == '/' else json.loads(content)


def start(image, database='upgrade', email='admin@sub2api.local', password='sync_admin_password', ready=True):
    env = {'SYNC_IMAGE': image, 'SYNC_DB': database,
           'SYNC_ADMIN_EMAIL': email, 'SYNC_ADMIN_PASSWORD': password}
    compose('up', '-d', '--force-recreate', 'app', env=env)
    container = compose('ps', '-aq', 'app', capture=True).strip()
    for _ in range(120):
        state = json.loads(subprocess.check_output(['docker', 'inspect', '-f', '{{json .State}}', container]))
        if not state['Running']:
            assert not ready and state['ExitCode'] != 0, 'unexpected app exit'
            logs = compose('logs', '--no-color', 'app', capture=True)
            assert 'invalid admin' in logs, 'exit was unrelated to credential validation'
            return container, logs
        if ready:
            try:
                if request('/ready')[0] == 200:
                    return container, compose('logs', '--no-color', 'app', capture=True)
            except (OSError, ValueError):
                pass
        time.sleep(0.5)
    raise AssertionError('app did not reach expected ready/exit state')


def login(email, password):
    status, key = request('/api/v1/auth/credential-key')
    assert status == 200
    envelope = json.loads(subprocess.check_output([
        'docker', 'run', '--rm', '-i', '-v', str(PARENT) + ':/fixture:ro',
        'node:24-alpine', 'node', '/fixture/credential_envelope_fixture.mjs'],
        input=json.dumps({'server': key['data'], 'email': email, 'password': password}).encode()))
    status, result = request('/api/v1/auth/login', {'credential_envelope': envelope})
    return status, result


def snapshot():
    # Compare hashes internally; credentials are never included in the report.
    return sql('upgrade', """SELECT json_build_object(
        'users',(SELECT jsonb_agg(jsonb_build_array(id,email,password_hash,balance) ORDER BY id) FROM users),
        'keys',(SELECT jsonb_agg(jsonb_build_array(id,name,quota) ORDER BY id) FROM api_keys),
        'orders',(SELECT jsonb_agg(jsonb_build_array(id,amount,pay_amount,bonus_amount,status) ORDER BY id) FROM payment_orders),
        'migrations',(SELECT COUNT(*) FROM schema_migrations));""")


def runtime(stage, state):
    assert request('/health')[0] == 200 and request('/ready')[0] == 200
    status, html = request('/')
    assert status == 200 and 'app' in html
    status, logged = login('admin@sub2api.local', 'sync_admin_password')
    assert status == 200 and logged['data']['user']['id'] == state['user_id']
    token = logged['data']['access_token']
    status, key = request('/api/v1/keys/' + str(state['key_id']), token=token)
    assert status == 200 and key['data']['name'] == state['name'] and key['data']['quota'] == 12.34
    assert request('/api/v1/settings/public')[0] == 200
    status, order = request('/api/v1/payment/public/orders/verify', {'out_trade_no': 'upstream-sync-20261015-legacy'})
    assert status == 200 and order['data']['status'] == 'PENDING' and order['data']['paid'] is False
    current_snapshot = json.loads(snapshot())
    baseline_snapshot = json.loads(state['snapshot'])
    for key in ('users', 'keys', 'orders'):
        assert current_snapshot.get(key) == baseline_snapshot.get(key), f'upgrade changed persisted {key}'
    expected_migrations = int(baseline_snapshot['migrations']) + (0 if stage == 'baseline' else 1)
    assert current_snapshot['migrations'] == expected_migrations, 'new migration was not applied exactly once'
    REPORT.append({'stage': stage, 'result': 'PASS', 'migrations': int(current_snapshot.get('migrations', 0))})
    print(stage + ': readiness/frontend/encrypted-login/persisted-key/legacy-order/data-snapshot PASS', flush=True)
    return token


def localization(stage,state,token):
    # Parent sync already delivers locale storage in both current-main and new image.
    code,available=request('/api/v1/groups/available',token=token,language='en')
    assert code==200
    group=next(g for g in available['data'] if g['id']==state['group_id'])
    assert group['name']=='默认分组'
    REPORT[-1]['locale_contract']='PASS'
    print(stage+': existing locale contract PASS',flush=True)


def model_metadata(stage,state,token):
    account_id=str(state['account_id'])
    path='/api/v1/admin/accounts/'+account_id
    if stage=='baseline':
        assert sql('upgrade',"SELECT COALESCE(extra -> 'upstream_model_metadata', 'null'::jsonb)::text FROM accounts WHERE id="+account_id)=='null'
    elif stage=='upgraded':
        # Store the same native, versioned representation exercised by the
        # real network catalog and PostgreSQL compare-and-swap tests.
        snapshot={'version':1,'source':'runtime-fixture','generation':1,'started_at':'2026-10-15T00:00:00Z','synced_at':'2026-10-15T00:00:01Z','models':{'gpt-6.1-sol':{'id':'gpt-6.1-sol','context_window':777777,'supports_reasoning':False,'input_modalities':[]}}}
        raw=json.dumps(snapshot)
        sql('upgrade',"UPDATE accounts SET extra=COALESCE(extra,'{}'::jsonb)||jsonb_build_object('upstream_model_metadata', '"+raw.replace("'","''")+"'::jsonb) WHERE id="+account_id)
    if stage in ['rollback','restored']:
        before=json.loads(sql('upgrade',"SELECT (extra -> 'upstream_model_metadata')::text FROM accounts WHERE id="+account_id))
        assert before['models']['gpt-6.1-sol']['context_window']==777777
        code,result=request(path,{'notes':'old-writer-preserves-model-metadata','extra':{}},token,method='PUT')
        assert code==200
    if stage!='baseline':
        snapshot=json.loads(sql('upgrade',"SELECT (extra -> 'upstream_model_metadata')::text FROM accounts WHERE id="+account_id))
        model=snapshot['models']['gpt-6.1-sol']
        assert snapshot['version']==1 and model['context_window']==777777 and model['supports_reasoning'] is False and model['input_modalities']==[]
    REPORT[-1]['metadata_mixed_writer']='PASS'
    print(stage+': metadata snapshot and old image Extra replacement PASS',flush=True)


def account_scope(stage, state, token):
    path = '/api/v1/admin/accounts/' + str(state['account_id'])
    code, result = request(path, token=token)
    assert code == 200
    credentials = result['data']['credentials']
    rule = credentials['temp_unschedulable_rules'][0]
    if stage == 'baseline':
        assert 'scope' not in rule
    elif stage == 'upgraded':
        rule['scope'] = 'account'
        code, result = request(path, {'credentials': credentials}, token, method='PUT')
        assert code == 200, ('candidate rule writer', code)
    else:
        assert rule['scope'] == 'account', ('scope lost after image replacement', stage)
        if stage == 'rollback':
            code, result = request(path, {'notes': 'mixed-writer-update'}, token, method='PUT')
            assert code == 200
            code, result = request(path, token=token)
            assert code == 200 and result['data']['credentials']['temp_unschedulable_rules'][0]['scope'] == 'account'
    REPORT[-1]['scope_persistence'] = 'PASS'
    print(stage + ': legacy rules and optional scope persistence PASS', flush=True)


# This named project contains only this script's synthetic fixtures.
# Replays start from a new database rather than reusing prior fixture orders.
compose('down', '--volumes', '--remove-orphans')
compose('up', '-d', '--wait', '--wait-timeout', '60', 'postgres', 'redis')
start(BASELINE)
status, logged = login('admin@sub2api.local', 'sync_admin_password')
assert status == 200
token = logged['data']['access_token']
# Seed only this disposable Docker test administrator's initialization state.
sql('upgrade', """INSERT INTO settings (key,value,updated_at)
    SELECT 'admin_compliance_acknowledgement:' || id,
    json_build_object('version','v2026.06.10','accepted_at',NOW(),'admin_user_id',id)::text, NOW()
    FROM users WHERE role='admin';""")
status, created = request('/api/v1/keys', {'name': 'upstream-sync-upgrade-fixture', 'quota': 12.34}, token)
assert status == 200
key = created['data']
status, group = request('/api/v1/admin/groups', {'name': '默认分组', 'description': 'original-description', 'platform': 'openai', 'rate_multiplier': 1}, token)
assert status == 200, ('baseline group fixture', status, group)
subscription_id = int(sql('upgrade', "INSERT INTO user_subscriptions (user_id,group_id,starts_at,expires_at,status) VALUES (" + str(logged['data']['user']['id']) + "," + str(group['data']['id']) + ",NOW(),NOW()+INTERVAL '1 day','active') RETURNING id"))
sql('upgrade', """INSERT INTO payment_orders (user_id,amount,pay_amount,expires_at,out_trade_no,payment_type,order_type,status)
    SELECT id,12.34,12.34,NOW()+INTERVAL '1 day','upstream-sync-20261015-legacy','alipay','balance','PENDING'
    FROM users WHERE role='admin' ORDER BY id LIMIT 1;""")
status, account = request('/api/v1/admin/accounts', {
    'name': 'upstream-sync-rule-fixture', 'platform': 'openai', 'type': 'apikey',
    'credentials': {'api_key': 'synthetic-upstream-key', 'temp_unschedulable_enabled': True,
                    'temp_unschedulable_rules': [{'error_code': 503, 'keywords': ['unavailable'], 'duration_minutes': 1}]},
    'concurrency': 1, 'priority': 1, 'group_ids': [group['data']['id']],
}, token)
assert status == 200, ('account fixture', status, account)
state = {'account_id': account['data']['id'], 'user_id': logged['data']['user']['id'], 'key_id': key['id'], 'group_id': group['data']['id'], 'subscription_id': subscription_id, 'name': key['name'], 'snapshot': snapshot()}
token = runtime('baseline', state)
localization('baseline', state, token)
model_metadata('baseline',state,token)
account_scope('baseline', state, token)
for stage, image in [('upgraded', CANDIDATE), ('rollback', BASELINE), ('restored', CANDIDATE)]:
    # Every replacement has a fresh data directory and stale invalid init env.
    # The installed database must be adopted without rewriting its admin.
    start(image, email='invalid-initialization-email', password='123456')
    token = runtime(stage, state)
    localization(stage, state, token)

    # Upgraded account_scope deliberately changes credentials. Snapshot that
    # identity only after the edit; invalidating an earlier snapshot is correct.
    if stage == 'upgraded':
        account_scope(stage, state, token)
        model_metadata(stage,state,token)
    else:
        model_metadata(stage,state,token)
        account_scope(stage, state, token)


(ROOT / "runtime-result.json").write_text(json.dumps(REPORT, indent=2)+"\n")
compose("down", "--volumes", "--remove-orphans")
