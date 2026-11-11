#!/usr/bin/env python3
"""Fixed source/CPU fixtures; never benchmarks a real upstream account."""
from pathlib import Path
import hashlib
import json
import re
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parents[1]
BASELINE = Path('/private/tmp/sub2api-sync-baseline-20261112')


def run(args, **kwargs):
    return subprocess.check_output(args, text=True, **kwargs)


samples = {}
for stage, source in [('baseline', BASELINE), ('candidate', REPO), ('rollback', BASELINE)]:
    output = run(['docker', 'run', '--rm', '--network=none', '--cpus=2', '--memory=4g',
        '-v', str(source / 'backend') + ':/source:ro', '-v', str(ROOT / 'performance') + ':/fixture:ro',
        '-v', '/private/tmp/sub2api-modcache:/go/pkg/mod', '-v', '/private/tmp/sub2api-gocache:/root/.cache/go-build',
        '-e', 'GOMAXPROCS=2', '-e', 'GOMEMLIMIT=1400MiB', '-e', 'GOGC=50', '-e', 'GOFLAGS=-p=1',
        '-e', 'GOPROXY=off', 'golang:1.26.11-alpine', 'sh', '-c',
        'mkdir /work && cp -a /source /work/backend && cd /work/backend && '
        'cp /fixture/bootstrap_test.go internal/shared/apicompat/sync_bootstrap_benchmark_test.go && '
        'go test ./internal/shared/apicompat -run "^$" -bench "^BenchmarkSyncBootstrapOrdinary1MiB$" -benchmem -benchtime=300ms -count=3 -cpu=1 && '
        'go test ./internal/application/service -run "^$" -bench "^BenchmarkDeriveOpenAIContentSessionSeedLargeBody$" -benchmem -benchtime=300ms -count=3 -cpu=1'])
    (ROOT / 'records' / ('performance-' + stage + '.stdout')).write_text(output)
    samples[stage] = []
    for line in output.splitlines():
        match = re.match(r'^(Benchmark\S+)\s+\d+\s+([\d.]+) ns/op\s+(?:[\d.]+ MB/s\s+)?(\d+) B/op\s+(\d+) allocs/op', line)
        if match:
            samples[stage].append(dict(name=match[1], ns_per_op=float(match[2]), bytes_per_op=int(match[3]), allocations=int(match[4])))
    print(json.dumps({'stage': stage, 'samples': samples[stage]}), flush=True)

container = 'sub2api-sync-20261112-perf-' + uuid.uuid4().hex[:8]
try:
    run(['docker', 'run', '-d', '--name', container, '--network=none', '--cpus=1', '--memory=1200m',
         '-e', 'POSTGRES_PASSWORD=fixture-only', 'postgres:18.1-alpine3.23'])
    for _ in range(60):
        ready = subprocess.run(['docker', 'exec', container, 'pg_isready', '-h', '127.0.0.1', '-U', 'postgres'],
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if ready.returncode == 0:
            break
        time.sleep(.5)
    else:
        raise RuntimeError('PostgreSQL fixture not ready')

    def sql(query):
        return run(['docker', 'exec', '-i', container, 'psql', '-U', 'postgres', '-qAt', '-v', 'ON_ERROR_STOP=1'], input=query)

    sql('''CREATE TABLE usage_logs(api_key_id bigint, created_at timestamptz, actual_cost numeric(20,10),
        input_tokens bigint, output_tokens bigint, cache_creation_tokens bigint, cache_read_tokens bigint, payload text);
        ALTER TABLE usage_logs ALTER COLUMN payload SET STORAGE EXTERNAL;
        INSERT INTO usage_logs SELECT (i%1000)+1, '2026-11-12 00:00:00+00'::timestamptz-(i%800)*interval '1 hour',
        (i%19)*0.00001, i%1000, i%100, i%20, i%40, repeat(md5(i::text),32) FROM generate_series(1,300000) i;
        CREATE INDEX idx_usage_logs_api_key_id ON usage_logs(api_key_id);
        CREATE INDEX idx_usage_logs_api_key_created_at ON usage_logs(api_key_id,created_at);
        VACUUM ANALYZE usage_logs;''')
    query = '''SELECT api_key_id,
        COALESCE(SUM(actual_cost) FILTER(WHERE created_at >= '2026-10-13'::timestamptz AND created_at < '2026-11-12'::timestamptz),0) AS total_cost,
        COALESCE(SUM(actual_cost) FILTER(WHERE created_at >= '2026-11-11'::timestamptz),0) AS today_cost,
        COALESCE(SUM(input_tokens+output_tokens+cache_creation_tokens+cache_read_tokens)
            FILTER(WHERE created_at >= '2026-10-13'::timestamptz AND created_at < '2026-11-12'::timestamptz),0) AS total_tokens
        FROM usage_logs WHERE api_key_id=ANY(ARRAY[1,2,3,4,5,6,7,8,9,10]::bigint[])
        AND created_at>=LEAST('2026-10-13'::timestamptz,'2026-11-11'::timestamptz) GROUP BY api_key_id'''
    db = {'fixture_rows': 300000, 'fixture_payload_bytes': 1024, 'key_batch_size': 10, 'samples': {}}
    expected = None
    for stage in ['baseline', 'candidate', 'rollback']:
        if stage == 'candidate':
            sql((REPO / 'backend/migrations/257_add_usage_logs_api_key_cost_index_notx.sql').read_text())
        elif stage == 'rollback':
            sql('DROP INDEX idx_usage_logs_api_key_created_cost;')
        actual = sql('COPY (' + query + ' ORDER BY api_key_id) TO STDOUT WITH CSV;')
        if expected is None:
            expected = actual
        assert expected == actual, 'cost/token aggregation changed'
        db['samples'][stage] = []
        for _ in range(3):
            plan = json.loads(sql('EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) ' + query))[0]
            db['samples'][stage].append(plan)
        if stage == 'candidate':
            db['covering_index_bytes'] = int(sql("SELECT pg_relation_size('idx_usage_logs_api_key_created_cost');"))
    db['result_sha256'] = hashlib.sha256(expected.encode()).hexdigest()
    samples['postgres'] = db
finally:
    subprocess.run(['docker', 'rm', '-f', container], check=False, stdout=subprocess.DEVNULL)

(ROOT / 'PERFORMANCE.json').write_text(json.dumps(samples, indent=2) + '\n')
print(json.dumps({'postgres': db}), flush=True)
