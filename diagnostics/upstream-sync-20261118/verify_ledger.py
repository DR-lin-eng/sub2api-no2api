#!/usr/bin/env python3
"""Validate the frozen increment without reopening inherited review decisions."""
from collections import Counter
import hashlib
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parents[1]
ledger = json.loads((ROOT / 'UPSTREAM_PRS.json').read_text())
entries = ledger['entries']
prs = {entry['pr'] for entry in entries if entry['pr'] is not None}
assert prs == {8906, 8904, 8903, 8598, 8914, 8577, 8174, 8920} and len(entries) == 9
inherited_prs = set()
inherited_entries = 0
for item in ledger['inherited_ledgers']:
    raw = (REPO / item['path']).read_bytes()
    assert hashlib.sha256(raw).hexdigest() == item['sha256']
    previous = json.loads(raw)['entries']
    inherited_entries += len(previous)
    inherited_prs.update(e['pr'] for e in previous if e['pr'] is not None)
assert len(inherited_prs) == 268 and inherited_entries == 270
assert not prs.intersection(inherited_prs)
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
print(json.dumps({'new_prs': len(prs), 'direct_commits': 1, 'decisions_closed': len(entries),
    'inherited_prs_unchanged': len(inherited_prs), 'patch_hashes': 'PASS'}))
