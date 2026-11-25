"""Verify the 2026-11-25 open upstream PR review ledger."""
import hashlib
import json
import subprocess
from collections import Counter
from pathlib import Path
HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
LEDGER = json.loads((HERE / "UPSTREAM_PRS.json").read_text())
def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT)
for entry in LEDGER["entries"]:
    head = entry["head_sha"]
    assert git("rev-parse", head).decode().strip() == head
    merge_base = entry["merge_base"]
    files = git("diff", "--name-only", merge_base, head).decode().splitlines()
    assert entry["files"] == files
    diff = git("diff", "--no-ext-diff", merge_base, head)
    assert entry["diff_sha256"] == hashlib.sha256(diff).hexdigest()
    assert entry["diff_review_closed"] is True
    assert isinstance(entry["capability_complete"], bool)
    assert entry["current_owners"] and all((ROOT / owner).exists() for owner in entry["current_owners"])
    assert entry["decision"] and entry["performance"] and entry["upgrade"] and entry["reopen_when"]
for inherited in LEDGER["inherited_ledgers"]:
    raw = (ROOT / inherited["path"]).read_bytes()
    assert hashlib.sha256(raw).hexdigest() == inherited["sha256"]
assert Counter(entry["status"] for entry in LEDGER["entries"]) == LEDGER["counts"]
print(f"{len(LEDGER['entries'])} open upstream PR patches verified")
