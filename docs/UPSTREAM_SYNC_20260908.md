# 上游同步审查记录（2026-09-08）

本文记录 2026-09-08 批次的选择性上游同步。它只说明当前提交中已经验证的处理和边界；完整命令、镜像、运行栈与文件回滚证据保存在 [`diagnostics/upstream-sync-20260908/VERIFICATION.txt`](../diagnostics/upstream-sync-20260908/VERIFICATION.txt)。

## 审查边界

- 本批项目基线：`2c6606b8374d64c3afd3dae0447eaf19f521523c`。
- 上游冻结点：`14e0a49e17afebf62c5f788f4ef1dc8eef56ac76`。
- 选择性同步分支：`codex/upstream-sync-20260908`。
- 本项目继续以模块化后端、feature owner 和现有协议兼容边界为事实源，不直接恢复上游 legacy 目录或旧迁移号。

## 选择性处理

| 主题 | 当前 owner 与处理 | 边界 |
| --- | --- | --- |
| 协议与工具事件 | 将上游协议修复映射到现有 `application/service`、`shared/apicompat` 和 WS relay owner，并保留本项目既有的 turn 状态与错误边界。 | 不改变已提交语义事件后的重放和 failover 规则。 |
| 价格与用量 | 复用当前统一计费、模型目录和账号统计 owner，补齐本批需要的模型/价格行为。 | 历史账单、渠道优先级和实际扣费仍以现有 billing 管线为准。 |
| 备份与兑换 | 保留当前备份锁、异步任务和兑换时间窗口实现，在现有 repository/service 边界内接入修复。 | 不新增重复 worker，不把管理页面状态当作持久事实。 |
| WebSocket turn 状态 | 在当前 `openai_ws_v2` / passthrough 生命周期中维护终态、失败和后续事件边界。 | 终态之后的合成进度、注释或重复失败不会继续转发。 |
| 管理端与用户端筛选 | 将账号、订阅和用量筛选落到当前 feature datasource、DTO 和页面 owner。 | 服务端分页、权限和用户范围仍是最终约束，前端筛选不扩大可见范围。 |
| GPT-6 Astra 目录 | 更新现有 OpenAI/Codex 模型目录、说明和能力映射。 | 静态目录更新不自动授予账号能力，也不替代价格或调度配置。 |

## 升级与性能边界

- 本批不以文档或兼容移植为理由恢复 legacy `internal/service`、`internal/handler` 或 `frontend/src/views` 结构。
- 迁移、配置和缓存变化必须保持旧数据库可启动、旧客户端协议可解析以及滚动升级期间的状态兼容。
- `BenchmarkUpstreamSyncArgumentTracking` 的对比显示：基线字符串拼接约 `16.82-21.08 ms/op`、`36,099,648-36,099,661 B/op`、`1023 allocs/op`；有界摘要实现约 `84.33-92.59 us/op`、`1256 B/op`、`9 allocs/op`。这些数字只代表该 microbenchmark，不代表生产吞吐或端到端延迟保证。

## 验证结果

现有批次证据记录了以下结果：

- Docker Go unit、聚焦 race、前端 Vitest（357 个文件、2173 项测试）、typecheck、lint、build、`make check-docs` 和 backend layout 检查通过。
- PostgreSQL 18 + Redis 8 升级/回退循环中 `/health` 和 `/ready` 保持成功，migration 计数和最新文件名保持不变。
- 独立副本回滚后恢复到基线提交，修改副本仍保留变更；具体输出和镜像摘要见验证文件。

发布后的远端 SHA、Actions 和 GHCR 结果应在实际推送后追加，不以本地测试替代远端证据。
