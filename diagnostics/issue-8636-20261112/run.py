#!/usr/bin/env python3
"""Docker-only gateway reproduction with synthetic credentials and upstreams.

Requires Python 3.14 (compression.zstd), cached Go modules, and the images in
compose.yaml. The default run builds the current backend and protocol fixture.
"""
import argparse
from compression import zstd
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import time

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parent.parent
RESULT = {}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--results', default='/private/tmp/sub2api-issue-8636-20261112')
    parser.add_argument('--skip-build', action='store_true')
    parser.add_argument('--expect-recovery', action='store_true')
    parser.add_argument('--expect-http200-recovery', action='store_true')
    parser.add_argument('--evidence-dir', type=Path, default=ROOT)
    parser.add_argument('--provider-source', type=Path, default=ROOT / 'provider.go')
    args = parser.parse_args()
    output = Path(args.results).resolve()
    output.mkdir(parents=True, exist_ok=True)

    env = {**os.environ, 'ISSUE_RESULTS': str(output),
           'ISSUE_DB_PASSWORD': secrets.token_urlsafe(24),
           'ISSUE_ADMIN_PASSWORD': 'Fixture8636!' + secrets.token_urlsafe(16),
           'ISSUE_JWT_SECRET': secrets.token_urlsafe(48),
           'ISSUE_ENCRYPTION_KEY': secrets.token_hex(32)}
    compose_cmd = ['docker', 'compose', '-f', str(ROOT / 'compose.yaml'),
                   '-p', 'sub2api-issue-8636-' + secrets.token_hex(3)]

    def command(cmd, **kwargs):
        completed = subprocess.run(cmd, env=env, capture_output=True, check=True, **kwargs)
        return completed.stdout

    def compose(*args):
        return command(compose_cmd + list(args)).decode().strip()

    def sql(statement):
        return compose('exec', '-T', 'postgres', 'psql', '-U', 'fixture', '-d', 'fixture',
                       '-v', 'ON_ERROR_STOP=1', '-qAtc', statement)

    def build(script):
        command(['docker', 'run', '--rm', '--network', 'none', '-e', 'GOTOOLCHAIN=local',
                 '-e', 'GOPROXY=off', '-e', 'GOSUMDB=off', '-e', 'CGO_ENABLED=0',
                 '-v', str(REPO / 'backend') + ':/src:ro',
                 '-v', str(args.provider_source.resolve().parent) + ':/fixture:ro', '-v', str(output) + ':/results',
                 '-v', 'sub2api-go-mod-cache:/go/pkg/mod',
                 '-v', 'sub2api-go-build-cache:/root/.cache/go-build',
                 '-w', '/src', 'golang:1.26.11-alpine', 'sh', '-ec', script])

    if not args.skip_build:
        build('go test -json -tags=unit -p 2 -count=1 -timeout=8m '
              './internal/shared/httputil ./internal/application/service '
              './internal/transport/http/server/middleware '
              '-run "Test(ReadRequestBodyWithPrealloc|CompressedBodyChecksTrailerAtExactDecodedLimit|'
              'DecodedBodyRejectsOverflowWithoutReturningTruncatedSuccess|CompressOpenAIOAuthCodexRequestBodyScope|'
              'OpenAIHTTPBuildersCompressOnlyOAuthBodies|OpenAIResponsesRejectedFieldRetryState|'
              'NormalizeOpenAIResponsesRejectedFieldRetryBodyRejectsAmbiguousErrors|GatewayModelValidation|OpenAIOAuthCompression)" '
              '> /results/unit.jsonl\n'
              'go build -buildvcs=false -p 2 -o /results/sub2api ./cmd/server')
    build('go build -buildvcs=false -o /results/provider /fixture/provider.go')
    RESULT.update(source=command(['git', '-C', str(REPO), 'rev-parse', 'HEAD']).decode().strip(),
                  backend_sha256=hashlib.sha256((output / 'sub2api').read_bytes()).hexdigest(),
                  real_upstream_tested=False, compression_recovery=args.expect_recovery,
                  http200_recovery=args.expect_http200_recovery,
                  cases=[], scope='HTTP Responses, synthetic OAuth accounts')
    def request(base, path, body=None, token=None, method=None, raw=None, headers=None):
        hdr = {'Content-Type': 'application/json', 'User-Agent': 'codex_cli_rs/0.130.0',
               'originator': 'codex_cli_rs', **(headers or {})}
        if token:
            hdr['Authorization'] = 'Bearer ' + token
        data = raw if raw is not None else None if body is None else encode(body)
        # Internal Docker networks intentionally have no host-published ports.
        # Enter the app's namespace for HTTP, without shell interpolation.
        args = ['docker', 'exec', '-i', app_container, 'curl', '--silent', '--show-error',
                '--noproxy', '*', '--max-time', '30', '--write-out', '\n%{http_code}',
                '--request', method or ('POST' if data is not None else 'GET')]
        if path.startswith('/api/v1/auth/'):
            args.extend(['--cookie', '/tmp/issue-8636-cookies',
                         '--cookie-jar', '/tmp/issue-8636-cookies'])
        for name, value in hdr.items():
            args.extend(['--header', name + ': ' + value])
        if data is not None:
            args.extend(['--data-binary', '@-'])
        args.append(base + path)
        value, _, status = command(args, input=data).decode().rpartition('\n')
        try:
            value = json.loads(value)
        except ValueError:
            pass
        return int(status), value

    def encode(value):
        return json.dumps(value, sort_keys=True, separators=(',', ':')).encode()

    def expect(status, value, label):
        assert status == 200, (label, status, str(value)[:300])
        return value['data']

    started = False
    try:
        started = True
        compose('up', '-d', 'app')
        compose('up', '-d', 'provider')
        app_container = compose('ps', '-q', 'app')
        app = 'http://127.0.0.1:8080'
        upstream = 'http://127.0.0.1:8000'
        for _ in range(100):
            try:
                if request(app, '/ready')[0] == 200 and request(upstream, '/fixture/state')[0] == 200:
                    break
            except (OSError, subprocess.CalledProcessError):
                pass
            time.sleep(.5)
        else:
            raise AssertionError('fixture failed readiness')
        public_key = expect(*request(app, '/api/v1/auth/credential-key'), 'credential key')
        login_fixture = REPO / 'diagnostics/upstream-sync-20261112/runtime/credential_envelope_fixture.mjs'
        envelope = json.loads(command(['docker', 'run', '--rm', '-i', '--network', 'none',
            '-v', str(login_fixture) + ':/fixture.mjs:ro', 'node:24-alpine', 'node', '/fixture.mjs'],
            input=encode({'server': public_key, 'email': 'admin@fixture.invalid',
                          'password': env['ISSUE_ADMIN_PASSWORD']})))
        login = expect(*request(app, '/api/v1/auth/login', {'credential_envelope': envelope}), 'login')
        admin = login['access_token']
        uid = login['user']['id']
        sql(f'UPDATE users SET balance=100, concurrency=32 WHERE id={uid}')
        # Initialize only this disposable, synthetic administrator fixture.
        # No existing installation or human administrator is modified.
        compliance = expect(*request(app, '/api/v1/admin/compliance', token=admin), 'fixture status')
        fixture_ack = json.dumps({'version': compliance['version'], 'admin_user_id': uid})
        sql("INSERT INTO settings(key,value) VALUES "
            f"('admin_compliance_acknowledgement:{uid}','{fixture_ack}')")
        fixtures = {}
        for passthrough in (False, True):
            label = 'passthrough' if passthrough else 'standard'
            group = expect(*request(app, '/api/v1/admin/groups',
                {'name': 'issue-8636-' + label, 'platform': 'openai', 'rate_multiplier': 1}, admin), 'group')
            extra = {'custom_base_url_enabled': True,
                     'custom_base_url': 'http://127.0.0.1:8000/backend-api/codex',
                     'openai_passthrough': passthrough,
                     'openai_oauth_responses_websockets_v2_enabled': False}
            account = expect(*request(app, '/api/v1/admin/accounts',
                {'name': 'issue-8636-' + label, 'platform': 'openai', 'type': 'oauth',
                 'credentials': {'access_token': 'synthetic-not-an-openai-token',
                                 'chatgpt_account_id': 'fixture-' + label},
                 'extra': extra, 'concurrency': 32, 'group_ids': [group['id']]}, admin), 'account')
            fixtures[label] = {'aid': account['id'], 'gid': group['id']}

        def compression(fixture, enabled):
            path = '/api/v1/admin/accounts/' + str(fixture['aid'])
            account = expect(*request(app, path, token=admin), 'read account')
            extra = account['extra']
            extra['codex_request_compression_enabled'] = enabled
            expect(*request(app, path, {'extra': extra}, admin, method='PUT'), 'update compression')
            persisted = sql(f"SELECT extra->'codex_request_compression_enabled' FROM accounts WHERE id={fixture['aid']}")
            assert persisted == str(enabled).lower(), persisted

        def key(fixture, name):
            data = expect(*request(app, '/api/v1/keys',
                {'name': name, 'group_id': fixture['gid']}, admin), 'create API key')
            return data['id'], data['key']

        def run_case(label, fixture, model, stream, enabled, mode, inbound='plain'):
            name = f'{label}-{model}-{stream}-{enabled}-{mode}-{inbound}'
            kid, api_key = key(fixture, name)
            payload = {'model': model, 'stream': stream, 'instructions': 'Synthetic diagnostic fixture.',
                       'input': [{'type': 'message', 'role': 'user', 'content': [
                           {'type': 'input_text', 'text': 'compression fixture ' * 256}]}]}
            request(upstream, '/fixture/state', {'mode': mode})
            data, headers = encode(payload), {}
            if inbound in ('zstd', 'zstd_without_header'):
                data = zstd.compress(data, level=3)
            if inbound in ('zstd', 'plain_with_zstd_header'):
                headers['Content-Encoding'] = 'zstd'
            status, value = request(app, '/v1/responses', token=api_key, raw=data, headers=headers)
            captures = request(upstream, '/fixture/state')[1]['captures']
            malformed = inbound in ('zstd_without_header', 'plain_with_zstd_header')
            structured = mode == 'reject_structured'
            recoverable = mode in ('reject_zstd', 'reject_zstd_415', 'reject_all')
            recovered = args.expect_recovery and enabled and recoverable
            rejected = malformed or structured or mode == 'reject_all' or (mode.startswith('reject_zstd') and enabled and not args.expect_recovery)
            assert status == (400 if rejected else 200), (name, status, str(value)[:350])
            assert len(captures) == (0 if malformed else 2 if recovered else 1), (name, captures)
            if captures:
                sent = captures[0]
                assert sent['encoding'] == ('zstd' if enabled else ''), (name, sent)
                assert sent['valid_json'] and sent['model'] == model, (name, sent)
                assert sent['input_hash'] == hashlib.sha256(encode(payload['input'])).hexdigest(), (name, sent)
                assert all(c['valid_json'] and c['content_length'] == c['wire_bytes'] for c in captures) if args.expect_recovery else True
                if recovered:
                    assert captures[1]['encoding'] == '', (name, captures)
                    assert captures[1]['body_hash'] == captures[0]['body_hash'], (name, captures)
                    assert captures[1]['identity_hash'] == captures[0]['identity_hash'], (name, captures)
            if rejected and not malformed:
                expected_message = 'Upstream request failed' if label == 'passthrough' else 'Invalid input parameter' if structured else 'Bad Request'
                assert value['error']['message'] == expected_message, (name, value)
            elif not rejected:
                assert ('response.completed' in value and 'fixture ok' in value) if stream else value['status'] == 'completed', (name, value)
            result = {'case': name, 'status': status, 'key_id': kid, 'success': not rejected,
                      'upstream_calls': len(captures), 'wire': captures,
                      'client_error': value if rejected else None}
            RESULT['cases'].append(result)
            print(json.dumps({'case': name, 'status': status, 'upstream_calls': len(captures)}), flush=True)

        for label, fixture in fixtures.items():
            for enabled in (True, False):
                if not enabled:
                    compression(fixture, False)
                for mode in ('accept', 'reject_zstd'):
                    for model in ('gpt-6.1-sol', 'gpt-6-astra'):
                        for stream in (False, True):
                            run_case(label, fixture, model, stream, enabled, mode)
            for stream in (False, True):
                run_case(label, fixture, 'gpt-6.1-sol', stream, False, 'reject_zstd', 'zstd')
                run_case(label, fixture, 'gpt-6.1-sol', stream, False, 'reject_all')
            for malformed in ('zstd_without_header', 'plain_with_zstd_header'):
                run_case(label, fixture, 'gpt-6.1-sol', True, False, 'accept', malformed)
            if args.expect_recovery:
                compression(fixture, True)
                for stream in (False, True):
                    for mode in ('reject_all', 'reject_zstd_415', 'reject_structured'):
                        run_case(label, fixture, 'gpt-6.1-sol', stream, True, mode)
                    run_case(label, fixture, 'gpt-6.1-sol', stream, True, 'reject_zstd', 'zstd')

        if args.expect_http200_recovery:
            for label, fixture in fixtures.items():
                compression(fixture, True)
                matrix = [(model, stream, 'http200_' + shape + '_once')
                          for model in ('gpt-6.1-sol', 'gpt-6-astra')
                          for stream in (False, True)
                          for shape in ('nested', 'flat', 'named', 'bare', 'failed', 'json')]
                matrix += [('gpt-6.1-sol', stream, 'http200_encoding_mixed') for stream in (False, True)]
                matrix += [('gpt-6.1-sol', True, 'http200_' + shape)
                           for shape in ('partial_error', 'truncated', 'reset', 'all_failed', 'all_failed_committed')]
                for model, stream, mode in matrix:
                    name = f'{label}-{model}-{stream}-{mode}'
                    kid, api_key = key(fixture, name)
                    payload = {'model': model, 'stream': stream, 'instructions': 'HTTP 200 diagnostic.',
                               'input': [{'role': 'user', 'content': [{'type': 'input_text', 'text': 'test ' + name}]}]}
                    request(upstream, '/fixture/state', {'mode': mode})
                    status, value = request(app, '/v1/responses', payload, api_key)
                    captures = request(upstream, '/fixture/state')[1]['captures']
                    success = mode.endswith('_once') or mode.endswith('_mixed')
                    expected_calls = 3 if mode.endswith('_mixed') else 2 if success or 'all_failed' in mode else 1
                    assert status == 200 or (mode.endswith('all_failed') and status == 502), (name, status, str(value)[:400])
                    assert len(captures) == expected_calls, (name, captures)
                    assert captures[0]['encoding'] == 'zstd', (name, captures)
                    assert all(c['body_hash'] == captures[0]['body_hash'] and
                               c['identity_hash'] == captures[0]['identity_hash'] and
                               c['content_length'] == c['wire_bytes'] for c in captures), (name, captures)
                    assert 'Upstream request failed' not in str(value), (name, value)
                    assert '"type":"error"' not in str(value), (name, value)
                    if success:
                        assert ('response.completed' in value and 'fixture ok' in value) if stream else value['status'] == 'completed', (name, value)
                        if mode.endswith('_mixed'):
                            assert [c['encoding'] for c in captures] == ['zstd', '', '']
                        else:
                            assert all(c['encoding'] == 'zstd' for c in captures)
                    else:
                        if status == 200:
                            assert 'response.failed' in value and 'response.completed' not in value, (name, value)
                        else:
                            assert value['error']['message'] == 'Upstream service temporarily unavailable', (name, value)
                        if mode.endswith('truncated'):
                            assert 'upstream_stream_truncated' in value
                        if mode.endswith('reset'):
                            assert 'upstream_stream_read_error' in value
                    RESULT['cases'].append({'case': name, 'status': status, 'key_id': kid, 'success': success,
                                            'upstream_calls': len(captures), 'wire': captures,
                                            'client_error': None if success else value})
                    print(json.dumps({'case': name, 'status': status, 'upstream_calls': len(captures)}), flush=True)

        # Repeated/concurrent requests use a separate key for an exact settlement count.
        kid, api_key = key(fixtures['standard'], 'concurrent-40')
        request(upstream, '/fixture/state', {'mode': 'http200_flat_once' if args.expect_http200_recovery else 'reject_zstd'})

        def repeat(index):
            stream = bool(index % 2)
            status, value = request(app, '/v1/responses', {
                'model': 'gpt-6.1-sol', 'stream': stream, 'instructions': 'Synthetic load fixture.',
                'input': [{'role': 'user', 'content': [{'type': 'input_text', 'text': f'fixture {index}'}]}]}, api_key)
            assert status == 200 and 'fixture ok' in str(value), (index, status, value)
            return status

        with ThreadPoolExecutor(max_workers=4) as pool:
            statuses = list(pool.map(repeat, range(40)))
        captures = request(upstream, '/fixture/state')[1]['captures']
        expected_calls = 80 if args.expect_recovery else 40
        assert len(captures) == expected_calls, captures
        if args.expect_recovery:
            assert sum(c['encoding'] == 'zstd' for c in captures) == (80 if args.expect_http200_recovery else 40)
            grouped = {}
            for capture in captures:
                grouped.setdefault(capture['input_hash'], []).append(capture)
            assert len(grouped) == 40
            assert all(len(pair) == 2 and pair[0]['body_hash'] == pair[1]['body_hash'] and
                       pair[0]['identity_hash'] == pair[1]['identity_hash'] for pair in grouped.values())
        else:
            assert all(c['encoding'] == '' for c in captures)
        RESULT['concurrency'] = {'requests': 40, 'workers': 4, 'success': statuses.count(200),
                                 'key_id': kid, 'upstream_calls': len(captures),
                                 'compression_enabled': args.expect_recovery}

        # Roll back the account option through the same API and observe zstd again.
        compression(fixtures['standard'], True)
        run_case('restored', fixtures['standard'], 'gpt-6.1-sol', True, True, 'reject_zstd')

        expected = sum(case['success'] for case in RESULT['cases']) + 40
        for _ in range(120):
            count = int(sql('SELECT count(*) FROM usage_logs'))
            if count == expected:
                break
            time.sleep(.1)
        assert count == expected, ('settlement total', count, expected)
        for case in RESULT['cases']:
            count = int(sql(f"SELECT count(*) FROM usage_logs WHERE api_key_id={case['key_id']}"))
            assert count == int(case['success']), ('case settlement', case['case'], count)
        repeats = int(sql(f'SELECT count(*) FROM usage_logs WHERE api_key_id={kid}'))
        assert repeats == 40, ('concurrent settlement', repeats)
        RESULT['settlement'] = {'rows': expected, 'concurrent_rows': repeats,
                                'failed_request_rows': 0, 'duplicate_rows': 0}
        RESULT['account_state'] = json.loads(sql("SELECT json_agg(json_build_object('status',status,'schedulable',schedulable,'error_msg',error_message)) FROM accounts"))
        assert all(a['status'] == 'active' and a['schedulable'] for a in RESULT['account_state'])
        logs = compose('logs', '--no-color', 'app')
        (output / 'app.log').write_text(logs)
        matching_logs = [line for line in logs.splitlines() if 'upstream error: 400' in line]
        RESULT['matching_error_log_count'] = len(matching_logs)
        assert any('message=Bad Request' in line for line in matching_logs), 'original log signature missing'
        if args.expect_recovery:
            RESULT['recovery_log_count'] = sum('Retrying rejected zstd request once' in line for line in logs.splitlines())
            RESULT['http200_recovery_log_count'] = sum('Retrying HTTP 200 generic stream failure once' in line for line in logs.splitlines())
            RESULT['compression_enabled_after_test'] = json.loads(sql("SELECT json_agg(extra->'codex_request_compression_enabled') FROM accounts"))
            assert RESULT['compression_enabled_after_test'] == [True, True]
        RESULT['result'] = 'passed'
    except Exception as error:
        RESULT['result'] = 'failed'
        RESULT['failure'] = str(error)
        raise
    finally:
        if started:
            (output / 'app.log').write_text(compose('logs', '--no-color', 'app'))
            compose('down', '-v', '--remove-orphans')
        args.evidence_dir.mkdir(parents=True, exist_ok=True)
        (args.evidence_dir / 'RESULTS.json').write_text(json.dumps(RESULT, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'result': RESULT['result'], 'cases': len(RESULT['cases']),
                      'concurrency': RESULT['concurrency'], 'settlement': RESULT['settlement']}), flush=True)


if __name__ == '__main__':
    main()
