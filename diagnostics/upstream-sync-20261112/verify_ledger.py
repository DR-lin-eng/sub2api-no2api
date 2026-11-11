#!/usr/bin/env python3
"""Validate frozen PR inventory and raw merge patch hashes without network access."""
from pathlib import Path
import hashlib
import json
import subprocess

ROOT = Path(__file__).resolve().parent
ledger = json.loads((ROOT / 'UPSTREAM_PRS.json').read_text())
entries = ledger['entries']
assert len(entries) == 33 and len({e['pr'] for e in entries}) == 33
assert sum(bool(e['fully_closed']) for e in entries) == 30
assert {e['pr'] for e in entries if not e['fully_closed']} == {8606, 8358, 8639}
for entry in [*entries, *ledger['direct_commits']]:
    sha = entry.get('merge_sha', entry.get('sha'))
    diff = subprocess.check_output(['git', 'diff', sha + '^1', sha])
    assert hashlib.sha256(diff).hexdigest() == entry['diff_sha256'], sha
for entry in entries:
    assert entry['reviewed'] and entry['owner'] and entry['decision'] and entry['reopen_when']

commits = subprocess.check_output(['git', 'log', '--first-parent', '--format=%H%x09%s',
    ledger['upstream_base'] + '..' + ledger['upstream_head']], text=True).splitlines()
expected = {e['merge_sha'] for e in entries} | {e['sha'] for e in ledger['direct_commits']}
assert {line.split('\t', 1)[0] for line in commits} == expected
print(json.dumps({'frozen_prs':len(entries), 'fully_closed':30, 'partial_or_deferred':[8606,8358,8639],
    'direct_commits':len(ledger['direct_commits']), 'raw_diff_hashes':'PASS', 'first_parent_inventory':'PASS'}))
