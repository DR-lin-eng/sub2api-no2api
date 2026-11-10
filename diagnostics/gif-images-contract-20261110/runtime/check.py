"""Two same-version gateways, separate DB/cache, disposable fixture credentials."""
import base64
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
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'compose.yaml'), '-p', 'sub2api-gif-contract-20261110']
ENV = {**os.environ, 'GIF_IMAGE': 'sub2api:gif-contract-baseline-20261110',
       'GIF_DB_PASSWORD': secrets.token_urlsafe(24), 'GIF_ADMIN_EMAIL': 'admin@fixture.invalid',
       'GIF_ADMIN_PASSWORD': secrets.token_urlsafe(24), 'GIF_JWT_SECRET': secrets.token_urlsafe(48),
       'GIF_ENCRYPTION_KEY': secrets.token_hex(32)}
PORTS = {'pool': 18573, 'downstream': 18574}
OPENERS = {name: urllib.request.build_opener(urllib.request.ProxyHandler({}),
           urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar())) for name in PORTS}


def compose(*args):
    result = subprocess.run(COMPOSE + list(args), env=ENV, capture_output=True, text=True)
    if result.returncode:
        raise RuntimeError('compose ' + str(args) + ': ' + result.stderr)
    return result.stdout.strip()


def sql(database, statement):
    return compose('exec', '-T', 'postgres', 'psql', '-U', 'fixture', '-d', database, '-v', 'ON_ERROR_STOP=1', '-qAtc', statement)


