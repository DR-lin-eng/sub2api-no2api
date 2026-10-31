#!/usr/bin/env python3
"""Disposable same-database upgrade/rollback fixture; never targets a deployment."""
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
COMPOSE = ['docker', 'compose', '-p', 'sub2api-pr169-compat', '-f', str(ROOT / 'compose.yaml')]
BASELINE = 'sub2api:multi-image-2f4a41cb8'
CANDIDATE = 'sub2api:pr169-reviewed'
ORIGIN = 'http://127.0.0.1:18489'
ENV = {**os.environ, 'PR169_FIXTURE_DB_PASSWORD': secrets.token_urlsafe(24),
       'PR169_FIXTURE_JWT_SECRET': secrets.token_hex(48), 'PR169_FIXTURE_ENCRYPTION_KEY': secrets.token_hex(32),
       'PR169_FIXTURE_IMAGE': BASELINE, 'PR169_FIXTURE_RESOURCE': ''}
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


def request(path, payload=None, token=None):
    headers = {'Content-Type': 'application/json'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    req = urllib.request.Request(ORIGIN + path, headers=headers,
                                 data=None if payload is None else json.dumps(payload).encode())
    try:
        response = OPENER.open(req, timeout=20)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        raw = response.read()
        try:
            body = json.loads(raw)
        except ValueError:
            body = raw.decode(errors='replace')
        return response.code, body


def login():
    code, data = request('/api/v1/auth/credential-key')
    assert code == 200, ('credential key', code)
    encrypted = subprocess.check_output(['docker', 'run', '--rm', '-i', '-v', str(ROOT) + ':/fixture:ro',
        'node:24-alpine', 'node', '/fixture/credential_envelope_fixture.mjs'],
        input=json.dumps({'server': data['data'], 'email': 'admin@pr169-fixture.invalid',
                          'password': 'Local-fixture-only-20261030!'}).encode())
    code, body = request('/api/v1/auth/login', {'credential_envelope': json.loads(encrypted)})
    assert code == 200, ('encrypted login', code)
    return body['data']['access_token']


def start(image):
    ENV['PR169_FIXTURE_IMAGE'] = image
    compose('up', '-d', '--no-deps', '--force-recreate', 'app')
    for _ in range(100):
        try:
            if request('/ready')[0] == 200:
                return
        except OSError:
            pass
        time.sleep(.5)
    raise AssertionError('application readiness failed')


def snapshot():
    value = sql("""SELECT json_build_object(
      'user',(SELECT jsonb_agg(jsonb_build_array(id,email,password_hash,balance)) FROM users),
      'key',(SELECT jsonb_agg(jsonb_build_array(id,user_id,group_id,name,quota,key)) FROM api_keys),
      'group',(SELECT jsonb_agg(jsonb_build_array(id,name,platform,status)) FROM groups),
      'templates',(SELECT jsonb_agg(jsonb_build_array(id,name,request_adapter)) FROM custom_model_request_templates),
      'configs',(SELECT jsonb_agg(jsonb_build_array(id,model_name,capabilities,template_id)) FROM custom_model_configs))""")
    return hashlib.sha256(value.encode()).hexdigest()


report = []
try:
    compose('up', '-d', '--wait', 'postgres', 'redis')
    start(BASELINE)
    sql("""INSERT INTO settings(key,value) VALUES
        ('media_studio_enabled','true'), ('custom_model_config_enabled','true')
        ON CONFLICT(key) DO UPDATE SET value=excluded.value;
        INSERT INTO groups(name,platform,status,rate_multiplier,subscription_type)
        VALUES ('studio-compat','openai','active',1,'standard');
        INSERT INTO custom_model_request_templates(name,description,request_adapter)
        VALUES ('compat-template','fixture','{}');
        INSERT INTO custom_model_configs(model_name,prefix_match,capabilities,video_api_type,template_id)
        SELECT 'fixture-image',false,'["image"]','',id FROM custom_model_request_templates WHERE name='compat-template';
        INSERT INTO settings(key,value)
        SELECT 'media_studio_group_routes',json_build_array(json_build_object(
          'group_id',id,'priority',0,'enabled',true,'models',json_build_array('fixture-image')))::text
        FROM groups WHERE name='studio-compat' ON CONFLICT(key) DO UPDATE SET value=excluded.value;""")
    token = login()
    group = int(sql("SELECT id FROM groups WHERE name='studio-compat'"))
    code, _ = request('/api/v1/keys', {'name': 'keep-existing-key', 'group_id': group, 'quota': 12.34}, token)
    assert code == 200, ('key creation', code)
    before = snapshot()
    for stage, image in [('BASELINE', BASELINE), ('MODIFIED', CANDIDATE), ('ROLLBACK', BASELINE), ('RESTORED', CANDIDATE)]:
        if stage != 'BASELINE':
            start(image)
        assert snapshot() == before, ('business data changed', stage)
        token = login()
        code, config = request('/api/v1/media-studio/config', token=token)
        assert code == 200, ('studio config', stage, code)
        assert request('/api/v1/settings/public')[0] == 200
        assert request('/media-studio')[0] == 200
        count = int(sql('SELECT count(*) FROM schema_migrations'))
        if stage in ('MODIFIED', 'RESTORED'):
            assert any(g['group_id'] == group and g['media_type'] == 'image' for g in config['data']['groups'])
            assert sql("SELECT attributes::text FROM custom_model_request_templates WHERE name='compat-template'") == '{}'
            assert sql("SELECT media_limits::text FROM custom_model_configs WHERE model_name='fixture-image'") == '{}'
            assert sql("SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename IN ('media_studio_history','media_studio_gallery','media_studio_sessions')") == '0'
        report.append({'stage': stage, 'migrations': count, 'business_snapshot': before})
    (ROOT / 'result.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))
    if os.environ.get('PR169_KEEP_RUNTIME') == '1':
        print('Browser fixture ready on ' + ORIGIN)
finally:
    if os.environ.get('PR169_KEEP_RUNTIME') != '1':
        compose('down', '-v', '--remove-orphans')
