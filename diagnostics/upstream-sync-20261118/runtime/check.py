#!/usr/bin/env python3
"""Disposable Docker database fixture for baseline, upgrade, rollback and restore."""
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
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'compose.yaml'), '-p', 'sub2api-sync-20261118']
BASELINE = 'sub2api:sync-baseline-20261118'
CANDIDATE = 'sub2api:sync-candidate-20261118'
ENV = {**os.environ, 'SYNC_DB_PASSWORD': secrets.token_urlsafe(24), 'SYNC_ADMIN_EMAIL': 'admin@fixture.invalid',
       'SYNC_ADMIN_PASSWORD': 'FixtureSync20261118!', 'SYNC_JWT_SECRET': secrets.token_urlsafe(48),
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
    req = urllib.request.Request('http://127.0.0.1:18578' + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
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

def usage_row(kid):
    for _ in range(60):
        rows=json.loads(sql(f"SELECT COALESCE(json_agg(json_build_object('input',input_tokens,'output',output_tokens,'creation',cache_creation_tokens,'5m',cache_creation_5m_tokens,'1h',cache_creation_1h_tokens,'count',image_count,'total',total_cost,'actual',actual_cost,'mode',billing_mode)),'[]'::json) FROM usage_logs WHERE api_key_id={kid}"))
        if rows:return rows
        time.sleep(.1)
    raise AssertionError('usage not persisted')

def create_fixture(name, platform, model, pricing):
    card=json.dumps(pricing)
    gid=int(sql(f"INSERT INTO groups(name,platform,status,rate_multiplier,model_pricing,allow_messages_dispatch) VALUES ('{name}','{platform}','active',1,'{card}',true) RETURNING id"))
    credentials=json.dumps({'api_key':'fixture','base_url':'http://127.0.0.1:8000','api_protocol':'chat_completions','account_mode':'payg'})
    extra=json.dumps({'openai_responses_supported':False})
    aid=int(sql(f"INSERT INTO accounts(name,platform,type,credentials,extra,status,schedulable,concurrency,egress_mode) VALUES ('{name}','{platform}','apikey','{credentials}','{extra}','active',true,3,'direct') RETURNING id"))
    sql(f'INSERT INTO account_groups(account_id,group_id) VALUES ({aid},{gid})')
    if platform=='openai':sql(f'UPDATE groups SET allow_image_generation=true WHERE id={gid}')
    return gid,aid


def check_stage(stage, token, fixed, explicit_gid):
    cases = []
    gid, aid = create_fixture(stage + '-fallback', 'openai', '', [])
    models = [('Qwen/Qwen3-Embedding-8B', 0.00004),
              ('Qwen/Qwen3-Embedding-4B', 0.00002),
              ('Qwen/Qwen3-Embedding-0.6B', 0.00001),
              ('qwen3-embedding-0-6b', 0.00001),
              ('qwen3-embedding-8billion', 0)]
    for index, (model, price) in enumerate(models):
        kid, key = create_key(token, gid, stage + '-embedding-' + str(index))
        code, body = request('/v1/embeddings', {'model': model, 'input': ['hello', 'world']}, key)
        assert code == 200 and body['data'][0]['embedding'] == [0.1, 0.2], (stage, model, code, body)
        rows = usage_row(kid)
        expected = price
        assert len(rows) == 1 and rows[0]['input'] == 1000 and rows[0]['output'] == 0, (stage, model, rows)
        assert abs(rows[0]['total'] - expected) < 1e-10, (stage, model, expected, rows)
        cases.append({'case': 'embedding-fallback', 'model': model, 'expected': expected, 'usage': rows})

    kid, key = create_key(token, explicit_gid, stage + '-explicit')
    code, _ = request('/v1/embeddings', {'model': 'Qwen/Qwen3-Embedding-8B', 'input': 'hello'}, key)
    assert code == 200, (stage, 'explicit pricing', code)
    rows = usage_row(kid)
    assert len(rows) == 1 and abs(rows[0]['total'] - 0.00008) < 1e-10, (stage, 'explicit price', rows)
    cases.append({'case': 'explicit-price-through-upgrade', 'usage': rows})

    # Configure the Responses account before its first scheduler snapshot.
    ultra_gid, ultra_aid = create_fixture(stage + '-ultrafast', 'openai', '', [])
    sql(f"UPDATE accounts SET extra='{{\"openai_responses_supported\":true}}' WHERE id={ultra_aid}")
    # Preserve the existing JSON/SSE 6x policy on both Astra and Sol.
    for model, standard in [('gpt-6-astra', 0.015), ('gpt-6.1-sol', 0.003)]:
        for stream in (False, True):
            kid, key = create_key(token, ultra_gid, stage + '-' + model + '-' + str(stream))
            code, body = request('/v1/responses', {'model': model, 'input': 'hello',
                                                'service_tier': 'ultrafast', 'stream': stream}, key)
            assert code == 200, (stage, 'ultrafast', model, stream, code, body)
            rows = usage_row(kid)
            assert len(rows) == 1 and rows[0]['input'] == 1000 and rows[0]['output'] == 100, (stage, model, rows)
            assert abs(rows[0]['total'] - 6 * standard) < 1e-9, (stage, model, stream, rows)
            cases.append({'case': 'existing-ultrafast-6x', 'model': model, 'stream': stream, 'usage': rows})
    cache_prices = [{'platform': 'anthropic', 'models': ['cache-fixture'],
                     'billing_mode': 'token', 'input_price': 0.001, 'output_price': 0.002,
                     'cache_write_price': 0.003, 'cache_read_price': 0.0001}]
    cache_gid, _ = create_fixture(stage + '-cache', 'anthropic', '', cache_prices)
    for stream in (False, True):
        kid, key = create_key(token, cache_gid, stage + '-cache-' + str(stream))
        code, body = request('/v1/messages', {'model': 'cache-fixture', 'max_tokens': 32,
                             'messages': [{'role': 'user', 'content': 'hello'}], 'stream': stream}, key)
        assert code == 200, (stage, 'cache status', code, body)
        if stream:
            assert 'message_stop' in body and 'ok' in body
        else:
            assert body.get('content', [{}])[0].get('text') == 'ok', (stage, stream, code, body)
        usage = usage_row(kid)
        creation = (50 if stream else 30) if fixed else 0
        assert len(usage) == 1 and usage[0]['creation'] == creation, (stage, 'cache tokens', stream, usage)
        assert usage[0]['input'] == 10 and usage[0]['output'] == 7, (stage, stream, usage)
        expected = 10 * 0.001 + 7 * 0.002 + creation * 0.003 + 5 * 0.0001
        assert abs(usage[0]['total'] - expected) < 1e-9, (stage, 'cache cost', stream, expected, usage)
        cases.append({'case': 'cache-ttl-snapshots', 'stream': stream, 'usage': usage})
    chat_gid, _ = create_fixture(stage + '-raw-chat', 'openai', '', [])
    for stream in (False, True):
        kid, key = create_key(token, chat_gid, stage + '-chat-' + str(stream))
        code, body = request('/v1/chat/completions', {'model': 'gpt-6.2-sol',
                             'messages': [{'role': 'user', 'content': 'hello'}], 'stream': stream,
                             'stream_options': {'include_usage': True}}, key)
        assert code == 200, (stage, 'raw chat status', code, body)
        if stream:
            assert body.count('data: [DONE]') == 1 and 'ok' in body, (stage, body)
        else:
            assert body['choices'][0]['message']['content'] == 'ok'
        usage = usage_row(kid)
        assert len(usage) == 1 and usage[0]['input'] == 1000 and usage[0]['output'] == 100, (stage, usage)
        cases.append({'case': 'raw-chat-regression', 'stream': stream, 'usage': usage})
    return cases


report = []
try:
    compose('up', '-d', 'postgres', 'redis')
    start(BASELINE)
    token, uid = login()
    ack = json.dumps({'version': 'v2026.06.10', 'admin_user_id': uid, 'accepted_at': '2026-11-18T00:00:00Z'})
    sql(f"INSERT INTO settings(key,value) VALUES ('admin_compliance_acknowledgement:{uid}','{ack}')")
    sql(f'UPDATE users SET balance=1000 WHERE id={uid}')
    gid, _ = create_fixture('persisted', 'openai', '', [])
    kid, _ = create_key(token, gid, 'persisted-key')
    sql(f"INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status) VALUES ({uid},{gid},NOW(),NOW()+INTERVAL '1 day','active')")
    pristine = snapshot(uid, kid)
    explicit = [{'platform': 'openai', 'models': ['Qwen/Qwen3-Embedding-8B'],
                 'billing_mode': 'token', 'input_price': 0.00000008, 'output_price': 0}]
    explicit_gid, _ = create_fixture('persisted-explicit-price', 'openai', '', explicit)
    price_snapshot = sql(f'SELECT model_pricing FROM groups WHERE id={explicit_gid}')
    migration_digest = sql("SELECT md5(string_agg(filename||checksum,',' ORDER BY filename)) FROM schema_migrations")
    migrations = int(sql('SELECT COUNT(*) FROM schema_migrations'))
    historical = None
    stages = [('BASELINE', BASELINE, False), ('CANDIDATE', CANDIDATE, True),
              ('ROLLBACK', BASELINE, False), ('RESTORED', CANDIDATE, True)]
    for stage, image, fixed in stages:
        if stage != 'BASELINE':
            start(image, True)
        token, uid = login()
        if historical:
            upper, digest = historical
            assert sql(f"SELECT md5(COALESCE(string_agg(row_to_json(u)::text,',' ORDER BY id),'')) FROM usage_logs u WHERE id<={upper}") == digest
        cases = check_stage(stage, token, fixed, explicit_gid)
        assert snapshot(uid, kid) == pristine, (stage, 'persistent identity/key/subscription')
        assert sql(f'SELECT model_pricing FROM groups WHERE id={explicit_gid}') == price_snapshot
        assert sql("SELECT md5(string_agg(filename||checksum,',' ORDER BY filename)) FROM schema_migrations") == migration_digest
        assert sql(f'SELECT ABS(balance+(SELECT COALESCE(SUM(actual_cost),0) FROM usage_logs WHERE user_id={uid})-1000)<0.00000001 FROM users WHERE id={uid}') == 't', (stage, 'balance equation')
        upper = int(sql('SELECT COALESCE(MAX(id),0) FROM usage_logs'))
        historical = (upper, sql(f"SELECT md5(COALESCE(string_agg(row_to_json(u)::text,',' ORDER BY id),'')) FROM usage_logs u WHERE id<={upper}"))
        item = {'stage': stage, 'result': 'PASS', 'cases': cases, 'migrations': migrations,
                'migration_checksums': 'PASS', 'persistent_identity_key_subscription': 'PASS',
                'historical_usage_unchanged': 'PASS', 'balance_equation': 'PASS'}
        report.append(item)
        print(json.dumps(item), flush=True)
    (ROOT / 'result.json').write_text(json.dumps(report, indent=2) + '\n')
finally:
    compose('down', '--volumes', '--remove-orphans')
