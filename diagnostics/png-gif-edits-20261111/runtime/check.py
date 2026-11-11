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
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'compose.yaml'), '-p', 'sub2api-png-gif-edits-20261111']
ENV = {**os.environ, 'EDIT_IMAGE': 'sub2api:gif-contract-fixed-20261110',
       'GIF_DB_PASSWORD': secrets.token_urlsafe(24), 'GIF_ADMIN_EMAIL': 'admin@fixture.invalid',
       'GIF_ADMIN_PASSWORD': secrets.token_urlsafe(24), 'GIF_JWT_SECRET': secrets.token_urlsafe(48),
       'GIF_ENCRYPTION_KEY': secrets.token_hex(32)}
PORTS = {'pool': 18583, 'downstream': 18584}
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
    png = (ROOT / 'reference.png').read_bytes()
    payload = {'model': model, 'prompt': 'Animate this PNG in a seamless loop.', 'output_format': 'gif',
               'size': 'auto', 'frame_count': 4, 'frame_duration_ms': 200, **body}
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
    gid, _ = fixture('downstream', stage, 'apikey', {'api_key': pool_key, 'base_url': 'http://pool:8080/v1'})
    cases = []
    before = len(storage_uploads())
    for model in ['gpt-image-2.6-sunburst', 'gpt-image-2.6-flare']:
        for multipart in [False, True]:
            for defaults in [False, True]:
                fields = {'quality': 'auto', 'complexity': 'high', 'moderation': 'auto'} if defaults else {}
                kid, client_key = key('downstream', downstream_token, gid, stage + str(len(cases)))
                code, body = submit_edit(fields, client_key, multipart, model)
                if fixed:
                    assert code == 200, (stage, model, multipart, defaults, code, body)
                    assert base64.b64decode(body['data'][0]['b64_json']).startswith(b'GIF89a')
                    assert settled('downstream', kid) == [{'images': 4, 'cost': .4}]
                    assert settled('pool', pool_kid, len(cases) + 1)[-1] == {'images': 4, 'cost': .4}
                else:
                    assert code == (400 if defaults else 502), (stage, code, body)
                    if defaults:
                        assert 'GIF output is not enabled for this request.' in str(body)
                cases.append({'model': model, 'multipart': multipart, 'explicit_defaults': defaults,
                              'status': code, 'billable_frames_per_gateway': 4 if fixed else 0})
    uploads = storage_uploads()[before:]
    assert len(uploads) == (8 if fixed else 0), (stage, uploads)
    for upload in uploads:
        assert upload['sha256'] == hashlib.sha256((ROOT / 'reference.png').read_bytes()).hexdigest()
        assert not upload['has_auth'] and upload['method'] == 'PUT'
    if fixed:
        assert all(not call['path'].endswith('/codex/images/edits') for call in provider_calls())
    return cases


cert_dir = Path(tempfile.mkdtemp(prefix='png-gif-fixture-cert-'))
ENV['EDIT_CERT_DIR'] = str(cert_dir)
report = []
try:
    config = cert_dir / 'cert.cnf'
    config.write_text('[req]\ndistinguished_name=dn\nx509_extensions=ext\nprompt=no\n[dn]\nCN=signed-storage.oaiusercontent.com\n[ext]\nsubjectAltName=DNS:signed-storage.oaiusercontent.com\nbasicConstraints=critical,CA:TRUE\nkeyUsage=critical,digitalSignature,keyEncipherment,keyCertSign\n')
    subprocess.run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '2',
                    '-config', str(config), '-keyout', str(cert_dir / 'server.key'), '-out', str(cert_dir / 'server.pem')],
                   check=True, capture_output=True)
    shutil.copyfile(cert_dir / 'server.pem', cert_dir / 'ca.pem')
    compose('up', '-d', '--wait', 'postgres', 'pool-cache', 'downstream-cache', 'storage')
    sql('postgres', 'CREATE DATABASE pool')
    sql('postgres', 'CREATE DATABASE downstream')
    for stage, image, fixed in [('BASELINE', 'sub2api:gif-contract-fixed-20261110', False),
                                ('MODIFIED', 'sub2api:png-gif-edits-fixed-20261111', True),
                                ('ROLLBACK', 'sub2api:gif-contract-fixed-20261110', False),
                                ('RESTORED', 'sub2api:png-gif-edits-fixed-20261111', True)]:
        ENV['EDIT_IMAGE'] = os.environ.get('EDIT_VERIFY_IMAGE', image)
        if os.environ.get('EDIT_VERIFY_IMAGE'):
            stage, fixed = 'PUBLISHED', True
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
                raise AssertionError('gateway not ready')
        result = {'stage': stage, 'same_image_for_both_gateways': ENV['EDIT_IMAGE'], 'result': 'PASS', 'cases': check(stage, fixed)}
        report.append(result)
        print(json.dumps(result), flush=True)
        if os.environ.get('EDIT_VERIFY_IMAGE'):
            break
    result_path = Path(os.environ.get('EDIT_RESULT_PATH', str(ROOT / 'result.json')))
    result_path.write_text(json.dumps(report, indent=2) + '\n')
finally:
    compose('down', '--volumes', '--remove-orphans')
    shutil.rmtree(cert_dir)
