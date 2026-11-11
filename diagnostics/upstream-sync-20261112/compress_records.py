#!/usr/bin/env python3
"""Keep byte-exact command evidence compact without normalizing tool output."""
from pathlib import Path
import gzip
import hashlib
import json

records = Path(__file__).resolve().parent / 'records'
for path in records.glob('*.record.json'):
    record = json.loads(path.read_text())
    name = path.name.removesuffix('.record.json')
    for stream in ['stdout', 'stderr']:
        raw = records / (name + '.' + stream)
        packed = raw.with_suffix(raw.suffix + '.gz')
        if raw.exists():
            data = raw.read_bytes()
            assert hashlib.sha256(data).hexdigest() == record[stream + '_sha256'], raw
            packed.write_bytes(gzip.compress(data, mtime=0))
            raw.unlink()
        else:
            data = gzip.decompress(packed.read_bytes())
            assert hashlib.sha256(data).hexdigest() == record[stream + '_sha256'], packed
        record[stream + '_file'] = packed.name
    record['output_encoding'] = 'gzip; recorded SHA-256 values cover the original decompressed bytes'
    path.write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n')

# Additional standalone benchmark logs have no run_record wrapper.
for raw in [*records.glob('*.stdout'), *records.glob('*.stderr')]:
    raw.with_suffix(raw.suffix + '.gz').write_bytes(gzip.compress(raw.read_bytes(), mtime=0))
    raw.unlink()
print('Verified output digests and compressed command records.')
