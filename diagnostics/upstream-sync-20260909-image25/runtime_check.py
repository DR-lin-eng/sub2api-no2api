#!/usr/bin/env python3
"""Verify one pinned image using this review's isolated Docker stack."""
import json
import os
from pathlib import Path
import subprocess
import sys

phase, image = sys.argv[1:]
root = Path(__file__).resolve().parents[2]
compose = ["docker", "compose", "-p", "sub2api-sync-image25", "-f", str(Path(__file__).with_name("compose.yaml"))]
env = dict(os.environ, SYNC_IMAGE=image)
log = Path(__file__).with_name("runtime-"+phase+".log")
records = []
def run(args):
    result = subprocess.run(args, cwd=root, env=env, text=True, capture_output=True)
    records.append({"command":args,"stdout":result.stdout,"stderr":result.stderr,"exit_status":result.returncode})
    log.write_text(json.dumps({"phase":phase,"image":image,"records":records},ensure_ascii=False,indent=2)+"\n")
    if result.returncode:
        sys.stderr.write(result.stderr)
        raise SystemExit(result.returncode)
    return result.stdout.strip()

run(compose+["up","-d","--wait","--wait-timeout","180"])
health=json.loads(run(compose+["exec","-T","app","wget","-qO-","http://127.0.0.1:8080/health"]))
ready=json.loads(run(compose+["exec","-T","app","wget","-qO-","http://127.0.0.1:8080/ready"]))
schema=run(compose+["exec","-T","db","psql","-U","sync_test","-d","sync_test","-Atc","select count(*) from schema_migrations; select max(filename) from schema_migrations; select count(*) from users; select count(*) from information_schema.columns where table_name='groups' and column_name='model_allowlist';"]).splitlines()
assert health=={"status":"ok"},health
assert ready["ready"] and not ready["draining"],ready
assert schema==["293","239_add_usage_openai_timing.sql","1","1"],schema
version=run(compose+["exec","-T","app","/app/sub2api","-version"])
print(f"{phase.upper()}: health=ok ready=true migrations=293 users=1 model_allowlist=present")
print(version)
