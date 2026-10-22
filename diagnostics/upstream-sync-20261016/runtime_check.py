#!/usr/bin/env python3
"""Run baseline -> upgrade -> rollback -> restore on one disposable database.

Ephemeral credentials and bearer tokens stay in memory and are never reported.
Refund finalization across mixed binaries is covered separately by source probes
and PostgreSQL tests; deployment must drain held refunds before binary rollback.
"""
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
COMPOSE = ["docker", "compose", "-f", str(ROOT / "compose.yaml"), "-p", "sub2api-sync-20261016-76ea"]
BASE = "http://127.0.0.1:18116"
PROVIDER = "http://127.0.0.1:18117"
BASELINE = "ghcr.io/dr-lin-eng/sub2api-no2api:sha-ed58484"
CANDIDATE = "sub2api-upstream-sync:20261016-candidate"
ENV = {**os.environ, "SYNC_DB_PASSWORD": secrets.token_urlsafe(24),
       "SYNC_ADMIN_EMAIL": "admin@fixture.invalid", "SYNC_ADMIN_PASSWORD": secrets.token_urlsafe(24),
       "SYNC_JWT_SECRET": secrets.token_urlsafe(48), "SYNC_ENCRYPTION_KEY": secrets.token_hex(32),
       "SYNC_IMAGE": BASELINE}
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}),
    urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
REPORT = []


def compose(*args):
    result = subprocess.run(COMPOSE + list(args), env=ENV, capture_output=True, text=True)
    if result.returncode:
        raise RuntimeError("compose " + " ".join(args) + ": " + result.stderr)
    return result.stdout.strip()


def sql(statement):
    return compose("exec", "-T", "postgres", "psql", "-U", "sub2api", "-d", "upgrade",
                   "-v", "ON_ERROR_STOP=1", "-qAtc", statement)


