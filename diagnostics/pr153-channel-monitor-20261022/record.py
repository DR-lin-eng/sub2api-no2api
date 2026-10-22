import datetime
import json
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parent
label, *command = sys.argv[1:]
start = datetime.datetime.now(datetime.timezone.utc).isoformat()
with (root / 'records' / (label + '.stdout')).open('wb') as out, (root / 'records' / (label + '.stderr')).open('wb') as err:
    result = subprocess.run(command, stdout=out, stderr=err)
record = {'label': label, 'command': command, 'started_at': start, 'finished_at': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'exit_status': result.returncode}
(root / 'records' / (label + '.json')).write_text(json.dumps(record, indent=2) + '\n')
with (root / 'VERIFICATION.txt').open('a') as log:
    log.write('\n' + json.dumps(record) + '\nSTDOUT:\n')
    log.write((root / 'records' / (label + '.stdout')).read_text(errors='replace'))
    log.write('\nSTDERR:\n')
    log.write((root / 'records' / (label + '.stderr')).read_text(errors='replace'))
print(json.dumps(record))
print((root / 'records' / (label + '.stdout')).read_text(errors='replace')[-2500:])
print((root / 'records' / (label + '.stderr')).read_text(errors='replace')[-1000:])
raise SystemExit(result.returncode)
