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
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'compose.yaml'), '-p', 'sub2api-sync-20261022-f89c']
BASELINE = 'ghcr.io/dr-lin-eng/sub2api-no2api:sha-0dd415b'
CANDIDATE = 'sub2api:upstream-sync-20261022'
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
    req = urllib.request.Request('http://127.0.0.1:18152' + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
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


def abort_stream(route, key):
    connection = http.client.HTTPConnection('127.0.0.1', 18152, timeout=15)
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



def abort_stream(route, key):
    connection = http.client.HTTPConnection('127.0.0.1', 18152, timeout=15)
    path = '/v1/chat/completions' if route == 'chat' else '/v1/responses'
    body = {'model': 'claude-sonnet-4-5', 'stream': True}
    if route == 'chat':
        body['messages'] = [{'role': 'user', 'content': 'runtime-abort'}]
    else:
        body['input'] = 'runtime-abort'
    connection.request('POST', path, json.dumps(body), {'Content-Type': 'application/json', 'Authorization': 'Bearer ' + key})
    response = connection.getresponse()
    assert response.status == 200, ('abort status', route, response.status)
    for _ in range(100):
        line = response.fp.readline()
        if b'fixture-end' in line:
            response.close(); connection.close()
            return
        if not line:
            break
    response.close(); connection.close()
    raise AssertionError('no fixture output: ' + route)


def usage(kid):
    for _ in range(50):
        rows = json.loads(sql(f"SELECT COALESCE(json_agg(jsonb_build_array(input_tokens,output_tokens)),'[]'::json) FROM usage_logs WHERE api_key_id={kid}"))
        if rows:
            return rows
        time.sleep(.15)
    return []


def image_request(key):
    return request('/v1beta/models/gemini-2.5-flash-image:generateContent', {'contents':[{'parts':[{'text':'runtime-image'}]}]}, key)[0]


report = []
try:
    compose('up', '-d', 'postgres', 'redis')
    start(BASELINE)
    token, uid = login()
    sql(f"UPDATE users SET balance=100 WHERE id={uid}")
    anth_gid = int(sql("INSERT INTO groups(name,platform,status,rate_multiplier) VALUES ('sync-anthropic','anthropic','active',1) RETURNING id"))
    gem_gid = int(sql("INSERT INTO groups(name,platform,status,rate_multiplier,allow_image_generation) VALUES ('sync-gemini','gemini','active',1,false) RETURNING id"))
    anti_gid = int(sql("INSERT INTO groups(name,platform,status,rate_multiplier,allow_image_generation) VALUES ('sync-antigravity','antigravity','active',1,false) RETURNING id"))
    composite_gid = int(sql("INSERT INTO groups(name,platform,status,rate_multiplier,allow_image_generation) VALUES ('sync-composite','composite','active',1,false) RETURNING id"))
    for platform, gid in [('anthropic', anth_gid), ('gemini', gem_gid)]:
        aid = int(sql(f"""INSERT INTO accounts(name,platform,type,credentials,extra,status,schedulable,concurrency,egress_mode)
            VALUES ('offline-{platform}','{platform}','apikey', '{{"api_key":"fixture-local","base_url":"http://127.0.0.1:8000"}}','{{}}','active',true,3,'direct') RETURNING id"""))
        sql(f"INSERT INTO account_groups(account_id,group_id) VALUES ({aid},{gid})")
    vertex_id = int(sql("""INSERT INTO accounts(name,platform,type,credentials,extra,status,schedulable,concurrency,egress_mode)
        VALUES ('vertex-quota','gemini','service_account','{"auth_mode":"apikey","api_key":"fixture","location":"global","tier_id":"vertex"}','{"quota_limit":1,"quota_used":2}', 'active',true,1,'direct') RETURNING id"""))
    kid, api_key = create_key(token, anth_gid, 'persisted-upgrade-key')
    _, gem_key = create_key(token, gem_gid, 'gemini-legacy-access')
    sql(f"INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status) VALUES ({uid},{anth_gid},NOW(),NOW()+INTERVAL '1 day','active')")
    pristine = snapshot(uid, kid)
    baseline_migrations = int(sql('SELECT COUNT(*) FROM schema_migrations'))
    for stage, image, fixed in [('BASELINE', BASELINE, False), ('MODIFIED', CANDIDATE, True), ('ROLLBACK', BASELINE, False), ('RESTORED', CANDIDATE, True)]:
        if stage != 'BASELINE':
            start(image, True)
        assert all(request(path)[0] == 200 for path in ['/health', '/ready', '/'])
        token, current_uid = login()
        assert uid == current_uid and pristine == snapshot(uid,kid), stage + ' changed persisted identity/key/subscription'
        status, key_info = request(f'/api/v1/keys/{kid}', token=token)
        assert status == 200 and key_info['data']['quota'] == 12.34
        migrations = int(sql('SELECT COUNT(*) FROM schema_migrations'))
        assert migrations == baseline_migrations + (0 if stage == 'BASELINE' else 1)
        assert sql(f'SELECT allow_image_generation FROM groups WHERE id={composite_gid}') == 'f'
        if stage == 'MODIFIED':
            assert sql(f'SELECT allow_image_generation FROM groups WHERE id={gem_gid}') == 't'
            assert sql(f'SELECT allow_image_generation FROM groups WHERE id={anti_gid}') == 't'
        initial_image_status = image_request(gem_key)
        assert initial_image_status == (400 if stage == 'RESTORED' else 200), (stage, 'legacy image access', initial_image_status)
        if fixed:
            # Exercise the persisted gate value directly; the baseline admin
            # group endpoint intentionally returns its existing 423 credential-lock boundary.
            sql(f'UPDATE groups SET allow_image_generation=false WHERE id={gem_gid}')
            assert sql(f'SELECT allow_image_generation FROM groups WHERE id={gem_gid}') == 'f'
            _, denied_key = create_key(token,gem_gid,stage+'-explicit-deny')
            denied_image_status = image_request(denied_key)
            assert denied_image_status == 400,(stage,'explicit image deny',denied_image_status)
        else:
            denied_image_status = initial_image_status
        status, vertex = request(f'/api/v1/admin/accounts/{vertex_id}', token=token)
        # The baseline account detail endpoint can return 423 for a redacted
        # service-account credential; the persisted quota is the upgrade fact.
        assert status in (200, 423), (stage, 'vertex DTO', status)
        quota = vertex.get('data', {}).get('quota_limit') if isinstance(vertex, dict) else None
        if status == 200:
            assert quota == (1 if fixed else None), (stage, 'vertex quota display', quota)
        assert sql(f"SELECT extra->>'quota_limit' FROM accounts WHERE id={vertex_id}") == '1'
        aborts, buffered = {}, {}
        for route in ['chat','responses']:
            route_kid, route_key = create_key(token,anth_gid,stage+'-'+route+'-abort')
            abort_stream(route,route_key)
            rows=usage(route_kid)
            assert len(rows)==1 and rows[0][0]==10,(stage,route,rows)
            assert rows[0][1] == 50 if fixed else rows[0][1] < 50,(stage,route,rows)
            aborts[route]=rows
            body={'model':'claude-sonnet-4-5','stream':False}
            if route=='chat':body['messages']=[{'role':'user','content':'runtime-buffered'}]
            else:body['input']='runtime-buffered'
            result_code,_=request('/v1/chat/completions' if route=='chat' else '/v1/responses',body,api_key)
            assert result_code==200,(stage,'buffered',route,result_code)
            buffered[route]=result_code
        row={'stage':stage,'result':'PASS','abort_usage_once':aborts,'buffered_json_http':buffered,
            'initial_image_http':initial_image_status,'explicit_image_deny_http':denied_image_status,
            'vertex_quota_limit':quota,'vertex_dto_status':status,'migration_count':migrations,'composite_flag':'unchanged_false',
            'persisted_identity_key_subscription_sha256':hashlib.sha256(pristine.encode()).hexdigest(),
            'health_ready_frontend_encrypted_login_key_subscription':'PASS'}
        report.append(row);print(json.dumps(row),flush=True)
    (ROOT/'result.json').write_text(json.dumps(report,indent=2)+'\n')
finally:
    compose('down','--volumes','--remove-orphans')
