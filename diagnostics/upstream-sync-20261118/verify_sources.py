#!/usr/bin/env python3
"""Check the source inputs used for final local verification."""
import hashlib
import json
from pathlib import Path

root = Path(__file__).resolve().parent
repo = root.parents[1]
manifest = json.loads((root / 'SOURCE_HASHES.json').read_text())
for name, expected in manifest.items():
    assert hashlib.sha256((repo / name).read_bytes()).hexdigest() == expected, name
print(json.dumps({'source_inputs': len(manifest), 'hashes': 'PASS'}))
