#!/usr/bin/env python3
"""Disposable local UI fixture; never connects to a deployed Sub2API."""
from pathlib import Path
import json
import time

fixture_path = Path(__file__).resolve().with_name('check.py')
namespace = {'__file__': str(fixture_path), '__name__': 'ui_fixture_helpers'}
exec(compile(fixture_path.read_text().split('\nreport=[]\n')[0], str(fixture_path), 'exec'), namespace)
compose = namespace['compose']
start = namespace['start']
sql = namespace['sql']
login = namespace['login']
namespace['ENV']['SYNC_ADMIN_EMAIL'] = 'ui-fixture@fixture.invalid'
namespace['ENV']['SYNC_ADMIN_PASSWORD'] = 'Local-fixture-only-20261029!'
stop = Path('/tmp/sub2api-sync-20261029-ui-stop')
ready = Path('/tmp/sub2api-sync-20261029-ui-ready')
stop.unlink(missing_ok=True)
ready.unlink(missing_ok=True)
try:
    compose('up', '-d', 'postgres', 'redis')
    start(namespace['CANDIDATE'])
    token, uid = login()
    ack = json.dumps({'version':'v2026.06.10','admin_user_id':uid,'accepted_at':'2026-10-29T00:00:00Z'})
    sql(f"INSERT INTO settings(key,value) VALUES ('admin_compliance_acknowledgement:{uid}','{ack}')")
    config = json.dumps({'enabled':True,'providers':[{'name':'fixture','type':'searxng','base_url':'http://127.0.0.1:8000'}]})
    sql(f"INSERT INTO settings(key,value) VALUES ('web_search_emulation_config','{config}') ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value")
    ready.write_text('http://127.0.0.1:18380/admin/channels\n')
    print('Disposable UI fixture ready on 127.0.0.1:18380', flush=True)
    deadline = time.monotonic()+600
    while not stop.exists() and time.monotonic()<deadline:
        time.sleep(.25)
finally:
    compose('down', '--volumes', '--remove-orphans')
    ready.unlink(missing_ok=True)
    stop.unlink(missing_ok=True)
