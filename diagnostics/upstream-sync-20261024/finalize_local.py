#!/usr/bin/env python3
from pathlib import Path
import hashlib,json,subprocess
EV=Path(__file__).resolve().parent;REPO=EV.parents[1];ROOT=REPO.parent
for label in ['backend-unit-optimized','capacity-race-optimized','backend-lint-optimized','production-image-build-optimized','runtime-matrix-final']:
 assert json.loads((EV/'records'/f'{label}.record.json').read_text())['exit_status']==0,label
perf=json.loads((EV/'PERFORMANCE.json').read_text());runtime=json.loads((EV/'runtime/result.json').read_text())
assert [s['stage'] for s in runtime]==['BASELINE','MODIFIED','ROLLBACK','RESTORED']
assert len({s['persisted_identity_key_subscription_sha256'] for s in runtime})==1
assert all(s['result']=='PASS' and s['migrations']==306 and len(s['cases'])==6 for s in runtime)
doc=REPO/'docs/UPSTREAM_SYNC_20261024.md';text=doc.read_text().split('\n## 验证结果')[0]
text+='\n## 验证结果\n\nDocker Go 1.26.10 的最终全后端单元测试、相关竞态测试和 lint 均通过，生产镜像构建通过。\n前端固定安装验证 DOMPurify 3.4.17，4 个清洗相关测试文件的 12 个用例通过。\n文档门禁通过；源码布局门禁只复现基线已有的两个 1201 行文件，不增加豁免。\n\n'
text+='同一 PostgreSQL/Redis 环境完成 4 阶段、24 个真实 HTTP 场景：\n\n| 阶段 | 1.25 秒提示下的实际重试间隔 | 迁移数 | 数据与行为检查 |\n| --- | --- | --- | --- |\n'
for stage in runtime:
 gaps=[v['retry_gap_seconds'] for v in stage['cases'] if 'retry_gap_seconds' in v]
 text+=f"| {stage['stage']} | {min(gaps):.3f}–{max(gaps):.3f} 秒 | {stage['migrations']} | 通过 |\n"
text+='\n流式和非流式均恢复为 200，每次成功请求仅产生一条 10/5 token 用量记录；已发出的真实文本只出现一次，普通 400 仅请求上游一次。身份、持久 Key、订阅、账号调度状态和余额等式在四阶段均通过。\n\n'
text+='固定 2 CPU、3 GiB、每项 5 次的 Docker 微基准中位数：\n\n| 夹具 | 基线 ns/op | 修改 ns/op | 回退 ns/op | 修改相对基线 | 修改 B/op、分配数 |\n| --- | --- | --- | --- | --- | --- |\n'
for name,v in perf['states']['MODIFIED'].items():
 b=perf['states']['BASELINE'][name];r=perf['states']['ROLLBACK'][name]
 text+=f"| {name} | {b['median_ns']:.2f} | {v['median_ns']:.2f} | {r['median_ns']:.2f} | {v['change_percent']:+.2f}% | {v['bytes']:g}、{v['allocs']:g} |\n"
text+='\n首轮无提示容量错误多做四次 JSON 路径扫描，333→561 ns；已通过无提示快速判定移除重复扫描，并对 Unicode 转义键保留完整解析。首轮结果保留在 `history/first-parser/`。错误元数据新增字段的内存成本单独列出；正常文本事件仍为零分配。以上仅是函数夹具，不能据此断言生产吞吐没有任何变化。\n\n'
text+='源码补丁与修改包分别重建相同字节；自包含回退脚本在独立副本执行后，所有受影响文件哈希恢复为基线，再重新应用修改。原工作树保持原 HEAD 与干净状态。\n'
doc.write_text(text)
before=json.loads((EV/'SOURCE_HASHES.json').read_text())
subprocess.run(['python3',str(EV/'package_roles.py')],cwd=REPO,check=True)
after=json.loads((EV/'SOURCE_HASHES.json').read_text())
assert all(after['modified_hashes'][p]==h for p,h in before['modified_hashes'].items() if p.endswith('.go'))
subprocess.run(['python3',str(EV/'record.py'),'source-transaction-final','python3',str(EV/'source_transaction.py')],cwd=REPO,check=True)
subprocess.run(['git','apply',str(EV/'DIFF_FILE.patch')],cwd=ROOT/'ROLLBACK_TARGET',check=True)
assert all(hashlib.sha256((ROOT/'ROLLBACK_TARGET'/p).read_bytes()).hexdigest()==h for p,h in after['modified_hashes'].items() if h is not None)
subprocess.run(['python3',str(EV/'record.py'),'check-docs-final','make','check-docs'],cwd=REPO,check=True)
subprocess.run(['git','diff','--check'],cwd=REPO,check=True)
state=json.loads((EV/'STATUS.json').read_text());state['last_confirmed_result']='Final local code, Docker runtime 24-case/306-migration matrix, performance and source rollback/reapply accepted.';state['next_executable_action']='Commit candidate and closure ledger, record reviewed upstream ancestry, push, create PR and track exact PR/main CI plus GHCR.';(EV/'STATUS.json').write_text(json.dumps(state,indent=2)+'\n')
subprocess.run(['python3',str(EV/'finalize_verification.py')],cwd=REPO,check=True)
