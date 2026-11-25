# 2026-11-25 上游开放 PR 增量审查

上游 `main` 仍停在 `0f6671457`。本批固定审查基于该边界的开放 PR head，机器台账保存每个 head、merge-base、净 diff SHA-256、当前 owner、性能判断、升级边界和关闭状态。没有重复移植 2026-11-24 台账中的 #9008、#9045、#9052。

已适配当前 owner 的变更包括内容审核队列按需分配、Gemini 空 usage 占位保护、无版本 Codex identity 净化、Grok 调度 tier 元数据、OpenCode 官方 UA、HelpTooltip viewport 坐标和代理快照展示。#9081、#9069、#9061 复用当前实现或测试 owner。大型协议、数据库迁移、DeepSeek 工具面、独立额度窗口和新身份契约已关闭本轮差异并记录重开条件。

验证台账：

```sh
python3 diagnostics/upstream-sync-20261125/verify_ledger.py
```

Docker 验证使用 Go 1.26.11、Node 24 和仓库 Dockerfile。定向后端测试、前端回归、类型检查、ESLint、生产前端构建、Go 编译和最终 arm64 镜像版本 smoke check 均已执行。

本批未新增 migration、Ent/Wire 生成结果、Redis key 或持久账单字段。候选镜像保留项目版本 `0.1.250`，未采用上游 `0.2.25`。