def request(instance, path, body=None, token=None):
    headers = {'Content-Type': 'application/json'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    req = urllib.request.Request(f'http://127.0.0.1:{PORTS[instance]}' + path,
          data=None if body is None else json.dumps(body).encode(), headers=headers)
    try:
        response = OPENERS[instance].open(req, timeout=40)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        raw = response.read().decode()
        try:
            value = json.loads(raw)
        except ValueError:
            value = raw
        return response.code, value


def login(instance):
    code, server = request(instance, '/api/v1/auth/credential-key')
    assert code == 200
    fixture = ROOT.parents[1] / 'image-api-siwc-svg-20261110/runtime'
    envelope = json.loads(subprocess.check_output(['docker', 'run', '--rm', '-i', '-v', str(fixture) + ':/fixture:ro',
        'node:24-alpine', 'node', '/fixture/credential_envelope_fixture.mjs'], input=json.dumps(
        {'server': server['data'], 'email': ENV['GIF_ADMIN_EMAIL'], 'password': ENV['GIF_ADMIN_PASSWORD']}).encode()))
    code, result = request(instance, '/api/v1/auth/login', {'credential_envelope': envelope})
    assert code == 200
    uid = result['data']['user']['id']
    ack = json.dumps({'version': 'v2026.06.10', 'admin_user_id': uid, 'accepted_at': '2026-11-10T00:00:00Z'})
    sql(instance, f"INSERT INTO settings(key,value) VALUES ('admin_compliance_acknowledgement:{uid}','{ack}') ON CONFLICT(key) DO NOTHING")
    sql(instance, f'UPDATE users SET balance=1000 WHERE id={uid}')
    return result['data']['access_token']


def fixture(instance, name, kind, credentials, extra=None):
    prices = json.dumps([{'platform': 'openai', 'models': ['gpt-image-2.6-flare', 'gpt-image-2.6-sunburst'],
                         'billing_mode': 'image', 'per_request_price': .1}])
    gid = int(sql(instance, f"INSERT INTO groups(name,platform,status,rate_multiplier,model_pricing,allow_image_generation,image_count_billing) VALUES ('{name}','openai','active',1,'{prices}',true,true) RETURNING id"))
    aid = int(sql(instance, f"INSERT INTO accounts(name,platform,type,credentials,extra,status,schedulable,concurrency,egress_mode) VALUES ('{name}','openai','{kind}','{json.dumps(credentials)}','{json.dumps(extra or {})}','active',true,3,'direct') RETURNING id"))
    sql(instance, f'INSERT INTO account_groups(account_id,group_id) VALUES ({aid},{gid})')
    return gid, aid


def key(instance, token, gid, name):
    code, result = request(instance, '/api/v1/keys', {'name': name, 'group_id': gid}, token)
    assert code == 200
    return result['data']['id'], result['data']['key']


def settled(instance, kid, minimum=1):
    for _ in range(100):
        rows = json.loads(sql(instance, f"SELECT COALESCE(json_agg(json_build_object('images',image_count,'cost',actual_cost) ORDER BY id),'[]'::json) FROM usage_logs WHERE api_key_id={kid}"))
        if len(rows) >= minimum:
            return rows
        time.sleep(.1)
    raise AssertionError('missing settlement')


def provider_calls():
    return json.loads(compose('exec', '-T', 'provider', 'python', '-c',
        'import urllib.request;print(urllib.request.urlopen("http://127.0.0.1:8000/fixture/calls").read().decode())'))


def check(stage, fixed):
    tokens = {name: login(name) for name in PORTS}
    gid, oauth = fixture('pool', 'oauth-' + stage, 'oauth', {'access_token': 'fixture-oauth'},
        {'custom_base_url_enabled': True, 'custom_base_url': 'http://127.0.0.1:8000/backend-api/codex'})
    pool_kid, pool_key = key('pool', tokens['pool'], gid, stage)
    outer_gid, outer = fixture('downstream', 'chain-' + stage, 'apikey', {'api_key': pool_key, 'base_url': 'http://pool:8080/v1'})
    cases = []
    for model in ['gpt-image-2.6-flare', 'gpt-image-2.6-sunburst']:
        options = {'model_id': model, 'prompt': 'a rotating logo', 'output_format': 'gif', 'frame_count': 9, 'frame_duration_ms': 200}
        _, result = request('pool', f'/api/v1/admin/accounts/{oauth}/test', options, tokens['pool'])
        assert '"success":true' in result and 'data:image/gif;base64,' in result
        cases.append({'case': 'OAuth-button-GIF', 'model': model, 'success': True})
        _, result = request('downstream', f'/api/v1/admin/accounts/{outer}/test', options, tokens['downstream'])
        assert ('"success":true' in result) == fixed, (stage, model, result[:300])
        if fixed:
            assert 'data:image/gif;base64,' in result and 'fallback' not in result and 'legacy' not in result
        else:
            assert 'fixture: native GIF unavailable' in result
        cases.append({'case': 'API-key-button-through-pool', 'model': model, 'success': fixed})
    for index, (frames, delay) in enumerate([(9, 200), (4, 200)]):
        kid, client_key = key('downstream', tokens['downstream'], outer_gid, stage + str(frames))
        body = {'model': 'gpt-image-2.6-flare', 'prompt': 'a rotating logo', 'output_format': 'gif',
                'size': 'auto', 'frame_count': frames, 'frame_duration_ms': delay}
        code, result = request('downstream', '/v1/images/generations', body, client_key)
        assert code == (200 if fixed else 400), (stage, code)
        if fixed:
            encoded = result['data'][0]['b64_json']
            assert base64.b64decode(encoded).startswith(b'GIF89a')
            rows = settled('downstream', kid)
            assert len(rows) == 1 and rows[0]['images'] == frames
            assert settled('pool', pool_kid, 3 + index)[-1]['images'] == frames
        cases.append({'case': 'standard-API-two-gateways', 'status': code, 'frames': frames, 'both_hops_settled': fixed})
    calls = provider_calls()
    image_calls = [call for call in calls if call['path'] == '/backend-api/f/conversation']
    assert image_calls
    if fixed:
        assert not any(call['path'].startswith('/backend-api/codex/') for call in calls)
        for call in image_calls:
            assert not any(field in call['body'] for field in ['output_format', 'frame_count', 'frame_duration_ms', 'animation'])
    return cases


report = []
try:
    compose('up', '-d', '--wait', 'postgres', 'pool-cache', 'downstream-cache')
    sql('postgres', 'CREATE DATABASE pool')
    sql('postgres', 'CREATE DATABASE downstream')
    for stage, image, fixed in [('BASELINE', 'sub2api:gif-contract-baseline-20261110', False),
                                ('MODIFIED', 'sub2api:gif-contract-fixed-20261110', True),
                                ('ROLLBACK', 'sub2api:gif-contract-baseline-20261110', False),
                                ('RESTORED', 'sub2api:gif-contract-fixed-20261110', True)]:
        ENV['GIF_IMAGE'] = image
        compose('rm', '-sf', 'provider', 'downstream', 'pool')
        compose('up', '-d', 'pool', 'downstream')
        compose('up', '-d', '--no-deps', 'provider')
        for instance in PORTS:
            for _ in range(100):
                try:
                    if request(instance, '/ready')[0] == 200:
                        break
                except OSError:
                    pass
                time.sleep(.5)
            else:
                raise AssertionError(instance + ' not ready')
        result = {'stage': stage, 'same_image_for_both_gateways': image, 'status': 'PASS', 'cases': check(stage, fixed)}
        report.append(result)
        print(json.dumps(result), flush=True)
    (ROOT / 'result.json').write_text(json.dumps(report, indent=2) + '\n')
finally:
    compose('down', '--volumes', '--remove-orphans')
