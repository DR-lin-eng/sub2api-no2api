#!/bin/sh
set -eu
artifact_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec python3 - "$artifact_dir" "$@" <<'PY'
import hashlib
import json
import os
from pathlib import Path
import sys
import tempfile

if len(sys.argv) != 3:
    raise SystemExit('usage: ROLLBACK.sh TARGET_CHECKOUT')
artifacts = Path(sys.argv[1]).resolve()
target = Path(sys.argv[2]).resolve()
if target == artifacts or artifacts in target.parents:
    raise SystemExit('TARGET_MUST_BE_A_CHECKOUT')
original = json.loads((artifacts / 'ORIGINAL_HASHES.json').read_text())
modified = json.loads((artifacts / 'MODIFIED_HASHES.json').read_text())
added = json.loads((artifacts / 'ADDED_FILES.json').read_text())
restore = {}
for name, expected in original.items():
    data = (artifacts / 'original' / name).read_bytes()
    if hashlib.sha256(data).hexdigest() != expected:
        raise SystemExit('ORIGINAL_HASH_MISMATCH=' + name)
    restore[name] = data
for name, expected in modified.items():
    destination = target / name
    if destination.is_symlink() or target not in destination.resolve().parents:
        raise SystemExit('INVALID_TARGET_PATH=' + name)
    if not destination.exists() and name in added:
        continue
    current = hashlib.sha256(destination.read_bytes()).hexdigest()
    if current not in (expected, original.get(name)):
        raise SystemExit('TARGET_CHANGED_SINCE_VERIFICATION=' + name)
for name, data in restore.items():
    destination = target / name
    mode = destination.stat().st_mode & 0o777
    with tempfile.NamedTemporaryFile(dir=destination.parent, delete=False) as staged:
        staged.write(data)
        staging_path = Path(staged.name)
    staging_path.chmod(mode)
    os.replace(staging_path, destination)
for name in added:
    (target / name).unlink(missing_ok=True)
for name, expected in original.items():
    if hashlib.sha256((target / name).read_bytes()).hexdigest() != expected:
        raise SystemExit('RESTORED_HASH_MISMATCH=' + name)
catalog = target / 'backend/resources/model-pricing/model_prices_and_context_window.json'
entry = json.loads(catalog.read_text())['gpt-5.6-sol']
prices = [entry[field] * 1e6 for field in ('input_cost_per_token', 'cache_read_input_token_cost', 'cache_creation_input_token_cost', 'output_cost_per_token')]
print('ROLLBACK_RESTORED=' + str(target))
print(f'ORIGINAL_FILES_VERIFIED={len(original)} ADDED_FILES_REMOVED={len(added)}')
print('RESTORED_SOL_USD_PER_MTOK=' + '/'.join(format(v, 'g') for v in prices))
print('RESTORED_CATALOG_SHA256=' + hashlib.sha256(catalog.read_bytes()).hexdigest())
PY
