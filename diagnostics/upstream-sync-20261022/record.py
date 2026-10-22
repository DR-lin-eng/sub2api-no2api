#!/usr/bin/env python3
"""Keep literal command evidence outside the source checkout."""
import datetime
import json
from pathlib import Path
import subprocess
import sys
import time

root = Path(__file__).resolve().parent
label, cwd, *command = sys.argv[1:]
started = datetime.datetime.now(datetime.timezone.utc).isoformat()
tick = time.monotonic()
directory = root / "records"
directory.mkdir(exist_ok=True)
path = directory / (label + ".record.json")
if path.exists():
    raise SystemExit("Evidence label already exists: " + str(path))
out_path, err_path = directory / (label + ".stdout"), directory / (label + ".stderr")
with out_path.open("w") as out, err_path.open("w") as err:
    code = subprocess.run(command, cwd=cwd, stdout=out, stderr=err).returncode
result = subprocess.CompletedProcess(command, code, out_path.read_text(), err_path.read_text())
record = {
    "command": command,
    "cwd": str(Path(cwd).resolve()),
    "started_at": started,
    "elapsed_seconds": round(time.monotonic() - tick, 3),
    "exit_status": result.returncode,
    "stdout": result.stdout,
    "stderr": result.stderr,
}
directory = root / "records"
directory.mkdir(exist_ok=True)
path = directory / (label + ".record.json")
if path.exists():
    raise SystemExit("Evidence label already exists: " + str(path))
path.write_text(json.dumps(record, indent=2, ensure_ascii=False) + "\n")
print(json.dumps({"record": str(path), "exit_status": result.returncode,
                  "elapsed_seconds": record["elapsed_seconds"]}, ensure_ascii=False))
for name in ("stdout", "stderr"):
    if record[name]:
        print(name + ":\n" + record[name][-2000:])
sys.exit(result.returncode)
