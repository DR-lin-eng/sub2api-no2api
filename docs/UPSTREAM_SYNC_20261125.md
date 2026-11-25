# 上游开放 PR 增量审查（2026-11-25）

上游 `main` 仍停在 `0f6671457`。本批审查基于该边界的 23 个开放 PR head，固定 head、merge-base、净 diff SHA-256、当前 owner、性能判断、升级边界和关闭状态见[机器台账](../diagnostics/upstream-sync-20261125/UPSTREAM_PRS.json)。已处理的 #9008、#9045、#9052 继承 2026-11-24 台账，不重复移植。

本批适配当前 owner 的变更包括内容审核队列按需分配、Gemini 空 usage 占位保护、无版本 Codex identity 净化、Grok 调度 tier 元数据、OpenCode 官方 UA、HelpTooltip viewport 坐标和代理快照展示。内容审核队列从构造时的最大容量改为首次入队分配 64 槽，按配置上限倍增。空闲实例不再预留约 28.8 MB backing storage，QueueSize、丢弃计数和 worker 消费语义保持有界。#9081 的请求级心跳累计、#9069 的代理账号计数和 #9061 的测试 owner 已在本项目复用。

大型协议、数据库迁移、DeepSeek 工具面、独立额度窗口、key-scoped introspection 和新的身份变量契约保留本地范围。#9071 的并行 tool_use 流状态机、#9075 的 Gemini 思考参数归一化和 #9064 的账号错误趋势查询需要单独的性能、数据量和滚动回退证据，已关闭本轮差异并记录重开条件。

本批未新增 migration、Ent/Wire 生成结果、Redis key 或持久账单字段。新增的 OpenCode UA 只作用于官方 OpenCodeGo 主机，默认版本为 1.19.6，可用 `OPENCODE_USER_AGENT_VERSION` 覆盖三段版本。代理列表只增加缓存已有的检查时间和出口字段展示。旧节点可继续读取已有 JSON 和缓存，回退不执行 Down SQL。

验证命令、固定补丁摘要和原始测试结果见[验证目录](../diagnostics/upstream-sync-20261125/README.md)。

```sh
python3 diagnostics/upstream-sync-20261125/verify_ledger.py
```

Docker 验证使用 Go 1.26.11、Node 24 和仓库 Dockerfile。候选 arm64 镜像 `sub2api:upstream-sync-20261125` 构建成功，版本 smoke check 保留项目版本 `0.1.250`，未采用上游 `0.2.25`。真实供应商凭据、生产数据和生产吞吐不在本批验证范围。
