#!/usr/bin/env python3
"""Keep compact, reproducible evidence for an explicitly supplied command."""
import datetime
import gzip
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import time

name, *command = sys.argv[1:]
records = Path(__file__).resolve().parent / "records"
records.mkdir(exist_ok=True)
started = time.monotonic()
result = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
output = result.stdout
with gzip.open(records / (name + ".log.gz"), "wb") as handle:
    handle.write(output)
record = {
    "command": command,
    "exit_code": result.returncode,
    "elapsed_seconds": round(time.monotonic() - started, 3),
    "utc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
    "output_sha256": hashlib.sha256(output).hexdigest(),
}
(records / (name + ".json")).write_text(json.dumps(record, indent=2) + "\n")
print(json.dumps(record))
decoded = output.decode(errors="replace")
if '"Action"' in decoded:
    events = []
    for line in decoded.splitlines():
        try:
            events.append(json.loads(line))
        except ValueError:
            pass
    failures = [event for event in events if event.get('Action') in ('fail', 'build-fail')]
    print(json.dumps({'failed': [event.get('Test', event.get('Package', event.get('ImportPath')))
                                 for event in failures[:12]],
                      'passed_tests': sum(event.get('Action') == 'pass' and 'Test' in event
                                          for event in events)}))
    if failures:
        first = failures[0].get('Test')
        details = ''.join(event.get('Output', '') for event in events
                          if event.get('Test') == first)
        print(details[-1800:])
else:
    print(decoded[-2800:])
sys.exit(result.returncode)
