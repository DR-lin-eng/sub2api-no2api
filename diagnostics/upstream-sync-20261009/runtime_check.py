#!/usr/bin/env python3
"""Only synthetic data in the dedicated sub2api-sync-20261009-matrix Docker project."""
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
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'compose.yaml'), '-p', 'sub2api-sync-20261009-matrix']
BASE = 'http://127.0.0.1:18109'
BASELINE = 'sub2api-upstream-sync:20261009-baseline'
CANDIDATE = 'sub2api-upstream-sync:20261009-final'
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}),
                                   urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
REPORT = []


def compose(*args, env=None, capture=False):
    result = subprocess.run(COMPOSE + list(args), env={**os.environ, **(env or {})},
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True)
    return result.stdout.decode() if capture else None


def sql(database, statement):
    return compose('exec', '-T', 'postgres', 'psql', '-U', 'sub2api', '-d', database,
                   '-v', 'ON_ERROR_STOP=1', '-Atc', statement, capture=True).strip()


def request(path, body=None, token=None):
    headers = {'Content-Type': 'application/json'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    req = urllib.request.Request(BASE + path, headers=headers,
        data=None if body is None else json.dumps(body).encode())
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
        'docker', 'run', '--rm', '-i', '-v', str(ROOT / 'credential_envelope.mjs') + ':/fixture.mjs:ro',
        'node:24-alpine', 'node', '/fixture.mjs'],
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
    status, order = request('/api/v1/payment/public/orders/verify', {'out_trade_no': 'upstream-sync-20261008-legacy'})
    assert status == 200 and order['data']['status'] == 'PENDING' and order['data']['paid'] is False
    assert snapshot() == state['snapshot'], 'upgrade changed persisted credentials, keys, orders or migration count'
    REPORT.append({'stage': stage, 'result': 'PASS', 'migrations': int(sql('upgrade', 'SELECT COUNT(*) FROM schema_migrations'))})
    print(stage + ': readiness/frontend/encrypted-login/persisted-key/legacy-order/data-snapshot PASS', flush=True)


# This named project contains only this script's synthetic fixtures.
# Replays start from a new database rather than reusing prior fixture orders.
compose('down', '--volumes', '--remove-orphans')
compose('up', '-d', '--wait', '--wait-timeout', '60', 'postgres', 'redis')
start(BASELINE)
status, logged = login('admin@sub2api.local', 'sync_admin_password')
assert status == 200
token = logged['data']['access_token']
status, created = request('/api/v1/keys', {'name': 'upstream-sync-upgrade-fixture', 'quota': 12.34}, token)
assert status == 200
key = created['data']
sql('upgrade', """INSERT INTO payment_orders (user_id,amount,pay_amount,expires_at,out_trade_no,payment_type,order_type,status)
    SELECT id,12.34,12.34,NOW()+INTERVAL '1 day','upstream-sync-20261008-legacy','alipay','balance','PENDING'
    FROM users WHERE role='admin' ORDER BY id LIMIT 1;""")
state = {'user_id': logged['data']['user']['id'], 'key_id': key['id'], 'name': key['name'], 'snapshot': snapshot()}
runtime('baseline', state)
for stage, image in [('upgraded', CANDIDATE), ('rollback', BASELINE), ('restored', CANDIDATE)]:
    # Every replacement has a fresh data directory and stale invalid init env.
    # The installed database must be adopted without rewriting its admin.
    start(image, email='invalid-initialization-email', password='123456')
    runtime(stage, state)


(ROOT / "runtime-result.json").write_text(json.dumps(REPORT, indent=2)+"\n")
compose("down", "--volumes", "--remove-orphans")
