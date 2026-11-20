"""Verify the reviewed range, inherited decisions and the optional tracking merge."""
import hashlib
import json
from collections import Counter
from pathlib import Path
import re
import subprocess
import sys

here = Path(__file__).resolve().parent
repo = here.parents[1]
ledger = json.loads((here / "UPSTREAM_PRS.json").read_text())


def git(*args):
    return subprocess.check_output(["git", *args], cwd=repo)


entries = ledger["entries"]
commits = git("log", "--reverse", "--first-parent", "--format=%H",
              ledger["upstream_base"] + ".." + ledger["upstream_head"]).decode().splitlines()
assert commits == [e["merge_sha"] for e in entries]
historical = set()
for previous in ledger["inherited_ledgers"]:
    raw = (repo / previous["path"]).read_bytes()
    assert hashlib.sha256(raw).hexdigest() == previous["sha256"]
    historical.update(e["pr"] for e in json.loads(raw)["entries"] if e["pr"] is not None)
new = set()
for sha, entry in zip(commits, entries):
    subject = git("show", "-s", "--format=%s", sha).decode().strip()
    pr = re.search(r"^Merge pull request #(\d+)\b", subject)
    assert entry["pr"] == (int(pr[1]) if pr else None)
    if pr:
        assert entry["pr"] not in historical | new
        new.add(entry["pr"])
    assert entry["files"] == git("diff", "--name-only", sha + "^1", sha).decode().splitlines()
    assert entry["diff_sha256"] == hashlib.sha256(git("diff", "--no-ext-diff", sha + "^1", sha)).hexdigest()
    assert entry["diff_review_closed"] is True and isinstance(entry["capability_complete"], bool)
    assert entry["current_owners"] and all((repo / owner).exists() for owner in entry["current_owners"])
    assert entry["decision"] and entry["reopen_when"]
assert Counter(e["status"] for e in entries) == ledger["counts"]
assert len(new) == 39 and len(entries) == 41 and len(historical) == 276
if "--require-ancestry" in sys.argv:
    git("merge-base", "--is-ancestor", ledger["upstream_head"], "HEAD")
print(f"{len(new)} PRs, {len(entries)} patches, {len(historical)} historical PRs verified")
