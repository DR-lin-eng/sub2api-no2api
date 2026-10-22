"""Isolated same-database baseline, upgrade, rollback and re-upgrade proof."""
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
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'compose.yaml'), '-p', 'sub2api-pr153-wda5i24i']
BASELINE = 'ghcr.io/dr-lin-eng/sub2api-no2api:sha-1aebeba'
CANDIDATE = 'sub2api:pr153-reviewed-20261022'
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
    req = urllib.request.Request('http://127.0.0.1:18153' + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
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


def login():
    status, key = request('/api/v1/auth/credential-key')
    assert status == 200, ('credential key', status)
    envelope = json.loads(subprocess.check_output(['docker', 'run', '--rm', '-i', '-v', str(ROOT) + ':/fixture:ro',
        'node:24-alpine', 'node', '/fixture/credential_envelope_fixture.mjs'],
        input=json.dumps({'server': key['data'], 'email': ENV['SYNC_ADMIN_EMAIL'], 'password': ENV['SYNC_ADMIN_PASSWORD']}).encode()))
    status, result = request('/api/v1/auth/login', {'credential_envelope': envelope})
    assert status == 200, ('encrypted login', status)
    return result['data']['access_token'], result['data']['user']['id']


def start(image):
    ENV['SYNC_IMAGE'] = image
    compose('rm', '-sf', 'app')
    compose('up', '-d', 'app')
    for _ in range(100):
        try:
            if request('/ready')[0] == 200:
                return
        except OSError:
            pass
        time.sleep(.5)
    raise AssertionError('app failed readiness: ' + compose('logs', '--tail', '15', 'app'))


def snapshot():
    raw = sql("""SELECT json_build_object('users',(SELECT jsonb_agg(jsonb_build_array(id,email,password_hash,balance)) FROM users),
        'keys',(SELECT jsonb_agg(jsonb_build_array(id,name,md5(key),status,group_id)) FROM api_keys),
        'groups',(SELECT jsonb_agg(jsonb_build_array(id,name,platform)) FROM groups),
        'subscriptions',(SELECT jsonb_agg(jsonb_build_array(user_id,group_id,expires_at,status)) FROM user_subscriptions))""")
    return hashlib.sha256(raw.encode()).hexdigest()


def stage(label, expected, invariant):
    token = TOKEN
    status, admin = request('/api/v1/admin/settings', token=token)
    assert status == 200, ('admin settings', status, admin)
    status, public = request('/api/v1/settings/public')
    assert status == 200, ('public settings', status)
    admin_value = admin['data'].get('channel_monitor_timeline_points')
    public_value = public['data'].get('channel_monitor_timeline_points')
    assert admin_value == public_value == expected, (label, admin_value, public_value, expected)
    current = snapshot()
    assert current == invariant, ('identity/key/subscription invariant', label)
    record = {'stage': label, 'ready': 200, 'admin_points': admin_value, 'public_points': public_value,
        'migrations': int(sql('SELECT count(*) FROM schema_migrations')),
        'invariant_sha256': current, 'persisted_points': sql("SELECT value FROM settings WHERE key='channel_monitor_timeline_points'")}
    print(json.dumps(record), flush=True)
    return record, token


records = []
try:
    start(BASELINE)
    TOKEN, uid = login()
    # Seed the already-acknowledged fictional administrator fixture.
    ack = json.dumps({'version': 'v2026.06.10', 'admin_user_id': uid, 'accepted_at': '2026-10-22T00:00:00Z'})
    sql(f"INSERT INTO settings(key,value) VALUES ('admin_compliance_acknowledgement:{uid}','{ack}')")
    gid = int(sql("INSERT INTO groups(name,platform) VALUES ('PR153 fixture','openai') RETURNING id"))
    # A managed canonical key created after the historical migration must keep its name.
    for name in ['Media Studio', 'Media Studio Legacy fixture']:
        value = secrets.token_urlsafe(32)
        sql(f"INSERT INTO api_keys(user_id,key,name,group_id,status) VALUES ({uid},'{value}','{name}',{gid},'active')")
    invariant = snapshot()
    before, _ = stage('BASELINE', None, invariant)
    records.append(before)
    start(CANDIDATE)
    upgraded, token = stage('MODIFIED_DEFAULT', 20, invariant)
    records.append(upgraded)
    status, saved = request('/api/v1/admin/settings', {'channel_monitor_timeline_points': 35}, token, 'PUT')
    assert status == 200 and saved['data']['channel_monitor_timeline_points'] == 35, ('save', status)
    status, _ = request('/api/v1/admin/settings', {'site_name': 'PR153 upgrade fixture'}, token, 'PUT')
    assert status == 200, ('old-client partial save', status)
    saved, _ = stage('MODIFIED_SAVED', 35, invariant)
    records.append(saved)
    start(BASELINE)
    rolled, _ = stage('ROLLBACK', None, invariant)
    assert rolled['persisted_points'] == '35', 'old binary must preserve additive setting'
    records.append(rolled)
    start(CANDIDATE)
    reapplied, _ = stage('REAPPLIED', 35, invariant)
    records.append(reapplied)
    assert len({item['migrations'] for item in records}) == 1, 'no migrations added'
    (ROOT / 'result.json').write_text(json.dumps({'passed': True, 'records': records}, indent=2) + '\n')
finally:
    compose('down', '-v', '--remove-orphans')