def request(path, body=None, token=None, method=None, base=BASE, form=False):
    headers = {"Content-Type": "application/x-www-form-urlencoded" if form else "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    data = None if body is None else (urllib.parse.urlencode(body).encode() if form else json.dumps(body).encode())
    req = urllib.request.Request(base + path, data=data, headers=headers, method=method)
    try:
        response = OPENER.open(req, timeout=20)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        raw = response.read().decode()
        try:
            content = json.loads(raw)
        except ValueError:
            content = raw
        return response.code, content


def start(image, installed=False):
    ENV["SYNC_IMAGE"] = image
    admin_email, admin_password = ENV["SYNC_ADMIN_EMAIL"], ENV["SYNC_ADMIN_PASSWORD"]
    if installed:
        ENV["SYNC_ADMIN_EMAIL"], ENV["SYNC_ADMIN_PASSWORD"] = "invalid-init-email", "123456"
    compose("up", "-d", "--force-recreate", "app")
    compose("up", "-d", "--force-recreate", "--no-deps", "provider")
    ENV["SYNC_ADMIN_EMAIL"], ENV["SYNC_ADMIN_PASSWORD"] = admin_email, admin_password
    for _ in range(100):
        try:
            if request("/ready")[0] == 200:
                return
        except (OSError, ValueError):
            pass
        time.sleep(0.5)
    raise AssertionError("application failed readiness")


def login():
    status, key = request("/api/v1/auth/credential-key")
    assert status == 200
    envelope = json.loads(subprocess.check_output(["docker", "run", "--rm", "-i", "-v",
        str(ROOT) + ":/fixture:ro", "node:24-alpine", "node", "/fixture/credential_envelope_fixture.mjs"],
        input=json.dumps({"server": key["data"], "email": ENV["SYNC_ADMIN_EMAIL"],
                          "password": ENV["SYNC_ADMIN_PASSWORD"]}).encode()))
    status, result = request("/api/v1/auth/login", {"credential_envelope": envelope})
    assert status == 200, ("encrypted login", status)
    return result["data"]["access_token"], result["data"]["user"]["id"]


def invariant_snapshot(uid, key_id):
    return sql(f"""SELECT json_build_object(
      'user',(SELECT jsonb_build_array(id,email,password_hash) FROM users WHERE id={uid}),
      'key',(SELECT jsonb_build_array(id,name,quota) FROM api_keys WHERE id={key_id}),
      'legacy',(SELECT jsonb_build_array(amount,pay_amount,bonus_amount,status) FROM payment_orders WHERE out_trade_no='sync-20261016-legacy'),
      'subscriptions',(SELECT jsonb_agg(jsonb_build_array(user_id,group_id,expires_at,status)) FROM user_subscriptions),
      'migrations',(SELECT COUNT(*) FROM schema_migrations))""")


def create_fixture_order(uid, instance_id, name, kind="balance"):
    return int(sql(f"""INSERT INTO payment_orders
      (user_id,amount,pay_amount,expires_at,out_trade_no,payment_type,order_type,status,recharge_code,provider_instance_id)
      VALUES ({uid},10,10,NOW()+INTERVAL '1 day','{name}','alipay','{kind}','PENDING','CODE-{name}','{instance_id}') RETURNING id"""))


def notify(name):
    params = {"pid": "1000", "trade_no": "TX-" + name, "out_trade_no": name,
              "type": "alipay", "name": "fixture", "money": "10.00", "trade_status": "TRADE_SUCCESS"}
    signed = "&".join(f"{key}={value}" for key, value in sorted(params.items())) + "fixture-only"
    params["sign"] = hashlib.md5(signed.encode()).hexdigest()
    params["sign_type"] = "MD5"
    return request("/api/v1/payment/webhook/easypay", params, form=True)[0]


def verify_stage(stage, fixed, uid, key_id, instance_id, snapshot):
    assert request("/health")[0] == 200 and request("/ready")[0] == 200
    assert request("/")[0] == 200
    token, found_uid = login()
    assert found_uid == uid
    status, key = request(f"/api/v1/keys/{key_id}", token=token)
    assert status == 200 and key["data"]["quota"] == 12.34
    assert invariant_snapshot(uid, key_id) == snapshot, stage + " changed persisted invariants"
    status, legacy = request("/api/v1/payment/public/orders/verify", {"out_trade_no": "sync-20261016-legacy"})
    assert status == 200 and legacy["data"]["status"] == "PENDING"
    status, result = request("/api/v1/payment/orders", {"amount": 10, "payment_type": "alipay", "order_type": "unsupported"}, token)
    assert status == (400 if fixed else 200), (stage, "order type", status)

    name = "sub2_" + stage + "_verified"
    order_id = create_fixture_order(uid, instance_id, name)
    before = float(sql(f"SELECT balance FROM users WHERE id={uid}"))
    request("/fixture/state", {name: {"paid": 0}}, base=PROVIDER)
    unpaid_status = notify(name)
    state = sql(f"SELECT status FROM payment_orders WHERE id={order_id}")
    assert unpaid_status == (502 if fixed else 200), (stage, unpaid_status)
    assert state == ("PENDING" if fixed else "COMPLETED"), (stage, state)
    if fixed:
        assert float(sql(f"SELECT balance FROM users WHERE id={uid}")) == before
        request("/fixture/state", {name: {"paid": 1}}, base=PROVIDER)
        assert notify(name) == 200
    assert float(sql(f"SELECT balance FROM users WHERE id={uid}")) == before + 10
    calls_before = request("/fixture/stats", base=PROVIDER)[1].get(name, 0)
    assert notify(name) == 200
    calls_after = request("/fixture/stats", base=PROVIDER)[1].get(name, 0)
    assert calls_before == calls_after, "duplicate callback queried upstream"
    assert float(sql(f"SELECT balance FROM users WHERE id={uid}")) == before + 10

    unsupported = "sub2_" + stage + "_unsupported"
    unknown_id = create_fixture_order(uid, instance_id, unsupported, "unsupported")
    request("/fixture/state", {unsupported: {"paid": 1}}, base=PROVIDER)
    before = float(sql(f"SELECT balance FROM users WHERE id={uid}"))
    assert notify(unsupported) == 200
    unknown_status = sql(f"SELECT status FROM payment_orders WHERE id={unknown_id}")
    assert unknown_status == ("FAILED" if fixed else "COMPLETED"), (stage, unknown_status)
    assert float(sql(f"SELECT balance FROM users WHERE id={uid}")) == before + (0 if fixed else 10)
    row = {"stage": stage, "result": "PASS", "migration_count": int(sql("SELECT COUNT(*) FROM schema_migrations")),
           "invariants_sha256": hashlib.sha256(snapshot.encode()).hexdigest(),
           "unknown_order_type_http": status, "unpaid_callback_http": unpaid_status,
           "unpaid_order_state": state, "unsupported_fulfillment_state": unknown_status,
           "duplicate_additional_queries": calls_after - calls_before,
           "readiness_frontend_encrypted_login_api_key_legacy_order_subscription": "PASS"}
    REPORT.append(row)
    print(json.dumps(row), flush=True)


try:
    compose("up", "-d", "postgres", "redis")
    start(BASELINE)
    token, uid = login()
    sql(f"""INSERT INTO settings(key,value,updated_at) VALUES ('admin_compliance_acknowledgement:{uid}',
      json_build_object('version','v2026.06.10','accepted_at',NOW(),'admin_user_id',{uid})::text,NOW())
      ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value""")
    status, _ = request("/api/v1/admin/payment/config", {"enabled": True, "enabled_payment_types": ["alipay"],
        "min_amount": 1, "max_amount": 10000, "max_pending_orders": 100}, token, method="PUT")
    assert status == 200, ("payment config", status)
    status, provider = request("/api/v1/admin/payment/providers", {"provider_key": "easypay", "name": "fixture",
        "config": {"pid": "1000", "pkey": "fixture-only", "apiBase": "http://127.0.0.1:8000",
                   "notifyUrl": BASE + "/api/v1/payment/webhook/easypay", "returnUrl": BASE + "/payment/result"},
        "supported_types": ["alipay"], "enabled": True, "payment_mode": "popup"}, token)
    assert status == 201, ("provider fixture", status)
    instance_id = provider["data"]["id"]
    status, key = request("/api/v1/keys", {"name": "upgrade-fixture", "quota": 12.34}, token)
    assert status == 200
    key_id = key["data"]["id"]
    sql(f"UPDATE users SET balance=100 WHERE id={uid}")
    sql(f"""INSERT INTO payment_orders (user_id,amount,pay_amount,expires_at,out_trade_no,payment_type,order_type,status)
      VALUES ({uid},12.34,12.34,NOW()+INTERVAL '1 day','sync-20261016-legacy','alipay','balance','PENDING')""")
    group_id = int(sql("INSERT INTO groups(name,platform,status,rate_multiplier) VALUES ('upgrade-fixture','openai','active',1) RETURNING id"))
    sql(f"INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status) VALUES ({uid},{group_id},NOW(),NOW()+INTERVAL '1 day','active')")
    snapshot = invariant_snapshot(uid, key_id)
    for stage, image, fixed in [("baseline", BASELINE, False), ("upgraded", CANDIDATE, True),
                                ("rollback", BASELINE, False), ("restored", CANDIDATE, True)]:
        if stage != "baseline":
            start(image, installed=True)
        verify_stage(stage, fixed, uid, key_id, instance_id, snapshot)
    (ROOT / "runtime-result.json").write_text(json.dumps(REPORT, indent=2) + "\n")
finally:
    compose("down", "--volumes", "--remove-orphans")
