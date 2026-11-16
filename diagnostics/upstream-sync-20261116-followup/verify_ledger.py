#!/usr/bin/env python3
"""Verify frozen main inventory and hashes without reopening inherited reviews."""
from pathlib import Path
from collections import Counter
import hashlib
import json
import subprocess
import sys

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parents[1]
ledger = json.loads((ROOT / 'UPSTREAM_PRS.json').read_text())
entries = ledger['entries']
prs = [entry['pr'] for entry in entries if entry['pr'] is not None]
assert len(prs) == len(set(prs)) == 7
assert len(entries) == 8
inherited_prs = set()
for inherited in ledger['inherited_ledgers']:
    raw = (REPO / inherited['path']).read_bytes()
    assert hashlib.sha256(raw).hexdigest() == inherited['sha256']
    inherited_prs.update(entry['pr'] for entry in json.loads(raw)['entries'])
assert not inherited_prs.intersection(prs)
actual = subprocess.check_output(['git', 'log', '--reverse', '--first-parent', '--format=%H',
    ledger['upstream_base'] + '..' + ledger['upstream_head']], cwd=REPO, text=True).splitlines()
assert actual == [entry['merge_sha'] for entry in entries]
for entry in entries:
    patch = subprocess.check_output(['git', 'diff', '--no-ext-diff', entry['merge_sha'] + '^1', entry['merge_sha']], cwd=REPO)
    assert hashlib.sha256(patch).hexdigest() == entry['diff_sha256']
    assert entry['diff_review_closed'] is True
    assert isinstance(entry['capability_complete'], bool)
    assert entry['decision'] and entry['reopen_when'] and entry['current_owners']
    assert all((REPO / path).exists() for path in entry['current_owners'])
assert dict(sorted(Counter(entry['status'] for entry in entries).items())) == ledger['counts']
if '--require-ancestry' in sys.argv:
    subprocess.run(['git', 'merge-base', '--is-ancestor', ledger['upstream_head'], 'HEAD'], cwd=REPO, check=True)
print(json.dumps({'new_prs': 7, 'direct_commits': 1, 'decisions_closed': 8,
    'inherited_prs_unchanged': 259, 'patch_hashes': 'PASS',
    'capability_incomplete': [entry['pr'] or entry['merge_sha'] for entry in entries if not entry['capability_complete']]}))
