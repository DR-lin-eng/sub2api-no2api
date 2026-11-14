#!/usr/bin/env python3
"""Same-database Docker baseline/modified/rollback/restore, synthetic data only."""
import argparse
from compression import zstd
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import time

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parent.parent


def encode(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':')).encode()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--results', type=Path, default=Path('/private/tmp/sub2api-oauth-alignment-20261114'))
    parser.add_argument('--baseline-ref', default='HEAD')
    parser.add_argument('--skip-build', action='store_true')
    args = parser.parse_args()
    output = args.results.resolve()
    output.mkdir(parents=True, exist_ok=True)
    (output / 'wire').mkdir(exist_ok=True)
    env = {**os.environ, 'ISSUE_RESULTS': str(output),
           'ISSUE_DB_PASSWORD': secrets.token_urlsafe(24),
           'ISSUE_ADMIN_PASSWORD': 'Fixture14!' + secrets.token_urlsafe(16),
           'ISSUE_JWT_SECRET': secrets.token_urlsafe(48),
           'ISSUE_ENCRYPTION_KEY': secrets.token_hex(32)}
    compose_cmd = ['docker', 'compose', '-f', str(ROOT.parent / 'issue-8636-20261112/compose.yaml'),
                   '-p', 'sub2api-oauth-alignment-' + secrets.token_hex(3)]

    def command(cmd, **kwargs):
        done = subprocess.run(cmd, env=env, capture_output=True, **kwargs)
        if done.returncode:
            raise RuntimeError(f'{cmd[0]} failed ({done.returncode}): {done.stderr.decode()[-1500:]}')
        return done.stdout

    def compose(*words):
        return command(compose_cmd + list(words)).decode().strip()

    def sql(statement):
        return compose('exec', '-T', 'postgres', 'psql', '-U', 'fixture', '-d', 'fixture',
                       '-v', 'ON_ERROR_STOP=1', '-qAtc', statement)

    def build(source, script):
        command(['docker', 'run', '--rm', '--network', 'none', '-e', 'GOTOOLCHAIN=local',
                 '-e', 'GOPROXY=off', '-e', 'GOSUMDB=off', '-e', 'CGO_ENABLED=0',
                 '-v', str(source) + ':/src:ro', '-v', str(ROOT) + ':/fixture:ro',
                 '-v', str(output) + ':/results',
                 '-v', 'sub2api-go-mod-cache:/go/pkg/mod',
                 '-v', 'sub2api-go-build-cache:/root/.cache/go-build',
                 '-w', '/src', 'golang:1.26.11-alpine', 'sh', '-ec', script])

    baseline = output / 'baseline'
    baseline.mkdir(exist_ok=True)
    (output / 'baseline-results').mkdir(exist_ok=True)
    if not args.skip_build:
        archive = command(['git', '-C', str(REPO), 'archive', args.baseline_ref, 'backend'])
        command(['tar', '-x', '-C', str(baseline)], input=archive)
        build(baseline / 'backend', 'go build -buildvcs=false -p 2 -o /results/baseline-results/sub2api ./cmd/server')
        build(REPO / 'backend', 'go build -buildvcs=false -p 2 -o /results/sub2api-modified ./cmd/server')
    build(REPO / 'backend', 'go build -buildvcs=false -o /results/provider /fixture/provider.go')
    report = {'baseline_ref': command(['git', '-C', str(REPO), 'rev-parse', args.baseline_ref]).decode().strip(),
              'real_upstream_tested': False, 'same_database': True, 'phases': [],
              'binary_sha256': {name: hashlib.sha256(path.read_bytes()).hexdigest() for name, path in
                                [('baseline', output / 'baseline-results/sub2api'),
                                 ('modified', output / 'sub2api-modified')]}}
    app_container = ''

    def request(base, path, body=None, token=None, raw=None, headers=None, method=None):
        hdr = {'Content-Type': 'application/json', 'User-Agent': 'codex_cli_rs/0.130.0',
               'originator': 'codex_cli_rs', **(headers or {})}
        if token:
            hdr['Authorization'] = 'Bearer ' + token
        data = raw if raw is not None else None if body is None else encode(body)
        words = ['docker', 'exec', '-i', app_container, 'curl', '--silent', '--show-error',
                 '--noproxy', '*', '--max-time', '30', '--write-out', '\n%{http_code}',
                 '--request', method or ('POST' if data is not None else 'GET')]
        if path.startswith('/api/v1/auth/'):
            words += ['--cookie', '/tmp/oauth-alignment-cookies', '--cookie-jar', '/tmp/oauth-alignment-cookies']
        for name, value in hdr.items():
            words += ['--header', name + ': ' + value]
        if data is not None:
            words += ['--data-binary', '@-']
        wire, _, status = command(words + [base + path], input=data).rpartition(b'\n')
        try:
            value = json.loads(wire)
        except (ValueError, UnicodeError):
            try:
                value = wire.decode()
            except UnicodeError:
                value = wire
        return int(status), value, wire

    def data(result):
        assert result[0] == 200, (result[0], str(result[1])[:300])
        return result[1]['data']

    app, provider = 'http://127.0.0.1:8080', 'http://127.0.0.1:8000'
    fixtures = {}
    admin = None
    started = False
    try:
        for phase in ('BASELINE', 'MODIFIED', 'ROLLBACK', 'RESTORED'):
            if started:
                compose('stop', 'provider', 'app')
                compose('rm', '-f', 'provider', 'app')
            binary = output / ('baseline-results/sub2api' if phase in ('BASELINE', 'ROLLBACK') else 'sub2api-modified')
            shutil.copyfile(binary, output / 'sub2api')
            (output / 'sub2api').chmod(0o755)
            started = True
            compose('up', '-d', 'app')
            compose('up', '-d', 'provider')
            app_container = compose('ps', '-q', 'app')
            for _ in range(120):
                try:
                    if request(app, '/ready')[0] == 200 and request(provider, '/fixture/state')[0] == 200:
                        break
                except RuntimeError:
                    pass
                time.sleep(.5)
            else:
                raise AssertionError('fixture readiness failed')
            if admin is None:
                public_key = data(request(app, '/api/v1/auth/credential-key'))
                envelope = json.loads(command(['docker', 'run', '--rm', '-i', '--network', 'none',
                    '-v', str(REPO / 'diagnostics/upstream-sync-20261112/runtime/credential_envelope_fixture.mjs') + ':/fixture.mjs:ro',
                    'node:24-alpine', 'node', '/fixture.mjs'], input=encode({
                        'server': public_key, 'email': 'admin@fixture.invalid', 'password': env['ISSUE_ADMIN_PASSWORD']})))
                login = data(request(app, '/api/v1/auth/login', {'credential_envelope': envelope}))
                admin, uid = login['access_token'], login['user']['id']
                sql(f'UPDATE users SET balance=100, concurrency=16 WHERE id={uid}')
                compliance = data(request(app, '/api/v1/admin/compliance', token=admin))
                acknowledgement = json.dumps({'version': compliance['version'], 'admin_user_id': uid})
                sql(f"INSERT INTO settings(key,value) VALUES ('admin_compliance_acknowledgement:{uid}','{acknowledgement}')")
                for label in ('standard', 'passthrough'):
                    group = data(request(app, '/api/v1/admin/groups', {
                        'name': 'alignment-' + label, 'platform': 'openai', 'rate_multiplier': 1}, admin))
                    account = data(request(app, '/api/v1/admin/accounts', {
                        'name': 'alignment-' + label, 'platform': 'openai', 'type': 'oauth',
                        'credentials': {'access_token': 'synthetic-not-an-openai-token', 'chatgpt_account_id': 'fixture-' + label},
                        'extra': {'custom_base_url_enabled': True, 'custom_base_url': provider + '/backend-api/codex',
                                  'openai_passthrough': label == 'passthrough',
                                  'openai_oauth_responses_websockets_v2_enabled': False},
                        'concurrency': 16, 'group_ids': [group['id']]}, admin))
                    fixtures[label] = {'aid': account['id'], 'gid': group['id']}

            phase_result = {'phase': phase, 'cases': [], 'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest()}
            report['phases'].append(phase_result)
            for label, fixture in fixtures.items():
                cases = [('responses', stream, mode, 'plain') for stream in (False, True) for mode in
                         ('accept', 'response_zlib', 'response_raw_deflate', 'response_gzip', 'response_br',
                          'response_zstd', 'reject_zstd', 'reject_zstd_415', 'reject_structured')]
                cases += [('responses/compact', False, mode, 'plain') for mode in ('accept', 'response_zlib')]
                cases += [('responses', False, 'accept', encoding) for encoding in
                          ('zstd', 'unknown_frame_size', 'truncated', 'trailing_garbage', 'double_encoded', 'plain_zstd')]
                for endpoint, stream, mode, inbound in cases:
                    name = f'{phase}-{label}-{endpoint.replace("/", "_")}-{stream}-{mode}-{inbound}'
                    key = data(request(app, '/api/v1/keys', {'name': name, 'group_id': fixture['gid']}, admin))
                    payload = {'model': 'gpt-6.1-sol', 'stream': stream, 'store': False,
                               'instructions': 'Synthetic compression alignment.',
                               'metadata': {'counter': 9007199254740993},
                               'input': [{'type': 'message', 'role': 'user', 'content': [
                                   {'type': 'input_text', 'text': 'Unicode 漢字 🌈 fixture. ' * 64}]}]}
                    raw = encode(payload)
                    headers = {'Accept-Encoding': 'gzip, zstd', 'x-codex-app-server-compression': 'zstd-stream-v1'}
                    if inbound != 'plain':
                        headers['Content-Encoding'] = 'zstd'
                        if inbound == 'unknown_frame_size':
                            encoder = zstd.ZstdCompressor(level=3)
                            raw = encoder.compress(raw) + encoder.flush()
                        elif inbound == 'plain_zstd':
                            pass
                        else:
                            raw = zstd.compress(raw, level=3)
                            if inbound == 'truncated':
                                raw = raw[:-2]
                            elif inbound == 'trailing_garbage':
                                raw += b'garbage'
                            elif inbound == 'double_encoded':
                                raw = zstd.compress(raw, level=3)
                    request(provider, '/fixture/state', {'mode': mode})
                    status, value, _ = request(app, '/v1/' + endpoint, token=key['key'], raw=raw, headers=headers)
                    captures = request(provider, '/fixture/state')[1]['captures']
                    malformed = inbound in ('truncated', 'trailing_garbage', 'double_encoded', 'plain_zstd')
                    success = not malformed and mode != 'reject_structured' and not (phase in ('BASELINE', 'ROLLBACK') and mode == 'response_zlib')
                    complete = status == 200 and ('response.completed' in value if isinstance(value, str) else value.get('status') == 'completed')
                    assert complete == success, (name, status, str(value)[:300])
                    assert (not captures) == malformed, (name, captures)
                    checks = []
                    for index, capture in enumerate(captures):
                        wire = request(provider, f'/fixture/wire?index={index}')[2]
                        (output / 'wire' / (name + f'-{index}.bin')).write_bytes(wire)
                        assert hashlib.sha256(wire).hexdigest() == capture['wire_hash']
                        assert len(wire) == capture['wire_bytes'] == capture['content_length']
                        if capture['encoding'] == 'zstd':
                            decoder = zstd.ZstdDecompressor()
                            decoded = decoder.decompress(wire)
                            assert decoder.eof and not decoder.unused_data, name
                        else:
                            decoded = wire
                        parsed = json.loads(decoded.decode('utf-8'))
                        assert hashlib.sha256(decoded).hexdigest() == capture['body_hash']
                        assert parsed['input'] == payload['input'], name
                        assert capture['content_type'] == 'application/json'
                        assert capture['app_server_compression'] == ''
                        checks.append({'wire_sha256': capture['wire_hash'], 'json_sha256': capture['body_hash'],
                                       'wire_bytes': len(wire), 'decoded_bytes': len(decoded), 'complete': True,
                                       'encoding': capture['encoding']})
                    if captures:
                        assert captures[0]['encoding'] == 'zstd', name
                        if mode.startswith('reject_zstd'):
                            assert [c['encoding'] for c in captures] == ['zstd', ''], (name, captures)
                            assert captures[0]['body_hash'] == captures[1]['body_hash']
                            assert captures[0]['identity_hash'] == captures[1]['identity_hash']
                    phase_result['cases'].append({'case': name, 'status': status, 'success': success,
                                                  'key_id': key['id'], 'wire': checks})
            expected = sum(case['success'] for case in phase_result['cases'])
            for _ in range(100):
                actual = sum(int(sql(f"SELECT count(*) FROM usage_logs WHERE api_key_id={case['key_id']}"))
                             for case in phase_result['cases'])
                if actual == expected:
                    break
                time.sleep(.1)
            assert actual == expected, (phase, 'settlement', actual, expected)
            for case in phase_result['cases']:
                assert int(sql(f"SELECT count(*) FROM usage_logs WHERE api_key_id={case['key_id']}")) == int(case['success']), case['case']
            phase_result['settlement'] = {'rows': actual, 'duplicates': 0, 'failed_request_rows': 0}
            phase_result['accounts'] = json.loads(sql("SELECT json_agg(json_build_object('id',id,'status',status,'schedulable',schedulable,'compression',COALESCE(extra->'codex_request_compression_enabled','true'::jsonb))) FROM accounts"))
            assert all(a['status'] == 'active' and a['schedulable'] and a['compression'] for a in phase_result['accounts'])
            (output / (phase + '.log')).write_text(compose('logs', '--no-color', 'app'))
            print(json.dumps({'phase': phase, 'cases': len(phase_result['cases']), 'successful': expected,
                              'settlements': actual}), flush=True)
        report['result'] = 'passed'
    except Exception as error:
        report.update(result='failed', failure=str(error))
        raise
    finally:
        if started:
            (output / 'app-final.log').write_text(compose('logs', '--no-color', 'app'))
            compose('down', '-v', '--remove-orphans')
        (ROOT / 'RESULTS.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'result': report['result'], 'phases': len(report['phases'])}), flush=True)


if __name__ == '__main__':
    main()
