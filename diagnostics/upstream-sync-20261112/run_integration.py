#!/usr/bin/env python3
"""Run integration tests against disposable Docker-only PostgreSQL and Redis."""
from pathlib import Path
import os
import subprocess
import time
import uuid

repo = Path(__file__).resolve().parents[2]
name = 'sub2api-sync-20261112-it-' + uuid.uuid4().hex[:8]
created = []

def docker(*args, capture=False):
    return subprocess.run(['docker', *args], check=True, text=True,
                          stdout=subprocess.PIPE if capture else subprocess.DEVNULL).stdout

network = docker('network', 'create', name, capture=True).strip()
skip = os.environ.get('SYNC_INTEGRATION_SKIP', '')
go_test_args = ['go', 'test', '-tags=integration', './...', '-count=1', '-timeout=12m']
if skip:
    go_test_args += ['-skip', skip]
# Skip selection is an explicit runner option; never changes repository tests.
import shlex
integration_command = shlex.join(go_test_args)
try:
    pg = name + '-postgres'
    redis = name + '-redis'
    created.append(docker('run', '-d', '--name', pg, '--network', name,
                          '-e', 'POSTGRES_PASSWORD=fixture-only', '-e', 'POSTGRES_DB=sub2api_test',
                          'postgres:18.1-alpine3.23', capture=True).strip())
    created.append(docker('run', '-d', '--name', redis, '--network', name,
                          'redis:8.4-alpine', capture=True).strip())
    for _ in range(60):
        if subprocess.run(['docker', 'exec', pg, 'pg_isready', '-h', '127.0.0.1', '-U', 'postgres'],
                          stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
            break
        time.sleep(.5)
    else:
        raise RuntimeError('disposable PostgreSQL did not become ready')
    docker('exec', pg, 'createdb', '-U', 'postgres', 'prompt_audit')
    args = ['run', '--rm', '--network', name, '--cpus=2', '--memory=4g',
            '-v', str(repo) + ':/src:ro', '-v', '/private/tmp/sub2api-modcache:/go/pkg/mod',
            '-v', '/private/tmp/sub2api-gocache:/root/.cache/go-build',
            '-e', 'SUB2API_TEST_EXTERNAL_POSTGRES_DSN=postgres://postgres:fixture-only@' + pg + ':5432/sub2api_test?sslmode=disable',
            '-e', 'SUB2API_TEST_EXTERNAL_REDIS_ADDR=' + redis + ':6379',
            '-e', 'CI=true', '-e', 'CGO_ENABLED=0', '-e', 'GOMAXPROCS=2',
            '-e', 'GOMEMLIMIT=1400MiB', '-e', 'GOGC=50', '-e', 'GOFLAGS=-p=1',
            '-e', 'GOTOOLCHAIN=local', '-e', 'GOPROXY=https://goproxy.cn,direct',
            '-e', 'PROMPT_AUDIT_POSTGRES_FIXTURE=postgres://postgres:fixture-only@' + pg + ':5432/prompt_audit?sslmode=disable',
            '-e', 'PROMPT_AUDIT_REDIS_FIXTURE=' + redis + ':6379',
            'golang:1.26.11-alpine',
            'sh', '-c', 'mkdir /work && cp -a /src/backend /work/backend && cd /work/backend && '
            + integration_command + ' && '
            'PROMPT_AUDIT_TEST_POSTGRES_DSN="$PROMPT_AUDIT_POSTGRES_FIXTURE" '
            'PROMPT_AUDIT_TEST_REDIS_ADDR="$PROMPT_AUDIT_REDIS_FIXTURE" '
            'go test ./internal/modules/securityaudit -count=1 -timeout=5m -v']
    result = subprocess.run(['docker', *args])
    raise SystemExit(result.returncode)
finally:
    for container in reversed(created):
        subprocess.run(['docker', 'rm', '-f', container], stdout=subprocess.DEVNULL, check=False)
    subprocess.run(['docker', 'network', 'rm', network], stdout=subprocess.DEVNULL, check=False)
