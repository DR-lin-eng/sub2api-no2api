"""Authenticated HTTP deletion races against a gated, local-only upstream."""
import base64
import concurrent.futures
from decimal import Decimal
import hashlib
import hmac
import json
import pathlib
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request

HERE = pathlib.Path(__file__).resolve().parent
MODE = sys.argv[1]
BASE = "http://127.0.0.1:18089"
UPSTREAM = "http://127.0.0.1:18090"
PROJECT = "sub2api-billing-audit-20261006"
JWT_SECRET = b"local_billing_audit_jwt_abcdefghijklmnopqrstuvwxyz_0123456789"
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}))
COMPOSE = ["docker", "compose", "-f", str(HERE / "compose.yaml"), "-p", PROJECT]


def sql(query):
    out = subprocess.check_output(COMPOSE + ["exec", "-T", "postgres", "psql", "-U", "audit", "-d", "audit", "-v", "ON_ERROR_STOP=1", "-Atq", "-c", query], text=True)
    return out.strip()


def api(path, body=None, token=None, method=None, base=BASE):
    headers = {"Content-Type": "application/json", "User-Agent": "local-billing-audit"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(base + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
    try:
        response = OPENER.open(req, timeout=40)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        raw = response.read()
        return response.status, json.loads(raw) if raw else None


def until(check, seconds=45):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        try:
            value = check()
            if value:
                return value
        except (ConnectionError, urllib.error.URLError):
            pass
        time.sleep(0.15)
    raise AssertionError("timed out waiting for fixture/settlement")


def token_for(user):
    # This signed fixture token is valid only in the synthetic Compose project.
    # It still passes the real JWT, ownership, user-state and token-version checks.
    material = user["email"].lower() + "\n" + "audit-password-hash"
    version = int.from_bytes(hashlib.sha256(material.encode()).digest()[:8], "big") & 0x7FFFFFFFFFFFFFFF
    claims = {"user_id": user["id"], "email": user["email"], "role": "user", "token_version": version,
              "iat": int(time.time()), "nbf": int(time.time()) - 1, "exp": int(time.time()) + 3600}
    encode = lambda value: base64.urlsafe_b64encode(value).rstrip(b"=")
    raw = encode(b'{"alg":"HS256","typ":"JWT"}') + b"." + encode(json.dumps(claims).encode())
    return (raw + b"." + encode(hmac.new(JWT_SECRET, raw, hashlib.sha256).digest())).decode()


def state(user_id, group_id, account_id, platform):
    data = json.loads(sql(f"""SELECT json_build_object(
        'balance', balance::text,
        'daily',COALESCE((SELECT daily_usage_usd FROM user_subscriptions WHERE user_id={user_id} AND group_id={group_id}),0)::text,
        'weekly',COALESCE((SELECT weekly_usage_usd FROM user_subscriptions WHERE user_id={user_id} AND group_id={group_id}),0)::text,
        'monthly',COALESCE((SELECT monthly_usage_usd FROM user_subscriptions WHERE user_id={user_id} AND group_id={group_id}),0)::text,
        'platform',COALESCE((SELECT daily_usage_usd FROM user_platform_quotas WHERE user_id={user_id} AND platform='{platform}' AND deleted_at IS NULL),0)::text,
        'account',COALESCE((SELECT (extra->>'quota_used')::numeric FROM accounts WHERE id={account_id}),0)::text)
        FROM users WHERE id={user_id}"""))
    return {key: Decimal(value) for key, value in data.items()}


def generate(path, body, key, first_frame):
    headers = {"Authorization": "Bearer " + key, "Content-Type": "application/json"}
    if path == "/v1/messages":
        headers["anthropic-version"] = "2023-06-01"
    req = urllib.request.Request(BASE + path, data=json.dumps(body).encode(), headers=headers)
    try:
        response = OPENER.open(req, timeout=40)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        if body["stream"] and response.status == 200:
            prefix = bytearray()
            while True:
                line = response.readline()
                prefix.extend(line)
                if line in (b"\n", b"\r\n", b""):
                    first_frame.set()
                    break
            raw = bytes(prefix) + response.read()
        else:
            raw = response.read()
        return response.status, raw


until(lambda: api("/ready")[0] == 200, seconds=90)
if MODE == "direct":
    sql((HERE / "runtime-fixtures.sql").read_text())
    subprocess.run(COMPOSE + ["restart", "app"], check=True, stdout=subprocess.DEVNULL)
    until(lambda: api("/ready")[0] == 200, seconds=90)

users = {kind: json.loads(sql("SELECT json_build_object('id',id,'email',email) FROM users WHERE email='" + kind + "@billing-audit.invalid'")) for kind in ("balance", "subscription")}
records = []
routes = (("responses", "openai", "/v1/responses", "gpt-4o-mini"),
          ("chat", "openai", "/v1/chat/completions", "gpt-4o-mini"),
          ("messages", "anthropic", "/v1/messages", "claude-sonnet-4-20250514"))

for route, platform, path, model in routes:
    account_id = int(sql("SELECT id FROM accounts WHERE name='audit-" + platform + "'"))
    for kind, user in users.items():
        jwt = token_for(user)
        group_id = int(sql("SELECT id FROM groups WHERE name='audit-" + platform + "-" + kind + "'"))
        for stream in (False, True):
            case = "audit-" + "-".join((MODE, route, kind, "stream" if stream else "json"))
            key_request = {"name": case, "group_id": group_id, "quota": 50, "rate_limit_5h": 50, "rate_limit_1d": 50, "rate_limit_7d": 50}
            code, data = api("/api/v1/keys", key_request, jwt)
            assert code == 200, (code, data)
            key = data["data"]
            before = state(user["id"], group_id, account_id, platform)
            body = {"model": model, "stream": stream}
            if route == "responses":
                body["input"] = case
            else:
                body["messages"] = [{"role": "user", "content": case}]
                body["max_tokens"] = 600
                if stream and route == "chat":
                    body["stream_options"] = {"include_usage": True}
            first_frame = threading.Event()
            with concurrent.futures.ThreadPoolExecutor(max_workers=1) as pool:
                pending = pool.submit(generate, path, body, key["key"], first_frame)
                until(lambda: api("/control/status?case=" + case, base=UPSTREAM)[1]["started"])
                if stream:
                    assert first_frame.wait(10), "stream did not reach client before deletion"
                assert not pending.done(), "upstream must still be generating at deletion time"
                code, deleted = api("/api/v1/keys/" + str(key["id"]), token=jwt, method="DELETE")
                assert code == 200, (code, deleted)
                code, _ = api("/v1/models", token=key["key"])
                assert code == 401, ("deleted key still authenticates", code)
                key_request["name"] += "-replacement"
                key_request["custom_key"] = key["key"]
                code, data = api("/api/v1/keys", key_request, jwt)
                assert code == 200, (code, data)
                replacement = data["data"]
                assert replacement["id"] != key["id"]
                code, _ = api("/control/release", {"case": case}, base=UPSTREAM)
                assert code == 200
                code, raw = pending.result(timeout=40)
                assert code == 200, (case, code, raw[:1000])
                assert b"event: error" not in raw, (case, raw[:1000])
                if stream and route == "chat":
                    text = ""
                    for line in raw.splitlines():
                        if line.startswith(b"data:") and line[5:].strip() != b"[DONE]":
                            chunk = json.loads(line[5:].strip())
                            for choice in chunk.get("choices", []):
                                text += choice.get("delta", {}).get("content", "") or ""
                    assert text == "audit result", (case, text)
                else:
                    assert b"audit result" in raw, (case, raw[:1000])

            def settled():
                rows = json.loads(sql(f"""SELECT json_build_object(
                    'logs',(SELECT COUNT(*) FROM usage_logs WHERE api_key_id={key['id']}),
                    'dedup',(SELECT COUNT(*) FROM usage_billing_dedup WHERE api_key_id={key['id']}),
                    'jobs',(SELECT COUNT(*) FROM usage_billing_jobs WHERE api_key_id={key['id']}),
                    'dead',(SELECT COUNT(*) FROM usage_billing_dead_letters WHERE api_key_id={key['id']}))"""))
                return rows if rows == {"logs": 1, "dedup": 1, "jobs": 0, "dead": 0} else None
            ledger = until(settled)
            log = json.loads(sql(f"""SELECT json_build_object('input',input_tokens,'output',output_tokens,
                'total',total_cost::text,'actual',actual_cost::text,'stream',stream,'user_id',user_id,
                'group_id',group_id,'api_key_id',api_key_id,'subscription_id',subscription_id)
                FROM usage_logs WHERE api_key_id={key['id']}"""))
            assert log["input"] == 1000 and log["output"] == 500 and log["stream"] == stream, log
            cost, total = Decimal(log["actual"]), Decimal(log["total"])
            assert cost > 0 and total > 0, log
            after = state(user["id"], group_id, account_id, platform)
            delta = {name: after[name] - before[name] for name in before}
            close = lambda amount, expected: abs(amount - expected) <= Decimal("0.00000001")
            assert close(delta["account"], total), delta
            if kind == "balance":
                assert close(delta["balance"], -cost) and close(delta["platform"], cost), delta
                assert log["subscription_id"] is None
            else:
                assert delta["balance"] == 0 and log["subscription_id"] is not None
                assert all(close(delta[window], cost) for window in ("daily", "weekly", "monthly")), delta
            retained = json.loads(sql(f"""SELECT json_build_object('deleted',deleted_at IS NOT NULL,'tombstoned',key LIKE '__deleted__%',
                'quota',quota_used,'rate',usage_5h) FROM api_keys WHERE id={key['id']}"""))
            assert retained == {"deleted": True, "tombstoned": True, "quota": 0, "rate": 0}, retained
            assert sql(f"SELECT quota_used+usage_5h FROM api_keys WHERE id={replacement['id']}") == "0.00000000"
            summary = {"case": case, "cost": str(cost), "delta": {name: str(value) for name, value in delta.items()}, "ledger": ledger}
            records.append(summary)
            print("PASS", case, "tokens=1000+500", "charged=" + str(cost), "dedup=1 jobs=0 dead_letters=0", flush=True)

(HERE / ("runtime-" + MODE + ".json")).write_text(json.dumps(records, indent=2) + "\n")
print("RUNTIME_PASS", MODE, "scenarios=" + str(len(records)), flush=True)
