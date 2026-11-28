"""Verify the 2026-11-27 upstream review ledger."""
import argparse
import hashlib
import json
import re
import subprocess
from collections import Counter
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
LEDGER = json.loads((HERE / "UPSTREAM_PRS.json").read_text())

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument(
    "--require-ancestry",
    action="store_true",
    help="also require the reviewed upstream head to be an ancestor of HEAD",
)
args = parser.parse_args()


def git(*git_args):
    return subprocess.check_output(["git", *git_args], cwd=ROOT)


entries = LEDGER["entries"]
commits = (
    git("log", "--reverse", "--first-parent", "--format=%H", LEDGER["upstream_base"] + ".." + LEDGER["upstream_head"])
    .decode()
    .splitlines()
)
assert commits == [entry["merge_sha"] for entry in entries], "first-parent commit list mismatch"
for entry, sha in zip(entries, commits):
    parent = sha + "^1"
    subject = git("show", "-s", "--format=%s", sha).decode().strip()
    pr = re.match(r"^Merge pull request #(\d+)\b", subject)
    assert entry["pr"] == (int(pr[1]) if pr else None), f"PR mismatch on {sha}"
    files = git("diff", "--name-only", parent, sha).decode().splitlines()
    assert entry["files"] == files, f"file list mismatch on {sha}"
    diff = git("diff", "--no-ext-diff", "--no-color", "--abbrev=9", parent, sha)
    assert entry["diff_sha256"] == hashlib.sha256(diff).hexdigest(), f"diff hash mismatch on {sha}"
    assert entry["diff_review_closed"] is True
    assert isinstance(entry["capability_complete"], bool)
    assert entry["current_owners"] and all((ROOT / owner).exists() for owner in entry["current_owners"]), (
        f"missing owner on #{entry['pr']}"
    )
    assert entry["decision"] and entry["reopen_when"]
for inherited in LEDGER["inherited_ledgers"]:
    raw = (ROOT / inherited["path"]).read_bytes()
    assert hashlib.sha256(raw).hexdigest() == inherited["sha256"], f"inherited ledger hash mismatch: {inherited['path']}"
assert Counter(entry["status"] for entry in entries) == LEDGER["counts"], "status counts mismatch"
if args.require_ancestry:
    git("merge-base", "--is-ancestor", LEDGER["upstream_head"], "HEAD")
    print(f"Upstream {LEDGER['upstream_head']} is an ancestor of HEAD")
print(f"{len(entries)} upstream first-parent patches verified")
