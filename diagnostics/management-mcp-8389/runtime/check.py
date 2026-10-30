#!/usr/bin/env python3
"""Isolated same-database compatibility transaction; never targets a deployment."""
import base64
import hashlib
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parent
COMPOSE = ['docker', 'compose', '-p', 'sub2api-mcp8389-transaction', '-f', str(ROOT / 'compose.yaml')]
BASELINE = 'ghcr.io/dr-lin-eng/sub2api-no2api:sha-8119c14'
CANDIDATE = 'sub2api:management-mcp-8389-final'
ORIGIN = 'http://127.0.0.1:18484'
RESOURCE = ORIGIN + '/api/v1/mcp'
ENV = {**os.environ, 'MCP_FIXTURE_DB_PASSWORD': secrets.token_urlsafe(24),
       'MCP_FIXTURE_JWT_SECRET': secrets.token_hex(48), 'MCP_FIXTURE_ENCRYPTION_KEY': secrets.token_hex(32),
       'MCP_FIXTURE_IMAGE': BASELINE, 'MCP_FIXTURE_RESOURCE': ''}
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}),
                                   urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))


def compose(*args):
    result = subprocess.run(COMPOSE + list(args), env=ENV, capture_output=True, text=True)
    if result.returncode:
        raise RuntimeError('compose operation failed: ' + result.stderr[-1500:])
    return result.stdout.strip()


def sql(statement):
    return compose('exec', '-T', 'postgres', 'psql', '-U', 'sub2api', '-d', 'upgrade',
                   '-v', 'ON_ERROR_STOP=1', '-qAtc', statement)


def request(path, payload=None, token=None, method=None, form=False):
    headers = {'Content-Type': 'application/x-www-form-urlencoded' if form else 'application/json',
               'Accept': 'application/json, text/event-stream'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    data = None if payload is None else (urllib.parse.urlencode(payload).encode() if form else json.dumps(payload).encode())
    req = urllib.request.Request(ORIGIN + path, data=data, headers=headers, method=method)
    try:
        response = OPENER.open(req, timeout=30)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        raw = response.read()
        try:
            body = json.loads(raw)
        except ValueError:
            body = raw.decode(errors='replace')
        return response.code, body, response.headers


def login(email):
    code, data, _ = request('/api/v1/auth/credential-key')
    assert code == 200, ('credential key', code)
    encrypted = subprocess.check_output(['docker', 'run', '--rm', '-i', '-v', str(ROOT) + ':/fixture:ro',
        'node:24-alpine', 'node', '/fixture/credential_envelope_fixture.mjs'],
        input=json.dumps({'server': data['data'], 'email': email, 'password': 'Local-fixture-only-20261030!'}).encode())
    code, body, _ = request('/api/v1/auth/login', {'credential_envelope': json.loads(encrypted)})
    assert code == 200, ('encrypted login', code)
    return body['data']['access_token'], body['data']['user']['id']


def start(image, enabled):
    ENV['MCP_FIXTURE_IMAGE'], ENV['MCP_FIXTURE_RESOURCE'] = image, RESOURCE if enabled else ''
    compose('up', '-d', '--no-deps', '--force-recreate', 'app')
    for _ in range(100):
        try:
            if request('/ready')[0] == 200:
                return
        except OSError:
            pass
        time.sleep(.5)
    raise AssertionError('application readiness failed')


def issue(browser, read_only=True):
    callback = 'http://127.0.0.1:18485/callback'
    code, client, _ = request('/api/v1/mcp/oauth/register', {'client_name': 'Compatibility fixture',
        'redirect_uris': [callback], 'token_endpoint_auth_method': 'none'})
    assert code == 201, ('registration', code)
    verifier = secrets.token_urlsafe(48)
    challenge = base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).decode().rstrip('=')
    auth = {'response_type': 'code', 'client_id': client['client_id'], 'redirect_uri': callback,
            'scope': 'mcp', 'resource': RESOURCE, 'code_challenge': challenge,
            'code_challenge_method': 'S256', 'state': 'fixture-state'}
    code, preview, _ = request('/api/v1/user/mcp/authorize?' + urllib.parse.urlencode(auth), token=browser)
    assert code == 200 and preview['data']['client_id'] == client['client_id'], ('preview', code)
    code, decision, _ = request('/api/v1/user/mcp/authorize',
        {'authorization': auth, 'approved': True, 'scope': 'user', 'read_only': read_only}, browser)
    assert code == 200, ('decision', code)
    oauth_code = urllib.parse.parse_qs(urllib.parse.urlsplit(decision['data']['redirect_url']).query)['code'][0]
    code, tokens, headers = request('/api/v1/mcp/oauth/token', {'grant_type': 'authorization_code',
        'client_id': client['client_id'], 'redirect_uri': callback, 'code': oauth_code,
        'code_verifier': verifier, 'resource': RESOURCE}, form=True)
    assert code == 200 and headers.get('Cache-Control') == 'no-store', ('exchange', code)
    return client, tokens, auth


def tool(token, name, arguments):
    code, result, _ = request('/api/v1/mcp', {'jsonrpc': '2.0', 'id': 1, 'method': 'tools/call',
        'params': {'name': name, 'arguments': arguments}}, token)
    assert code == 200, ('tool transport', code)
    if 'result' not in result:
        return None, result
    text = result['result']['content'][0]['text']
    try:
        payload = json.loads(text)
    except ValueError:
        return None, result
    return payload['http_status'], payload['result']


