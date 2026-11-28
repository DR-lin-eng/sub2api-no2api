"""Verify a frozen upstream review ledger."""
import argparse
import hashlib
import json
import re
import subprocess
from collections import Counter
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument(
    "--require-ancestry",
    action="store_true",
    help="also require the reviewed upstream head to be an ancestor of HEAD",
)
parser.add_argument("--ledger", type=Path, default=HERE / "UPSTREAM_PRS.json")
args = parser.parse_args()
LEDGER = json.loads(args.ledger.read_text())


def git(*git_args):
    return subprocess.check_output(["git", *git_args], cwd=ROOT)


entries = LEDGER["entries"]
pr_numbers = [entry["pr"] for entry in entries if entry["pr"] is not None]
assert len(pr_numbers) == len(set(pr_numbers)), "duplicate PR in first-parent review"
upstream_range = LEDGER["upstream_base"] + ".." + LEDGER["upstream_head"]
if "upstream_commit_count" in LEDGER:
    assert int(git("rev-list", "--count", upstream_range)) == LEDGER["upstream_commit_count"], "commit count mismatch"
commits = (
    git("log", "--reverse", "--first-parent", "--format=%H", upstream_range)
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
