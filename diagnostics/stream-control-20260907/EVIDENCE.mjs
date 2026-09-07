import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { execFileSync, spawnSync } from 'node:child_process';

const directory = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(directory, '../..');
const digest = data => crypto.createHash('sha256').update(data).digest('hex');
const hash = file => digest(fs.readFileSync(file));
const added = [
  'backend/internal/application/service/openai_response_header_keepalive.go',
  'backend/internal/application/service/openai_stream_control_test.go',
  'backend/internal/application/service/openai_stream_pool_budget_test.go',
  'backend/internal/application/service/testdata/openai_control_failure.sse',
  'backend/internal/transport/http/handler/openai_stream_pool_recovery_test.go',
];
const probes = [
  added[1], added[3], added[4],
  'backend/internal/transport/http/handler/openai_responses_failover_cancel_test.go',
];
const mode = process.argv[2];
if (mode === 'package') {
  const tracked = execFileSync('git', ['diff', '--name-only'], { cwd: root, encoding: 'utf8' }).trim().split('\n');
  const records = tracked.concat(added).map(file => {
    const original = path.join(directory, 'original', file);
    const old = fs.existsSync(original) ? fs.readFileSync(original) : null;
    if (old !== null) {
      const committed = execFileSync('git', ['show', `HEAD:${file}`], { cwd: root });
      if (digest(committed) !== digest(old)) throw new Error(`Original mismatch: ${file}`);
    } else if (!added.includes(file)) throw new Error(`Missing original: ${file}`);
    return { file, original: old === null ? null : digest(old), modified: hash(path.join(root, file)) };
  });
  fs.writeFileSync(path.join(directory, 'MANIFEST.json'), JSON.stringify({
    base: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8' }).trim(),
    records,
  }, null, 2) + '\n');
  let diff = execFileSync('git', ['diff', '--binary', '--', ...tracked], { cwd: root, encoding: 'utf8' });
  for (const file of added) {
    const result = spawnSync('git', ['diff', '--no-index', '--', '/dev/null', file], { cwd: root, encoding: 'utf8' });
    if (result.status !== 1) throw new Error(`Diff failed: ${file}`);
    diff += result.stdout;
  }
  fs.writeFileSync(path.join(directory, 'DIFF_FILE'), diff);
  fs.copyFileSync(path.join(root, 'backend/internal/transport/http/handler/openai_gateway_responses.go'), path.join(directory, 'MODIFIED_FILE'));
  console.log(`PACKAGED files=${records.length} original_hashes=PASS modified_hashes=PASS`);
} else if (mode === 'probe') {
  const target = fs.realpathSync(process.argv[3]);
  if (target === root) throw new Error('Probe requires an independent copy');
  for (const file of probes) {
    fs.mkdirSync(path.dirname(path.join(target, file)), { recursive: true });
    fs.copyFileSync(path.join(root, file), path.join(target, file));
  }
  console.log('PROBE tests=installed production=unchanged');
} else if (mode === 'rollback') {
  const target = fs.realpathSync(process.argv[3]);
  const { records } = JSON.parse(fs.readFileSync(path.join(directory, 'MANIFEST.json'), 'utf8'));
  for (const record of records) {
    const file = path.join(target, record.file);
    if (!fs.lstatSync(file).isFile() || hash(file) !== record.modified) throw new Error(`Modified hash mismatch: ${record.file}`);
    if (record.original !== null && hash(path.join(directory, 'original', record.file)) !== record.original) throw new Error(`Original hash mismatch: ${record.file}`);
  }
  for (const record of records) {
    const file = path.join(target, record.file);
    if (record.original === null) fs.unlinkSync(file);
    else fs.copyFileSync(path.join(directory, 'original', record.file), file);
  }
  for (const record of records) {
    const file = path.join(target, record.file);
    if (record.original === null ? fs.existsSync(file) : hash(file) !== record.original) throw new Error(`Restore failed: ${record.file}`);
  }
  console.log(`ROLLBACK restored=${records.length} original_hashes=PASS added_files=absent`);
} else if (mode === 'report') {
  const manifest = JSON.parse(fs.readFileSync(path.join(directory, 'MANIFEST.json'), 'utf8'));
  for (const record of manifest.records) {
    if (hash(path.join(root, record.file)) !== record.modified) throw new Error(`Source changed: ${record.file}`);
  }
  const baseline = fs.readFileSync(path.join(directory, 'BASELINE.txt'), 'utf8');
  const modified = fs.readFileSync(path.join(directory, 'MODIFIED.txt'), 'utf8');
  const rollback = fs.readFileSync(path.join(directory, 'ROLLBACK.txt'), 'utf8');
  const regression = fs.readFileSync(path.join(directory, 'REGRESSION.txt'), 'utf8');
  const race = fs.readFileSync(path.join(directory, 'RACE.txt'), 'utf8');
  if (!baseline.includes('BASELINE exit=1') || !rollback.includes('ROLLBACK exit=1') ||
      !modified.includes('MODIFIED exit=0')) throw new Error('Incomplete phase results');
  for (const output of [baseline, rollback]) {
    if (output.includes('[build failed]') || output.includes('signal: killed')) throw new Error('Not a behavioral failure');
  }
  for (const output of [regression, race]) {
    if ((output.match(/^ok\s+/gm) || []).length !== 2 || /^FAIL/m.test(output)) throw new Error('Incomplete regression');
  }
  const verify = path.join(directory, 'VERIFY.sh');
  const rollbackCommand = `${path.join(directory, 'ROLLBACK.sh')} /private/tmp/sub2api-stream-control-rollback-20260907`;
  const baseImage = 'sha256:64e6355fc108df54c0f9751c91a24b593dceebf01cebebe2f2e5658849e8fa4a';
  const docker = `docker run --rm --network none --entrypoint go -e GOMAXPROCS=2 -e GOGC=20 -e GOMEMLIMIT=1200MiB -v sub2api-stream-control-gobuild-20260907:/root/.cache/go-build -v ${root}/backend:/app/backend:ro -w /app/backend`;
  let report = `Changed branch/field: detached HEAD ${manifest.base}; recoverStreamPool; poolAccount/poolUsed; openai_stream_last_write; codex.rate_limits/codex.response.metadata preamble; type:error/server_error classification; response.failed event boundary.\n`;
  report += 'Behavior: before semantic output, preserve one downstream HTTP connection, suppress intermediate retryable errors, and try distinct eligible accounts until success or selection exhausts the pool.\n';
  report += 'Per-account transport attempts remain capped at four and never refill on reselection. Client cancellation, deterministic request/policy failures, non-replayable request checks, and post-semantic-output replay prohibition remain authoritative. No full-response buffering was enabled.\n';
  report += 'Heartbeat spans header/body/attempt transitions. Failed response stops at its complete SSE boundary; usage in error + response.failed pairs is retained.\n';
  report += 'Original attachment unchanged SHA-256: c026b0f0cdd88d12abd591fcfe674121d9979f486dd1493550fad71ea1cdbd59\n';
  report += 'Test fixture contains synthetic identifiers only. Actual attachment credentials and turn state are not copied into the repository.\n';
  report += 'Official event reference: https://developers.openai.com/api/reference/resources/responses/streaming-events#response.failed\n';
  for (const name of ['MODIFIED_FILE', 'DIFF_FILE', 'VERIFICATION.txt', 'ROLLBACK.sh']) report += `${name}: ${path.join(directory, name)}\n`;
  report += `\nBASELINE command: ${verify} BASELINE /private/tmp/sub2api-stream-control-baseline-20260907\nInput: HEAD archive + behavioral test probes (no production edits); test-only budget assertions require new API and are modified-only.\nExit: 1 (expected regression failures).\nLiteral output:\n${baseline}`;
  report += `\nMODIFIED command: ${verify} MODIFIED ${root}\nInput: current modified source and sanitized fixture.\nExit: 0.\nLiteral output:\n${modified}`;
  report += `\nROLLBACK command: ${rollbackCommand}\nInput: independent copy of modified backend/docs; hash preflight applies to all 14 files.\nExit: 0.\nLiteral output:\n${fs.readFileSync(path.join(directory, 'ROLLBACK_RESULT.txt'), 'utf8')}`;
  report += `\nROLLBACK behavior command: ${verify} ROLLBACK /private/tmp/sub2api-stream-control-rollback-20260907\nInput: restored production source + reinjected behavioral probes only.\nExit: 1 (expected, original failures restored).\nLiteral output:\n${rollback}`;
  report += `\nREGRESSION command: ${docker} sub2api-cpa-backend-test:20260906 test -tags=unit -p 1 ./internal/application/service ./internal/transport/http/handler -count=1 -timeout=600s\nInput: final current source. Exit: 0. Literal output:\n${regression}`;
  report += `\nRACE command: docker run --rm --network none --entrypoint go -e GOMAXPROCS=2 -e GOGC=20 -e GOMEMLIMIT=1200MiB -e CGO_ENABLED=1 -v sub2api-stream-control-gobuild-20260907:/root/.cache/go-build -v ${root}/backend:/app/backend:ro -w /app/backend sub2api-stream-control-test:20260907 test -race -tags=unit -p 1 ./internal/application/service ./internal/transport/http/handler -run '^TestOpenAIStream(Control|PoolRecovery)|^TestOpenAIGatewayHandlerResponses_FailoverAbortsWhenClientDisconnected$|^TestOpenAIResponseFlush_KeepaliveDoesNotSplitOpenEvent$|^TestOpenAIStreamingTimeout$' -count=1 -timeout=300s\nInput: final current source. Exit: 0. Literal output:\n${race}`;
  report += `\nDocker base image: ${baseImage}\nDocker race image: sha256:3250446107c8abf24a19a644a22ce7ddfa1761a9de4c3a8ea8bb7cfc0c9500cf\nToolchain: go version go1.26.6 linux/arm64\n`;
  report += 'Checks: make check-docs -> Documentation checks passed. (exit 0); ./backend/scripts/check-source-layout.sh -> empty output (exit 0); git diff --check -> empty output (exit 0).\n';
  report += 'Earlier attempts encountered a stale test symbol during concurrent edits and memory-limited compilation. A full-suite run also found two overly broad conditions (semantic open-event keepalive and non-OpenAI timeout); both were narrowed and the full suite rerun. Final recorded phases are fixed-source reruns with no build failures.\n';
  report += 'Restored behavior/status: old control-frame classification, early pool termination and failure/heartbeat continuation reproduced only in independent rollback copy. Current worktree and MODIFIED_FILE remain modified. No commit, push, or deployment performed.\n';
  report += `\nOriginal/modified hashes:\n${JSON.stringify(manifest, null, 2)}\n`;
  fs.writeFileSync(path.join(directory, 'VERIFICATION.txt'), report);
  for (const name of ['MODIFIED_FILE', 'DIFF_FILE', 'VERIFICATION.txt', 'ROLLBACK.sh']) {
    const bytes = fs.readFileSync(path.join(directory, name));
    console.log(`REOPENED ${name} bytes=${bytes.length} sha256=${digest(bytes)}`);
  }
} else throw new Error('Expected package, probe, rollback, or report');