def snapshot(owner, key):
    value = sql(f"""SELECT json_build_object(
      'user',(SELECT jsonb_build_array(id,email,password_hash,balance) FROM users WHERE id={owner}),
      'key',(SELECT jsonb_build_array(id,name,quota,key) FROM api_keys WHERE id={key}),
      'subscription',(SELECT jsonb_agg(jsonb_build_array(user_id,group_id,expires_at,status)) FROM user_subscriptions))""")
    return hashlib.sha256(value.encode()).hexdigest()


report = []
try:
    compose('up', '-d', 'postgres', 'redis')
    start(BASELINE, False)
    admin, admin_id = login('admin@mcp-fixture.invalid')
    owner = int(sql(f"INSERT INTO users(email,password_hash,role,status,balance,concurrency) SELECT 'owner@mcp-fixture.invalid',password_hash,'user','active',123.45,3 FROM users WHERE id={admin_id} RETURNING id"))
    group = int(sql("INSERT INTO groups(name,platform,status,rate_multiplier,subscription_type) VALUES ('mcp-compat','openai','active',1,'standard') RETURNING id"))
    sub_group = int(sql("INSERT INTO groups(name,platform,status,rate_multiplier,subscription_type) VALUES ('mcp-subscription','openai','active',1,'subscription') RETURNING id"))
    sql(f"INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status,assigned_at) VALUES ({owner},{sub_group},NOW(),NOW()+INTERVAL '7 days','active',NOW())")
    browser, _ = login('owner@mcp-fixture.invalid')
    code, key, _ = request('/api/v1/keys', {'name': 'existing-key', 'group_id': group, 'quota': 12.34}, browser)
    assert code == 200, ('persistent key creation', code)
    key_id = key['data']['id']
    before = snapshot(owner, key_id)
    count = int(sql('SELECT count(*) FROM schema_migrations'))
    report.append({'stage': 'BASELINE', 'migrations': count, 'snapshot': before})

    start(CANDIDATE, True)
    browser, _ = login('owner@mcp-fixture.invalid')
    code, metadata, _ = request('/.well-known/oauth-authorization-server/api/v1/mcp')
    assert code == 200 and metadata['issuer'] == RESOURCE
    assert request('/.well-known/oauth-authorization-server')[0] == 404
    assert request('/api/v1/settings/public')[1]['data']['mcp_enabled'] is True
    client, tokens, auth = issue(browser)
    code, result = tool(tokens['access_token'], 'user_list_api_keys', {'query': {'page': 1, 'page_size': 10}})
    assert code == 200 and key['data']['key'] not in json.dumps(result)
    denied, _ = tool(tokens['access_token'], 'user_create_api_key', {'idempotency_key': 'denied', 'body': {'name': 'denied'}})
    assert denied is None
    _, write_tokens, _ = issue(browser, False)
    # Existing key ownership still applies to read-write tools.
    wrong_owner, _ = tool(write_tokens['access_token'], 'user_update_api_key', {'id': key_id + 1000000, 'body': {'name': 'denied'}})
    assert wrong_owner == 404
    assert snapshot(owner, key_id) == before
    assert int(sql('SELECT count(*) FROM schema_migrations')) == count + 1
    report.append({'stage': 'MODIFIED', 'migrations': count + 1, 'snapshot': before,
                   'read_only_write_denied': True, 'existing_key_redacted': True, 'other_owner_denied': True})

    ready = ROOT / 'ui_ready.json'
    resume = ROOT / 'ui_continue'
    resume.unlink(missing_ok=True)
    ready.write_text(json.dumps({'connections': ORIGIN + '/settings/mcp',
                                'consent': ORIGIN + '/oauth/mcp/authorize?' + urllib.parse.urlencode(auth)}))
    if os.environ.get('MCP_FIXTURE_UI') == 'true':
        print('UI fixture ready; waiting for local ui_continue marker', flush=True)
        for _ in range(1200):
            if resume.exists():
                break
            time.sleep(.5)

    start(BASELINE, False)
    browser, _ = login('owner@mcp-fixture.invalid')
    assert snapshot(owner, key_id) == before
    sql(f"UPDATE users SET status='disabled' WHERE id={owner}")
    sql(f"UPDATE users SET status='active' WHERE id={owner}")
    assert int(sql(f'SELECT count(*) FROM mcp_oauth_grants WHERE user_id={owner} AND revoked_at IS NULL')) == 0
    report.append({'stage': 'ROLLBACK', 'migrations': count + 1, 'snapshot': before,
                   'old_binary_user_writes_revoke_grants': True})

    start(CANDIDATE, True)
    assert request('/api/v1/mcp', {'jsonrpc': '2.0', 'id': 1, 'method': 'tools/list'}, tokens['access_token'])[0] == 401
    browser, _ = login('owner@mcp-fixture.invalid')
    _, fresh, _ = issue(browser)
    code, _ = tool(fresh['access_token'], 'user_get_api_key', {'id': key_id})
    assert code == 200 and snapshot(owner, key_id) == before
    report.append({'stage': 'RESTORED', 'migrations': count + 1, 'snapshot': before,
                   'old_grant_still_invalid': True, 'new_grant_works': True})
    (ROOT / 'result.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))
finally:
    compose('down', '--volumes', '--remove-orphans')
    (ROOT / 'ui_ready.json').unlink(missing_ok=True)
    (ROOT / 'ui_continue').unlink(missing_ok=True)
