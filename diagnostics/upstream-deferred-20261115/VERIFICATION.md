# 验证结果

所有执行命令、退出码、耗时和原始输出见 `records/*.json` / `*.log.gz`。原始日志保留失败，以下列出最终有效结果；未调用真实模型或支付账号。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| Docker Go 1.26.11 全包 unit 与官方 MCP OAuth 客户端互操作 | PASS | `backend-unit-final.json` |
| 最后新增 SDK / RetryInfo / UA / 审核用例及完整共享协议包 | PASS | `backend-final-additions.json`, `shared-final.json` |
| Docker PostgreSQL 18.1 / Redis 8.4 全包 integration，独立审计数据库 | PASS | `integration.json` |
| PostgreSQL 真实并发订阅扣减与续期交错 | PASS | `subscription-concurrency-final.json` |
| Docker golangci-lint 2.9 / Go 1.26.11 | PASS, 0 issues | `golangci-final.json` |
| Docker 前端 ESLint / vue-tsc | PASS | `frontend-release.log.gz` 中独立步骤；该次总退出 1 为后续 CSV 旧预期，已在最终全套修正 |
| Docker 全部前端测试 | PASS, 427 files / 2635 tests | `frontend-tests-final.json` |
| 完整前后端生产 Docker 构建 | PASS | `build-baseline-final.json`, `build-candidate-final.json`, `IMAGES.json` |
| 最终镜像同库 baseline → candidate → rollback → restored | PASS, 42 HTTP 场景 | `runtime-final.json`, `runtime/result.json` |
| 文档、后端源码布局、diff 空白检查 | PASS | 仓库根目录 `make check-docs`, `make -C backend check-layout`, `git diff --check` |
| 冻结 PR / owner / 净 diff 摘要与覆盖集合复核 | PASS, 198 entries | `build_ledger.py`, `UPSTREAM_PRS.json` |

全包 unit/integration 之后的最后几项纯协议新增，由对应整包/聚焦用例补充验证；推送后的 CI 仍对最终 commit 执行完整门禁。`SOURCE_HASHES.json` 保存最终改动源码与测试的字节摘要；最终镜像的 66 个运行源码改动与构建输入逐一核对一致。测试文件后续的夹具修正不改变运行二进制。

同库四阶段迁移数量均为 317。每阶段核对安装后的加密登录、旧 refresh credential 消费、Key、订阅和身份快照，验证已存在用量记录字节摘要不变，以及 `余额 + 已落库实际费用 = 初始余额`。Responses 流式与非流式、Chat、图片按实际输出计费、Grok 和 Kimi 错误分类均有 HTTP 场景；候选与恢复阶段还验证 8 路 refresh 只有一次消费成功。该矩阵验证兼容性，不代表真实上游账户或生产吞吐验收。

性能三次样本保存于 `PERFORMANCE.json` 和原始 performance 记录。1000 条历史的中位耗时 `37.215433ms → 0.357461ms`，分配 `88746865 → 501400 B/op`。源码采用逐消息修改后一次拼接；数字只说明这个固定输入。

首次前端全量的 8 个主线失败已在冻结主线同镜像复现，见 `frontend-baseline.json`。修正 Pinia/权限/确认密码夹具及中英文 literal 文案；CSV 缺省值预期随 #7446 的新行为更新。初始 Node 堆不足、移植测试使用上游专属入口、未用 helper 和未检查类型断言均已修正；这些记录不覆盖或删除。

发布以 PR head / main merge 的精确 SHA 和远端工作流结论为准。本目录中的本地 Docker 构建不充当 GHCR 已发布证明。

PR 首次 CI 的 push lint 在获取 `golangci.v2.9.jsonschema.json` 时连接超时，未进入代码检查；PR 的另一轮 lint 正常继续。CLA 工作流的守卫在 `39ea47746` 模块更名中被从上游仓库名称替换为 fork 名称，从而误启用未配置 `cla-signatures` 分支的流程。本轮恢复其父提交的原工作流，保持此前 fork 跳过上游 CLA 的行为，不创建或修改任何签名记录。`pull_request_target` 使用 base 分支工作流，因此修复在合并进入 base 后才生效。
