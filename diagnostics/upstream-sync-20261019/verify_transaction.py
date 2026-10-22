#!/usr/bin/env python3
"""Reconstruct, roll back, execute, and reapply the four-role source artifact."""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile

ROOT = Path(__file__).resolve().parent
COPY = Path(sys.argv[1]).resolve() if len(sys.argv) > 1 else ROOT / 'rollback-final'
STATE = json.loads((ROOT / 'SOURCE_TRANSACTION.json').read_text())
COPY.mkdir(exist_ok=False)
if (ROOT / 'BASELINE_REPOSITORY.tar').exists():
    with tarfile.open(ROOT / 'BASELINE_REPOSITORY.tar') as archive:
        archive.extractall(COPY, filter='data')
else:
    repo = ROOT.parents[1]
    with subprocess.Popen(['git', 'archive', STATE['baseline_sha']], cwd=repo, stdout=subprocess.PIPE) as process:
        with tarfile.open(fileobj=process.stdout, mode='r|') as archive:
            archive.extractall(COPY, filter='data')
        assert process.wait() == 0


def record(label, command):
    subprocess.run([sys.executable, str(ROOT / 'record.py'), label, str(COPY), *command], check=True)


def check_hashes(hashes):
    assert all(hashlib.sha256((COPY / k).read_bytes()).hexdigest() == v for k, v in hashes.items())


def probe(label):
    shutil.copy2(ROOT / 'probes/upstream_sync_probe_test.go', COPY / 'backend/internal/application/service/zz_upstream_sync_probe_test.go')
    record(label, ['docker', 'run', '--rm', '-e', 'GOMAXPROCS=2', '-e', 'GOMEMLIMIT=3GiB', '-e', 'GOFLAGS=-p=1',
        '-v', str(COPY / 'backend') + ':/app', '-v', 'sub2api-upstream-go-mod-20261016:/go/pkg/mod',
        '-v', 'sub2api-upstream-go-build-20261016:/root/.cache/go-build', '-w', '/app', 'golang:1.26.9-alpine', 'sh', '-c',
        'export PATH=/usr/local/go/bin:$PATH; go test -tags unit -count=1 -run ^TestUpstreamSync20261019Probe$ -v ./internal/application/service'])


record('source-final-patch-check', ['git', 'apply', '--check', str(ROOT / 'DIFF_FILE.patch')])
record('source-final-reconstruct', ['git', 'apply', str(ROOT / 'DIFF_FILE.patch')])
check_hashes(STATE['modified_hashes'])
probe('source-final-modified')
record('source-final-rollback-command', [str(ROOT / 'ROLLBACK.sh'), str(COPY)])
check_hashes(STATE['baseline_hashes'])
assert all(not (COPY / path).exists() for path in STATE['added_source_paths'])
(ROOT / 'ROLLBACK_HASH_VERIFIED.json').write_text(json.dumps({'copy': str(COPY), 'all_baseline_hashes_match': True,
    'all_added_paths_removed': True, 'baseline_sha': STATE['baseline_sha']}, indent=2) + '\n')
probe('source-final-rollback')
record('source-final-reapply-command', ['git', 'apply', str(ROOT / 'DIFF_FILE.patch')])
check_hashes(STATE['modified_hashes'])
(ROOT / 'REAPPLY_HASH_VERIFIED.json').write_text(json.dumps({'copy': str(COPY), 'all_modified_hashes_match': True,
    'diff_sha256': STATE['diff_sha256']}, indent=2) + '\n')
probe('source-final-reapplied')
print('Final modified, rollback, and reapplied source hashes and observed behaviors verified.')
