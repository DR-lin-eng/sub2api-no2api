#!/usr/bin/env python3
import hashlib,json,pathlib,tarfile
r=pathlib.Path(__file__).resolve().parent;root=r.parents[1]
required=['modified-accepted-probe','reapply-accepted-source-valid','unit-go1269','integration-go1269-valid','govulncheck-go1269','frontend-required','build-baseline','build-candidate-final','runtime-upgrade-rollback','lint-go1269-reviewed','race-and-http2-go1269','source-review']
for name in required:
 j=json.loads((r/'records'/f'{name}.record.json').read_text())
 assert j['exit_status']==0,(name,j['exit_status'])
for name in ['baseline-probe-valid','rollback-accepted-probe-valid']:
 j=json.loads((r/'records'/f'{name}.record.json').read_text());assert j['exit_status']==1
 out=(r/'records'/f'{name}.stdout').read_text()
 assert out.count('declared= choice=auto trigger_last=true input_history=web_search_call')==4,name
 assert 'signal: killed' not in out
m=json.loads((r/'SOURCE_HASHES.json').read_text())
assert all(hashlib.sha256((root/p).read_bytes()).hexdigest()==sha for p,sha in m['modified'].items())
with tarfile.open(r/'MODIFIED_FILE') as a:
 assert {e.name:hashlib.sha256(a.extractfile(e).read()).hexdigest() for e in a.getmembers()}==m['modified']
header='''VERIFIED TRANSACTION / selective upstream synchronization 2026-10-09
TARGET=/Users/lin/.codex/worktrees/b2e2/sub2api-no2api
MODIFIED_COPY=/private/tmp/sub2api-upstream-sync-20261009
BASELINE_SHA=03a9207f932378207bf07ad09ce57aa154b5e0cd
PREVIOUS_UPSTREAM_SHA=5fc0e486c3f6a8a191b8bd140f39b60457f611cf
UPSTREAM_SHA=3a6fd1c9db07203ca308aaba69e502bc1f35b307
BRANCH=codex/upstream-sync-20261009
CHANGED_SYMBOL=applyCodexOAuthTransformWithOptions; normalizeOpenAIOAuthWebSearchHistoryForAccount
CHANGED_FIELD=tools / input.additional_tools / tool_choice; trimmed BillingModel
MATCHING_COMMAND=go test -tags=unit ./internal/application/service -run '^TestForwardOAuthWebSearchHistory20261009$' -count=1 -v
MATCHING_INPUT=committed HTTP Forward fixture: OAuth full history web_search_call + user + compaction_trigger; tools=[]; tool_choice=auto; standard/Lite x transform/passthrough
BASELINE=All four paths no search declaration, choice=auto, trigger_last=true; exit 1.
MODIFIED=All four paths cached-only web_search declaration, choice=none, trigger_last=true; exit 0.
ROLLBACK=All four paths restored to no declaration, choice=auto, trigger_last=true; exit 1.
ROLLBACK_SCRIPT_EXIT=0; pristine baseline hashes verified before same test-only fixture injection.
DIFF_REAPPLY=0; all source/config/document hashes equal MODIFIED_FILE.
UNIT=Go 1.26.9 Docker full unit PASS.
INTEGRATION=Go 1.26.9 Docker full integration PASS; CI=true and real Testcontainers required. Initial missing-Docker-CLI error retained and corrected.
RACE=OAuth HTTP/WS helper/new Forward/native Anthropic + HTTP2 PING/proxy/H2C PASS.
LINT=golangci-lint v2.9 in Go 1.26.9; exact owner/symbol HTTP2 compatibility deprecation rules as upstream #7618; no global SA1019 suppression.
SECURITY=Go 1.26.6/x-net0.57 initial scan reported 10 reachable stdlib vulnerabilities; Go1.26.9/x-net0.60 reports 0 reachable vulnerabilities, 1 module-only noncalled finding.
FRONTEND=Node24/frozen dependencies lint/typecheck/29 mandatory files/247 tests PASS; existing 11-platform quota tests 25 PASS; no required test excluded.
RUNTIME=PostgreSQL18/Redis8 same database baseline/upgraded/rollback/restored: health/readiness/frontend/RSA-OAEP+AES-GCM cookie login/persisted-key+quota/legacy pending order/data snapshot PASS. Invalid old initialization env cannot overwrite admin. Migrations 301 throughout.
LAYOUT=Forward reduced baseline1233->1200; remaining payment_fulfillment_test.go1201/pricing_service.go1238 are byte-identical baseline failures. No layout allowlist expansion.
PERFORMANCE=Frozen upstream vs adapted, same Go1.26.6/arm64/GOMAXPROCS2/8MiB/10iterations/3rounds: top-declared median5.73ms->1.54ms,8.40MB->0B; Lite insert12.31ms->8.79ms,50.38MB->16.79MB. additional-declared latency not improved; allocation0. Not total gateway throughput.
NO_ENT_WIRE_MIGRATION=confirmed by path set. Original worktree stayed clean at original baseline SHA.
UPSTREAM_DECISIONS=#7939 fully closed; #7618 security/model/current coverage closed by subitem, new platform/profile/catalog/Command Code/Cline deferrals retained, fully_closed=false. Older ledgers unchanged.
PUBLICATION=PR/head/main/Actions/GHCR must be independently read back; publication records extend these roles without changing tested source.

'''
for name in ['MODIFIED_FILE','DIFF_FILE','ROLLBACK.sh','BASELINE_FILE','SOURCE_HASHES.json']:
 b=(r/name).read_bytes();header+=f'ARTIFACT={r/name}\nSHA256={hashlib.sha256(b).hexdigest()}\nBYTES={len(b)}\n'
sections=[]
for path in sorted((r/'records').glob('*.record.json')):
 j=json.loads(path.read_text());name=path.name.removesuffix('.record.json')
 sections.append('\nRECORD='+name+'\nMETADATA='+json.dumps(j,ensure_ascii=False)+'\nSTDOUT_BEGIN\n'+(r/'records'/f'{name}.stdout').read_text(errors='replace')+'\nSTDOUT_END\nSTDERR_BEGIN\n'+(r/'records'/f'{name}.stderr').read_text(errors='replace')+'\nSTDERR_END\n')
(r/'VERIFICATION.txt').write_text(header+'\nACCEPTED/ERROR COMMANDS AND LITERAL OUTPUTS\n'+''.join(sections))
roles={}
for name in ['MODIFIED_FILE','DIFF_FILE','VERIFICATION.txt','ROLLBACK.sh']:
 p=r/name;b=p.read_bytes();roles[name]={'path':str(p),'sha256':hashlib.sha256(b).hexdigest(),'bytes':len(b)}
(r/'ROLE_HASHES.json').write_text(json.dumps(roles,indent=2)+'\n')
print(json.dumps(roles,indent=2))
