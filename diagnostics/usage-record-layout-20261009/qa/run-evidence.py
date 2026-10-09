from pathlib import Path
import datetime
import json
import subprocess
import sys

state = json.loads(Path('/Users/lin/.codex/worktrees/142f/sub2api-no2api/diagnostics/usage-record-layout-20261009/STATE.json').read_text())
evidence = Path(state['EVIDENCE'])
label = sys.argv[1]
command = sys.argv[2:]
result = subprocess.run(command, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
(evidence / f'{label}.stdout.txt').write_text(result.stdout)
(evidence / f'{label}.stderr.txt').write_text(result.stderr)
with (evidence / 'VERIFICATION.txt').open('a') as log:
    log.write(f'\nEVENT={label}\nUTC={datetime.datetime.now(datetime.timezone.utc).isoformat()}\n')
    log.write('COMMAND_JSON=' + json.dumps(command) + '\nINPUT=' + (str(evidence / 'PUBLICATION_INPUT.json') if label.startswith('PUB_') else state['INPUT']) + '\nSTDOUT_BEGIN\n' + result.stdout + 'STDOUT_END\nSTDERR_BEGIN\n' + result.stderr + 'STDERR_END\nEXIT_STATUS=' + str(result.returncode) + '\n')
print(result.stdout, end='')
print(result.stderr, file=sys.stderr, end='')
print(label + '_EXIT_STATUS=' + str(result.returncode))
sys.exit(result.returncode)
