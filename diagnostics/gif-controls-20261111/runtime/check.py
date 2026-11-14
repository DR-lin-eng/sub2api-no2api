"""Two same-version gateways, separate DB/cache, disposable fixture credentials."""
import base64
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import tempfile
import shutil
import uuid
import hashlib
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parent
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'compose.yaml'), '-p', 'sub2api-app-optional-20261111']
ENV = {**os.environ, 'EDIT_IMAGE': 'sub2api:app-image-optional-20261111',
       'GIF_DB_PASSWORD': secrets.token_urlsafe(24), 'GIF_ADMIN_EMAIL': 'admin@fixture.invalid',
       'GIF_ADMIN_PASSWORD': secrets.token_urlsafe(24), 'GIF_JWT_SECRET': secrets.token_urlsafe(48),
       'GIF_ENCRYPTION_KEY': secrets.token_hex(32)}
PORTS = {'pool': 18593, 'downstream': 18594}
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


def storage_uploads():
    return json.loads(compose('exec', '-T', 'storage', 'python', '-c',
        'import urllib.request,ssl;print(urllib.request.urlopen("https://signed-storage.oaiusercontent.com:8443/uploads",context=ssl.create_default_context(cafile="/cert/ca.pem")).read().decode())'))


def submit_edit(body, token, multipart=False, model='gpt-image-2.6-sunburst'):
    png = (ROOT.parents[1] / 'png-gif-edits-20261111/runtime/reference.png').read_bytes()
    payload = {'model': model, 'prompt': 'Animate this PNG in a seamless loop.', 'output_format': 'gif',
               'size': 'auto', 'frame_count': 4, 'frame_duration_ms': 200, **body}
    if payload['output_format'] != 'gif':
        payload.pop('frame_count', None)
        payload.pop('frame_duration_ms', None)
    if not multipart:
        payload['images'] = [{'image_url': 'data:image/png;base64,' + base64.b64encode(png).decode()}]
        return request('downstream', '/v1/images/edits', payload, token)
    boundary = 'fixture-' + uuid.uuid4().hex
    data = bytearray()
    for key, value in payload.items():
        data.extend(f'--{boundary}\r\nContent-Disposition: form-data; name="{key}"\r\n\r\n{value}\r\n'.encode())
    data.extend(f'--{boundary}\r\nContent-Disposition: form-data; name="image[]"; filename="reference.png"\r\nContent-Type: image/png\r\n\r\n'.encode())
    data.extend(png)
    data.extend(f'\r\n--{boundary}--\r\n'.encode())
    req = urllib.request.Request(f'http://127.0.0.1:{PORTS["downstream"]}/v1/images/edits', data=bytes(data),
        headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'multipart/form-data; boundary=' + boundary})
    try:
        response = OPENERS['downstream'].open(req, timeout=40)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        return response.code, json.loads(response.read())


