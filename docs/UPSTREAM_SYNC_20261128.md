# 上游主线增量审查（2026-11-28）

项目基线为 `559794d09`。上游 `81ce8356d..f53641e53` 有 104 个新增提交，按首父划分为 35 个 PR、一次回退和一次版本更新。每项的净 diff SHA-256、当前代码归属及重开条件见[同步台账](../diagnostics/upstream-sync-20261128/UPSTREAM_PRS.json)。继承的 2026-11-27 台账保持原始内容。

- Anthropic 兼容输出保留起始输出用量，把思考用量作为输出总量的明细。Chat 转 Responses 在截断时发送 `response.incomplete`，并按 ID 或下标区分并行工具调用。Compact 异常终态补客户端模型，保留路由别名。
- Antigravity 明确配额耗尽时先沿用现有积分规则。积分未接住时写入现有冷却并立即换号。裸 429 仍只冷却一个账号，换号继续使用既有排除、粘性和槽位释放路径。会话窗口采样复用现有 Anthropic 账号判断。
- 模型候选在请求字节上同步解析，跨阶段只保存独立的小字符串。OpenCode 标识按账号和目标复用，模型正文不进入会话标记。AI Studio 的兼容转换只覆盖明确的直连 Chat 目标，有签名的工具批次继续使用原签名。图像策略事件复用有限审核队列，封禁计数排除这些事件。
- Kimi Code 的 K3.1 Pro 和高速版补协议识别。本项目继续按渠道配置或动态价卡计费，把套餐消耗倍率与按量 API 价格分开。保留本地 Codex 提示词契约。上游更新器补丁和回退的净 diff 为空，本地签名校验继续生效。 [Kimi Code 模型说明](https://www.kimi.com/code/docs/kimi-code/models.html) · [Google 思考签名说明](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/thinking/thought-signatures)

升级和回退使用同一 PostgreSQL 与 Redis，不涉及 SQL 迁移或缓存键格式转换。混合节点运行期间，客户端可能收到旧节点的截断 SSE 终态，旧节点也按原策略处理 Antigravity 配额耗尽。图像审计事件沿用已有表和 action 字段，按审计记录保存，混合版本中的旧节点也不会将其计入封禁阈值。

[验证记录](../diagnostics/upstream-sync-20261128/README.md)
