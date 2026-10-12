#!/usr/bin/env python3
"""Preserve literal process streams and reopen the four transaction roles."""
import hashlib
import json
from pathlib import Path
from datetime import datetime, timezone

ROLE = Path(__file__).resolve().parent
ROOT = ROLE.parents[1]
selected = [
    'baseline-probe-valid', 'final-modified-probe', 'final-rollback-probe',
    'unit-full-final', 'final-progress-and-billing', 'final-lock-integration',
    'billing-baseline-valid-benchmark', 'billing-final-matrix-valid',
    'final-native-build', 'final-runtime-complete-valid',
    'ui-baseline-final', 'ui-modified-final', 'ui-rollback-final',
    'frontend-lint-final', 'golangci-final', 'final-golangci',
    'frontend-baseline-maintenance', 'frontend-candidate-maintenance',
    'backend-layout-baseline-retry', 'backend-layout-final',
    'final-docs-closed', 'final-artifact-closed',
]
parts = [
    'UPSTREAM SYNC VERIFIED TRANSACTION 2026-10-12\n',
    'Generated UTC: ' + datetime.now(timezone.utc).isoformat() + '\n',
    'TARGET_COPY: ' + str(ROOT) + '\n',
    'ORIGINAL_WORKTREE: /Users/lin/.codex/worktrees/d660/sub2api-no2api (untouched)\n',
    'BASELINE_SHA: ' + (ROLE / 'BASELINE_SHA.txt').read_text(),
    'FROZEN_UPSTREAM_SHA: ' + (ROLE / 'UPSTREAM_SHA.txt').read_text(),
    '\nCHANGED_BRANCHES_AND_FIELDS:\n',
    'OpenAIGatewayService.isCodexImageGenerationBridgeEnabled: only OpenAI accounts bridge hosted images.\n',
    'CostInput.DisableModelLongContextPricing and openAISubscriptionLongContextExempt: actual credential and sent-model policy.\n',
    'groupRepository.Delete: direct FOR UPDATE; config writes lock live groups before settings.\n',
    'Group.I18n and groups.i18n migration 248: additive display text, old-writer preservation.\n',
    'SubscriptionProgress.GroupI18n: internal JSON-excluded display parameter, no added query.\n',
    'OpsRequestDetail.AccountName: projection from existing joined SQL, no account enumeration.\n',
    '\nOBSERVED_BEHAVIORS (same transaction_fixture.go):\n',
    'BASELINE exit=0: OpenAI/Kimi/DeepSeek/MiniMax bridge true; OAuth total=1.230000 long_context=true; API key total=1.230000 long_context=true.\n',
    'MODIFIED exit=0: OpenAI bridge true, other three false; OAuth total=0.620000 long_context=false; API key total=1.230000 long_context=true.\n',
    'ROLLBACK exit=0: original behavior restored; original source hashes match.\n',
    'Browser baseline/modified/rollback exit=0/0/0: clipping true/false/true, +2 visibility false/true/false; no horizontal overflow at 320/375/1440.\n',
    'Docker database matrix: baseline/upgraded/rollback/restored all PASS, migrations 301/302/302/302; old writer preserves translations.\n',
    'Group and single subscription progress names: default/English/default/English; admin source names remain default.\n',
    'Performance: default cost-resolution allocation remains 209 B, 9 allocs; medians 732.5 ns baseline and 659.5 ns modified. Exemption check 19.05 ns, 0 allocs. Not a gateway throughput result.\n',
    'Pre-existing limits: CreateAccountDialog.vue 1512>1500, payment_fulfillment_test.go 1201>1200, pricing_service.go 1238>1200; identical baseline/candidate failures.\n',
]
for name in selected:
    record = json.loads((ROLE / 'records' / (name + '.record.json')).read_text())
    out = (ROLE / 'records' / (name + '.stdout')).read_bytes()
    err = (ROLE / 'records' / (name + '.stderr')).read_bytes()
    for kind, data in [('stdout', out), ('stderr', err)]:
        assert hashlib.sha256(data).hexdigest() == record[kind + '_sha256'], (name, kind)
    parts.extend([
        '\nEVENT: ' + name + '\n',
        'COMMAND_JSON: ' + json.dumps(record['command'], ensure_ascii=False) + '\n',
        'CWD: ' + record['cwd'] + '\n',
        'INPUT: ' + record['input'] + '\n',
        'EXIT_STATUS: ' + str(record['exit_status']) + '\n',
        'STDOUT_SHA256: ' + record['stdout_sha256'] + '\n',
        'STDERR_SHA256: ' + record['stderr_sha256'] + '\n',
        'STDOUT_BEGIN\n', out.decode(errors='replace'), 'STDOUT_END\n',
        'STDERR_BEGIN\n', err.decode(errors='replace'), 'STDERR_END\n',
    ])
parts.append('\nOTHER_RETAINED_RECORDS (including corrected fixture/tool errors):\n')
for path in sorted((ROLE / 'records').glob('*.record.json')):
    if path.name.removesuffix('.record.json') in selected:
        continue
    record = json.loads(path.read_text())
    parts.append(path.name + ' exit=' + str(record['exit_status']) + '\n')
parts.append('\nROLE_HASHES (VERIFICATION hash is in ROLE_HASHES.json):\n')
for name in ['BASELINE_FILE', 'MODIFIED_FILE', 'DIFF_FILE', 'ROLLBACK.sh']:
    content = (ROLE / name).read_bytes()
    parts.append(str(ROLE / name) + ' bytes=' + str(len(content)) + ' sha256=' + hashlib.sha256(content).hexdigest() + '\n')
(ROLE / 'VERIFICATION.txt').write_text(''.join(parts))
hashes = {}
for name in ['MODIFIED_FILE', 'DIFF_FILE', 'VERIFICATION.txt', 'ROLLBACK.sh']:
    path = ROLE / name
    content = path.read_bytes()
    hashes[name] = {'path': str(path), 'bytes': len(content), 'sha256': hashlib.sha256(content).hexdigest()}
    print(json.dumps(hashes[name]))
(ROLE / 'ROLE_HASHES.json').write_text(json.dumps(hashes, indent=2) + '\n')