def check(stage, fixed):
    pool_token, downstream_token = login('pool'), login('downstream')
    gid, _ = fixture('pool', stage, 'oauth', {'access_token': 'fixture-oauth'},
        {'custom_base_url_enabled': True, 'custom_base_url': 'http://127.0.0.1:8000/backend-api/codex'})
    pool_kid, pool_key = key('pool', pool_token, gid, stage)
    outer_gid, _ = fixture('downstream', stage, 'apikey', {'api_key': pool_key, 'base_url': 'http://pool:8080/v1'})
    options = {'size': '1536x1024', 'background': 'transparent', 'quality': 'low', 'complexity': 'medium',
               'moderation': 'low', 'input_fidelity': 'high', 'style': 'vivid',
               'safety_identifier': 'fixture-hash', 'user': 'fixture-user'}
    cases = []
    for model in ['gpt-image-2.6-sunburst', 'gpt-image-2.6-flare']:
        for gif in [False, True]:
            fields = {**options, 'output_format': 'gif' if gif else 'jpeg'}
            if gif:
                fields.update(frame_count=9, frame_duration_ms=200)
            else:
                fields.update(output_compression=80, partial_images=1)
            for mode in ['generation', 'stream', 'edit-json', 'edit-multipart']:
                if gif and mode == 'stream':
                    continue
                kid, client_key = key('downstream', downstream_token, outer_gid, stage + str(len(cases)))
                if mode.startswith('edit'):
                    code, body = submit_edit(fields, client_key, mode == 'edit-multipart', model)
                else:
                    code, body = request('downstream', '/v1/images/generations',
                        {'model': model, 'prompt': 'draw a blue circle', 'n': 1, 'stream': mode == 'stream', **fields}, client_key)
                if not fixed and mode == 'stream':
                    # The Images heartbeat may have committed HTTP 200 before
                    # the native endpoint rejects the request. Check SSE error
                    # semantics and absence of a completed image as well.
                    assert code == 200 and 'fixture: native image route selected' in body
                    assert 'image_generation.completed' not in body
                else:
                    assert code == (200 if fixed else 400), (stage, model, gif, mode, code, body)
                if fixed:
                    units = 9 if gif else 1
                    rows = settled('downstream', kid)
                    assert len(rows) == 1 and rows[0]['images'] == units, (stage, mode, rows)
                    pool_rows = settled('pool', pool_kid, len(cases) + 1)
                    assert pool_rows[-1]['images'] == units
                    assert 'fallback' not in str(body).lower() and 'legacy' not in str(body).lower()
                    if mode == 'stream':
                        assert 'image_generation.completed' in body
                    else:
                        actual = base64.b64decode(body['data'][0]['b64_json'])
                        assert actual.startswith(b'GIF89a' if gif else b'\x89PNG\r\n\x1a\n')
                        assert body['output_format'] == ('gif' if gif else 'png')
                        assert body['size'] == '1x1', body
                cases.append({'model': model, 'output': 'gif' if gif else 'jpeg-request', 'mode': mode,
                              'status': code, 'billable_units': (9 if gif else 1) if fixed else 0})
    if fixed:
        calls = provider_calls()
        assert all('/codex/' not in call['path'] for call in calls)
        for call in calls:
            if not call['path'].endswith('/f/conversation'):
                continue
            body = call['body']
            assert not any(field in body for field in [*options, 'output_format', 'output_compression', 'partial_images', 'frame_count', 'frame_duration_ms'])
            prompt = body['messages'][0]['content']['parts'][-1]
            if 'animated GIF' not in prompt:
                assert prompt in ['draw a blue circle', 'Animate this PNG in a seamless loop.']
        assert len(storage_uploads()) == 8
        expected_hash = hashlib.sha256((ROOT.parents[1] / 'png-gif-edits-20261111/runtime/reference.png').read_bytes()).hexdigest()
        assert all(row['sha256'] == expected_hash and not row['has_auth'] for row in storage_uploads())
    return cases


cert_dir = Path(tempfile.mkdtemp(prefix='image-optional-cert-'))
ENV['EDIT_CERT_DIR'] = str(cert_dir)
report = []
try:
    config = cert_dir / 'cert.cnf'
    config.write_text('[req]\ndistinguished_name=dn\nx509_extensions=ext\nprompt=no\n[dn]\nCN=signed-storage.oaiusercontent.com\n[ext]\nsubjectAltName=DNS:signed-storage.oaiusercontent.com\nbasicConstraints=critical,CA:TRUE\nkeyUsage=critical,digitalSignature,keyEncipherment,keyCertSign\n')
    subprocess.run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '2', '-config', str(config),
        '-keyout', str(cert_dir / 'server.key'), '-out', str(cert_dir / 'server.pem')], check=True, capture_output=True)
    shutil.copyfile(cert_dir / 'server.pem', cert_dir / 'ca.pem')
    compose('up', '-d', '--wait', 'postgres', 'pool-cache', 'downstream-cache', 'storage')
    sql('postgres', 'CREATE DATABASE pool')
    sql('postgres', 'CREATE DATABASE downstream')
    stages = [('BASELINE', 'ghcr.io/dr-lin-eng/sub2api-no2api@sha256:627f6ace313ab71af53e6b0a18238a3cafdc218bb7bfd0b243d95ecadc89b7c8', False),
              ('MODIFIED', 'sub2api:app-image-optional-20261111', True),
              ('ROLLBACK', 'ghcr.io/dr-lin-eng/sub2api-no2api@sha256:627f6ace313ab71af53e6b0a18238a3cafdc218bb7bfd0b243d95ecadc89b7c8', False),
              ('RESTORED', 'sub2api:app-image-optional-20261111', True)]
    if os.environ.get('EDIT_VERIFY_IMAGE'):
        stages = [('PUBLISHED', os.environ['EDIT_VERIFY_IMAGE'], True)]
    for stage, image, fixed in stages:
        ENV['EDIT_IMAGE'] = image
        compose('rm', '-sf', 'provider', 'downstream', 'pool', 'storage')
        compose('up', '-d', 'pool', 'downstream', 'storage')
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
                raise AssertionError('gateway not ready')
        result = {'stage': stage, 'image': image, 'result': 'PASS', 'cases': check(stage, fixed)}
        report.append(result)
        print(json.dumps(result), flush=True)
    Path(os.environ.get('EDIT_RESULT_PATH', str(ROOT / 'result.json'))).write_text(json.dumps(report, indent=2) + '\n')
finally:
    compose('down', '--volumes', '--remove-orphans')
    shutil.rmtree(cert_dir)
