#!/usr/bin/env python3
"""Same-database upgrade/rollback with ephemeral credentials kept in memory."""
import hashlib
import base64
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
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'compose.yaml'), '-p', 'sub2api-image-standards-20261110']
BASELINE = 'sub2api:image-standards-baseline-20261110'
CANDIDATE = 'sub2api:image-api-siwc-svg-20261110'
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
    req = urllib.request.Request('http://127.0.0.1:18563' + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
    try:
        response = OPENER.open(req, timeout=40)
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


def create_fixture(name, kind, credentials, extra=None, count_billing=False):
    prices = json.dumps([{'platform': 'openai', 'models': ['gpt-image-2', 'gpt-image-2.6-flare', 'gpt-image-2.6-sunburst'], 'billing_mode': 'image', 'per_request_price': .1}])
    gid = int(sql(f"INSERT INTO groups(name,platform,status,rate_multiplier,model_pricing,allow_image_generation,image_count_billing) VALUES ('{name}','openai','active',1,'{prices}',true,{str(count_billing).lower()}) RETURNING id"))
    aid = int(sql(f"INSERT INTO accounts(name,platform,type,credentials,extra,status,schedulable,concurrency,egress_mode) VALUES ('{name}','openai','{kind}','{json.dumps(credentials)}','{json.dumps(extra or {})}','active',true,3,'direct') RETURNING id"))
    sql(f'INSERT INTO account_groups(account_id,group_id) VALUES ({aid},{gid})')
    return gid, aid


def create_key(token, gid, name):
    status, result = request('/api/v1/keys', {'name': name, 'group_id': gid}, token)
    assert status == 200, ('create key', status)
    return result['data']['id'], result['data']['key']


def provider():
    script = 'import urllib.request;print(urllib.request.urlopen("http://127.0.0.1:8000/fixture/last").read().decode())'
    return json.loads(compose('exec', '-T', 'provider', 'python', '-c', script))


def settled(kid):
    for _ in range(100):
        rows = json.loads(sql(f"SELECT COALESCE(json_agg(json_build_object('images',image_count,'cost',actual_cost)),'[]'::json) FROM usage_logs WHERE api_key_id={kid}"))
        if rows:
            assert len(rows) == 1, 'duplicate settlement'
            return rows[0]
        time.sleep(.1)
    raise AssertionError('missing reliable settlement')


def check_stage(stage, token, fixed):
    cases = []
    gid, oauth = create_fixture('oauth-' + stage, 'oauth', {'access_token': 'fixture-oauth'},
        {'custom_base_url_enabled': True, 'custom_base_url': 'http://127.0.0.1:8000/backend-api/codex'})
    _, leaf_key = create_key(token, gid, 'leaf-' + stage)
    outer_gid, outer = create_fixture('chain-' + stage, 'apikey', {'api_key': leaf_key, 'base_url': 'http://127.0.0.1:8080/v1'})
    outer_kid, outer_key = create_key(token, outer_gid, 'chain-' + stage)
    payload = {'model': 'gpt-image-2.6-sunburst', 'prompt': 'draw', 'size': '1024x1024', 'quality': 'high', 'output_format': 'png'}
    code, result = request('/v1/images/generations', payload, outer_key)
    assert code == (200 if fixed else 400), (stage, 'double gateway', code, str(result)[:300])
    if fixed:
        sent = provider()
        assert sent['path'] == '/backend-api/codex/images/generations'
        assert sent['body']['size'] == payload['size'] and sent['body']['quality'] == 'high'
        settled(outer_kid)
    else:
        assert 'requires legacy image generation' in result['error']['message']
    cases.append({'case': 'double-gateway-standard-request', 'status': code})
    code, result = request(f'/api/v1/admin/accounts/{outer}/test', {'model_id': payload['model'], 'prompt': 'draw'}, token)
    assert ('"success":true' in result) == fixed, (stage, 'API-key account probe', str(result)[:300])
    cases.append({'case': 'API-key-probe-through-OAuth-gateway', 'success': fixed})

    siwc_gid, siwc = create_fixture('siwc-' + stage, 'oauth', {'auth_mode': 'siwc', 'access_token': 'fixture-siwc'},
        {'custom_base_url_enabled': True, 'custom_base_url': 'http://127.0.0.1:8000/v1'})
    _, siwc_key = create_key(token, siwc_gid, 'siwc-' + stage)
    code, result = request('/v1/images/generations', {'model': 'gpt-image-2', 'prompt': 'draw', 'size': '1024x1024'}, siwc_key)
    assert code == 200, (stage, 'SIWC generation', code, str(result)[:200])
    sent = provider()
    assert sent['path'] == ('/v1/images/generations' if fixed else '/v1/responses'), (stage, 'SIWC endpoint', sent['path'])
    if fixed:
        assert 'messages' not in sent['body'] and 'tools' not in sent['body']
        assert not any(k == 'originator' or k == 'chatgpt-account-id' or k.startswith('x-codex-') for k in sent['headers'])
    cases.append({'case': 'SIWC-protocol', 'upstream_path': sent['path']})

    svg = {'model_id': 'gpt-image-2.6-flare', 'prompt': 'draw', 'output_format': 'svg'}
    code, result = request(f'/api/v1/admin/accounts/{oauth}/test', svg, token)
    assert ('"success":true' in result) == fixed, (stage, 'SVG probe', str(result)[:250])
    if fixed:
        assert 'fallback' not in result.lower() and 'legacy' not in result.lower()
        events = [json.loads(line[6:]) for line in result.splitlines() if line.startswith('data: {')]
        image = next(event for event in events if event['type'] == 'image')
        decoded = base64.b64decode(image['image_url'].split(',', 1)[1]).decode('utf-8')
        assert '向量 🚲' in decoded and image['mime_type'] == 'image/svg+xml'
    else:
        assert 'invalid UTF-8' in result
    cases.append({'case': 'OAuth-SVG-UTF16-probe', 'success': fixed})

    direct_gid, direct = create_fixture('api-' + stage, 'apikey', {'api_key': 'fixture-api', 'base_url': 'http://127.0.0.1:8000/v1'}, count_billing=True)
    _, api_key = create_key(token, direct_gid, 'api-' + stage)
    for stream in (False, True):
        code, result = request('/v1/images/generations', {**payload, 'stream': stream}, api_key)
        assert code == 200
        sent = provider()
        assert sent['path'] == '/v1/images/generations' and sent['body']['size'] == '1024x1024'
        assert sent['body']['stream'] == stream
        cases.append({'case': 'API-key-standard-images', 'stream': stream, 'status': code})
    _, svg_key = create_key(token, direct_gid, 'api-svg-' + stage)
    code, result = request('/v1/images/generations', {'model': 'gpt-image-2.6-flare', 'prompt': 'draw', 'output_format': 'svg'}, svg_key)
    assert code == 200
    data = base64.b64decode(result['data'][0]['b64_json'])
    if fixed:
        assert '向量 🚲' in data.decode('utf-8') and result['svg_usage']['input_tokens'] == 50
    else:
        assert data.startswith((b'\xff\xfe', b'\xfe\xff'))
    cases.append({'case': 'API-key-SVG-public-encoding', 'canonical_utf8': fixed})
    gif_kid, gif_key = create_key(token, direct_gid, 'api-gif-' + stage)
    code, result = request('/v1/images/generations', {'model': 'gpt-image-2.6-flare', 'prompt': 'draw', 'output_format': 'gif', 'frame_count': 4, 'frame_duration_ms': 100}, gif_key)
    assert code == 200 and settled(gif_kid)['images'] == 4
    cases.append({'case': 'GIF-actual-frame-billing', 'billable_frames': 4, 'settled_once': True})
    return cases


report = []
try:
    compose('up', '-d', 'postgres', 'redis')
    start(BASELINE)
    token, uid = login()
    ack = json.dumps({'version': 'v2026.06.10', 'admin_user_id': uid, 'accepted_at': '2026-11-10T00:00:00Z'})
    sql(f"INSERT INTO settings(key,value) VALUES ('admin_compliance_acknowledgement:{uid}','{ack}')")
    sql(f'UPDATE users SET balance=1000 WHERE id={uid}')
    migrations = sql('SELECT COUNT(*) FROM schema_migrations')
    historical = None
    for stage, image, fixed in [('BASELINE', BASELINE, False), ('MODIFIED', CANDIDATE, True), ('ROLLBACK', BASELINE, False), ('RESTORED', CANDIDATE, True)]:
        if stage != 'BASELINE':
            start(image, True)
        token, _ = login()
        if historical:
            upper, digest = historical
            assert sql(f"SELECT md5(COALESCE(string_agg(row_to_json(u)::text,',' ORDER BY id),'')) FROM usage_logs u WHERE id<={upper}") == digest
        cases = check_stage(stage, token, fixed)
        assert sql('SELECT COUNT(*) FROM schema_migrations') == migrations
        upper = int(sql('SELECT COALESCE(MAX(id),0) FROM usage_logs'))
        historical = (upper, sql(f"SELECT md5(COALESCE(string_agg(row_to_json(u)::text,',' ORDER BY id),'')) FROM usage_logs u WHERE id<={upper}"))
        item = {'stage': stage, 'result': 'PASS', 'cases': cases, 'migrations_unchanged': True, 'historical_usage_unchanged': True}
        report.append(item)
        print(json.dumps(item), flush=True)
    (ROOT / 'result.json').write_text(json.dumps(report, indent=2) + '\n')
finally:
    compose('down', '--volumes', '--remove-orphans')
