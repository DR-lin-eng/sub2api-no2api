#!/usr/bin/env python3
"""Same-database upgrade/rollback with ephemeral credentials kept in memory."""
import hashlib
import http.client
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
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'compose.yaml'), '-p', 'sub2api-sync-20261019-1f7d']
BASELINE = 'ghcr.io/dr-lin-eng/sub2api-no2api:sha-b75c196'
CANDIDATE = 'sub2api:upstream-sync-20261019'
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


def request(path, body=None, token=None):
    headers = {'Content-Type': 'application/json'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    req = urllib.request.Request('http://127.0.0.1:18139' + path, data=None if body is None else json.dumps(body).encode(), headers=headers)
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
       'subscriptions',(SELECT jsonb_agg(jsonb_build_array(user_id,group_id,expires_at,status)) FROM user_subscriptions),
       'migrations',(SELECT COUNT(*) FROM schema_migrations))""")


def create_key(token, group_id, name):
    status, key = request('/api/v1/keys', {'name': name, 'group_id': group_id, 'quota': 12.34}, token)
    assert status == 200, ('create key', status)
    return key['data']['id'], key['data']['key']


def abort_stream(route, key):
    connection = http.client.HTTPConnection('127.0.0.1', 18139, timeout=15)
    if route == 'native':
        path = '/v1beta/models/gemini-2.5-flash:streamGenerateContent?alt=sse'
        body = {'contents': [{'parts': [{'text': 'fixture'}]}]}
    else:
        path = '/v1/messages' if route == 'messages' else '/v1/chat/completions'
        body = {'model': 'gemini-2.5-flash', 'max_tokens': 16, 'stream': True, 'messages': [{'role': 'user', 'content': 'fixture'}]}
    connection.request('POST', path, json.dumps(body), {'Content-Type': 'application/json', 'Authorization': 'Bearer ' + key})
    response = connection.getresponse()
    assert response.status == 200, ('stream', route, response.status)
    for _ in range(100):
        line = response.fp.readline()
        if b'fixture-end' in line:
            time.sleep(.15)
            response.close()
            connection.close()
            return
        if not line:
            break
    response.close()
    connection.close()
    raise AssertionError('no fixture output: ' + route)


report = []
try:
    compose('up', '-d', 'postgres', 'redis')
    start(BASELINE)
    token, uid = login()
    gid = int(sql("INSERT INTO groups(name,platform,status,rate_multiplier) VALUES ('sync-gemini-fixture','gemini','active',1) RETURNING id"))
    sql(f"UPDATE users SET balance=100 WHERE id={uid}")
    aid = int(sql('''INSERT INTO accounts(name,platform,type,credentials,extra,status,schedulable,concurrency,egress_mode)
        VALUES ('offline-gemini','gemini','apikey',
        '{"api_key":"fixture-local","base_url":"http://127.0.0.1:8000"}', '{}', 'active',true,3,'direct') RETURNING id'''))
    sql(f"INSERT INTO account_groups(account_id,group_id) VALUES ({aid},{gid})")
    kid, api_key = create_key(token, gid, 'persisted-upgrade-key')
    sql(f"INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status) VALUES ({uid},{gid},NOW(),NOW()+INTERVAL '1 day','active')")
    pristine = snapshot(uid, kid)
    for stage, image, fixed in [('BASELINE', BASELINE, False), ('MODIFIED', CANDIDATE, True), ('ROLLBACK', BASELINE, False), ('RESTORED', CANDIDATE, True)]:
        if stage != 'BASELINE':
            start(image, True)
        assert all(request(path)[0] == 200 for path in ['/health', '/ready', '/'])
        token, current_uid = login()
        assert uid == current_uid and pristine == snapshot(uid, kid), stage + ' changed persisted identity/key/subscription/schema'
        status, key_info = request(f'/api/v1/keys/{kid}', token=token)
        assert status == 200 and key_info['data']['quota'] == 12.34
        status, response = request('/v1/messages', {'model': 'gemini-2.5-flash', 'max_tokens': 16,
            'messages': [{'role': 'user', 'content': 'fixture'}]}, api_key)
        assert status == 200, (stage, 'cache-only response', status)
        observed_input = response['usage']['input_tokens']
        assert observed_input == (0 if fixed else -30), (stage, 'cache-only input', observed_input)
        # This fork's Messages JSON omits the cache-read display field at both
        # revisions. Check the preserved cache count through persisted usage.
        cache_read = None
        for _ in range(24):
            value = sql(f"SELECT cache_read_tokens FROM usage_logs WHERE api_key_id={kid} ORDER BY id DESC LIMIT 1")
            if value:
                cache_read = int(value)
                break
            time.sleep(.25)
        assert cache_read == 30, (stage, 'persisted cache-read tokens', cache_read)
        aborts = {}
        for route in ['native', 'messages', 'chat']:
            route_kid, route_key = create_key(token, gid, stage + '-' + route)
            abort_stream(route, route_key)
            rows = []
            for _ in range(24):
                rows = json.loads(sql(f"SELECT COALESCE(json_agg(json_build_array(input_tokens,output_tokens)),'[]'::json) FROM usage_logs WHERE api_key_id={route_kid}"))
                if rows:
                    break
                time.sleep(.25)
            assert rows == ([[10, 5]] if fixed else []), (stage, route, rows)
            aborts[route] = rows
        row = {'stage': stage, 'result': 'PASS', 'cache_only_input_tokens': observed_input, 'cache_read_tokens': 30,
            'client_abort_usage': aborts, 'migration_count': int(sql('SELECT COUNT(*) FROM schema_migrations')),
            'persisted_invariants_sha256': hashlib.sha256(pristine.encode()).hexdigest(),
            'health_ready_frontend_encrypted_login_key_subscription': 'PASS'}
        report.append(row)
        print(json.dumps(row), flush=True)
    (ROOT / 'result.json').write_text(json.dumps(report, indent=2) + '\n')
finally:
    compose('down', '--volumes', '--remove-orphans')
