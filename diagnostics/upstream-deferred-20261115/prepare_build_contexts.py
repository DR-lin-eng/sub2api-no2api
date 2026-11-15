#!/usr/bin/env python3
"""Build contexts omit unrelated diagnostics and local dependencies."""
import io
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile

repo = Path(__file__).resolve().parents[2]
output = Path(tempfile.mkdtemp(prefix="sub2api-deferred-build-"))
baseline, candidate = output / "baseline", output / "candidate"
baseline.mkdir()
candidate.mkdir()
paths = ["backend", "frontend", "Dockerfile", ".dockerignore", "docs/legal", "deploy/docker-entrypoint.sh"]
archive = subprocess.check_output(["git", "archive", "39ea4774671a5035ea996ef0553a8b31be67fb3b", *paths], cwd=repo)
with tarfile.open(fileobj=io.BytesIO(archive)) as handle:
    handle.extractall(baseline, filter="data")
for name in ["backend", "frontend", "docs/legal"]:
    shutil.copytree(repo / name, candidate / name, ignore=shutil.ignore_patterns("node_modules", ".cache", "*.tsbuildinfo", "bin", "dist", ".DS_Store"))
for name in ["Dockerfile", ".dockerignore", "deploy/docker-entrypoint.sh"]:
    (candidate / name).parent.mkdir(exist_ok=True)
    shutil.copy2(repo / name, candidate / name)
print(output)
