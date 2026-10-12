#!/usr/bin/env python3
"""Reconstruct, rollback, and reapply this task's immutable source roles."""
import hashlib
import io
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile

ROLE = Path(__file__).resolve().parent
ROOT = ROLE.parents[1]
baseline = (ROLE / 'BASELINE_SHA.txt').read_text().strip()
manifest = json.loads((ROLE / 'SOURCE_HASHES.json').read_text())
copy = Path(tempfile.mkdtemp(prefix='sub2api-sync-20261012-verified-copy-', dir='/private/tmp'))
archive = subprocess.check_output(['git', 'archive', baseline], cwd=ROOT)
with tarfile.open(fileobj=io.BytesIO(archive)) as source:
    source.extractall(copy, filter='data')

def check(label, expected, absent=()):
    for path, sha in expected.items():
        actual = hashlib.sha256((copy / path).read_bytes()).hexdigest()
        if actual != sha:
            raise RuntimeError(label + ' mismatch: ' + path)
    for path in absent:
        if (copy / path).exists():
            raise RuntimeError(label + ' unexpected added path: ' + path)
    print(label + '_HASH_PASS count=' + str(len(expected)), flush=True)

check('BASELINE', manifest['baseline'], manifest['added'])
subprocess.run(['git', 'apply', '--check', str(ROLE / 'DIFF_FILE')], cwd=copy, check=True)
subprocess.run(['git', 'apply', str(ROLE / 'DIFF_FILE')], cwd=copy, check=True)
check('MODIFIED', manifest['modified'])
with tarfile.open(ROLE / 'MODIFIED_FILE') as source:
    for member in source:
        data = source.extractfile(member).read()
        if hashlib.sha256(data).hexdigest() != manifest['modified'][member.name]:
            raise RuntimeError('MODIFIED_FILE archive mismatch')
subprocess.run([str(ROLE / 'ROLLBACK.sh'), str(copy)], check=True)
check('ROLLBACK', manifest['baseline'], manifest['added'])
subprocess.run(['git', 'apply', str(ROLE / 'DIFF_FILE')], cwd=copy, check=True)
check('REAPPLIED', manifest['modified'])
(ROLE / 'VERIFIED_COPY.txt').write_text(str(copy) + '\n')
print('VERIFIED_COPY=' + str(copy))
