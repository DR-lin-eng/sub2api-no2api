#!/usr/bin/env python3
"""Record verification output and exit status without losing failed test names."""
import datetime
import gzip
import hashlib
import json
import pathlib
import re
import subprocess
import sys

name, *command = sys.argv[1:]
root = pathlib.Path(__file__).resolve().parent
records = root / 'records'
records.mkdir(exist_ok=True)
started = datetime.datetime.now(datetime.timezone.utc).isoformat()
result = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
output = result.stdout
archive = records / (name + '.log.gz')
archive.write_bytes(gzip.compress(output, mtime=0))
text_output = output.decode('utf-8', errors='replace')
lines = text_output.splitlines()
failures = []
for index, line in enumerate(lines):
    if re.search(r'--- FAIL:|^FAIL\s|error:|Error:|✖', line):
        failures.append('\n'.join(lines[max(0, index-1):index+12]))
record = {'command': command, 'started_at': started,
          'finished_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
          'exit_code': result.returncode, 'output_sha256': hashlib.sha256(output).hexdigest(),
          'output_bytes': len(output), 'output': archive.name,
          'failures': failures[:15], 'tail': '\n'.join(lines[-12:])}
(records / (name + '.json')).write_text(json.dumps(record, indent=2, ensure_ascii=False) + '\n')
print(json.dumps({k:v for k,v in record.items() if k != 'command'}, indent=2, ensure_ascii=False))
sys.exit(result.returncode)
