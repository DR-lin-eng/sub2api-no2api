#!/usr/bin/env python3
"""Verify the immutable first-parent inventory, patch hashes and inherited decisions."""
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
assert len(entries) == len({e['pr'] for e in entries}) == 61
late = json.loads((ROOT / 'LATE_UPSTREAM_INPUT.json').read_text())
assert late['new_frozen_head'] == ledger['upstream_head']
assert {e['pr'] for e in late['entries']} == {8746, 8827, 8822, 8816, 8833}
inherited = REPO / ledger['inherited_ledger']
assert hashlib.sha256(inherited.read_bytes()).hexdigest() == ledger['inherited_ledger_sha256']
actual = subprocess.check_output(['git', 'log', '--reverse', '--first-parent', '--format=%H',
    ledger['upstream_base'] + '..' + ledger['upstream_head']], cwd=REPO, text=True).splitlines()
assert actual == [e['merge_sha'] for e in entries]
for entry in entries:
    patch = subprocess.check_output(['git', 'diff', '--no-ext-diff', entry['merge_sha'] + '^1', entry['merge_sha']], cwd=REPO)
    assert hashlib.sha256(patch).hexdigest() == entry['diff_sha256'], entry['pr']
    assert entry['diff_review_closed'] is True and isinstance(entry['capability_complete'], bool)
    assert entry['decision'] and entry['reopen_when'] and entry['current_owners']
    assert all((REPO / owner).exists() for owner in entry['current_owners'])
assert dict(sorted(Counter(e['status'] for e in entries).items())) == ledger['counts']
if '--require-ancestry' in sys.argv:
    subprocess.run(['git', 'merge-base', '--is-ancestor', ledger['upstream_head'], 'HEAD'], cwd=REPO, check=True)
print(json.dumps({'frozen_prs': 61, 'diff_decisions_closed': 61,
    'capability_incomplete': [e['pr'] for e in entries if not e['capability_complete']],
    'inherited_ledger': 'unchanged', 'first_parent_inventory': 'PASS', 'patch_sha256': 'PASS'}))
