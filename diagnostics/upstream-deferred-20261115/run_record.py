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
print(output.decode(errors="replace")[-5000:])
sys.exit(result.returncode)
