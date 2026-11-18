#!/usr/bin/env python3
"""Build the frozen local main tree without modifying another checkout."""
from pathlib import Path
import json
import subprocess
import tarfile
import tempfile

root = Path(__file__).resolve().parent
repo = root.parents[1]
baseline = json.loads((root / 'UPSTREAM_REVIEW_INPUT.json').read_text())['baseline']
with tempfile.TemporaryDirectory(prefix='sub2api-sync-baseline-20261118-', dir='/private/tmp') as name:
    checkout = Path(name)
    archive = subprocess.Popen(['git', 'archive', baseline], cwd=repo, stdout=subprocess.PIPE)
    with tarfile.open(fileobj=archive.stdout, mode='r|') as source:
        source.extractall(checkout, filter='data')
    assert archive.wait() == 0
    subprocess.run(['docker', 'build', '--build-arg', 'COMMIT=' + baseline,
                    '-t', 'sub2api:sync-baseline-20261118', str(checkout)], check=True)
