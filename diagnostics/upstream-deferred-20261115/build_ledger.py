#!/usr/bin/env python3
"""Freeze historical decisions and verify them without replaying source patches."""
import collections
import hashlib
import json
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parent
repo = root.parents[1]
historical = json.loads((root / "HISTORICAL_INPUT.json").read_text())
groups = json.loads((root / "RESOLUTIONS.json").read_text())
decisions = {}
for group in groups:
    for pr in group["prs"]:
        assert pr not in decisions, f"duplicate decision #{pr}"
        decisions[pr] = group
    for name in group["owner_files"]:
        assert (repo / name).is_file(), name
assert set(decisions) == {item["pr"] for item in historical}, "historical coverage changed"

entries = []
for item in historical:
    pr, sha = item["pr"], item["merge_sha"]
    group = decisions[pr]
    patch = subprocess.check_output(["git", "diff", sha + "^1", sha], cwd=repo)
    title = subprocess.check_output(["git", "show", "-s", "--format=%b", sha], cwd=repo, text=True).strip()
    entries.append({
        "pr": pr,
        "url": f"https://github.com/Wei-Shaw/sub2api/pull/{pr}",
        "merge_sha": sha,
        "upstream_diff_sha256": hashlib.sha256(patch).hexdigest(),
        "title": title,
        "status": group["status"],
        "diff_review_closed": True,
        "capability_complete": group["capability_complete"],
        "decision": group["decision"],
        "current_owners": group["owner_files"],
        "historical_references": item["references"],
        "reopen_when": "Frozen upstream semantics change, a concrete current-owner regression is demonstrated, or the explicitly unimplemented capability is commissioned with a new design. A repeated frozen diff is not a reopen trigger.",
    })

counts = collections.Counter(entry["status"] for entry in entries)
ledger = {
    "baseline": "39ea4774671a5035ea996ef0553a8b31be67fb3b",
    "upstream_head": "530c5e3a4f9c73ca2b0881191d0ef8e0d82c5484",
    "meaning": "Closing a frozen diff review is distinct from implementing its capability. capability_complete=false is explicit non-adoption or retained local scope, never a claim of functional parity.",
    "counts": dict(sorted(counts.items())),
    "entries": entries,
}
(root / "UPSTREAM_PRS.json").write_text(json.dumps(ledger, ensure_ascii=False, indent=2) + "\n")
print(json.dumps({"entries": len(entries), "counts": counts}, ensure_ascii=False))
