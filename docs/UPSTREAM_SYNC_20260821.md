# 上游同步审查记录（2026-08-21）

本次审查以本项目远端 `origin/main` 的 `b9f7a552174a9280148281dc4ceb42ff18549654` 为发布基线，将上游 `Wei-Shaw/sub2api` 的 `main` 冻结在 `4033387fd49b15b676d2e5c9fd3833f156050f57`。先前已审查边界是 `2bc139ab527b4a687546d145dc7bb9063cf14510`，本次增量包含合并 PR #5925（Grok compatibility）及其 28 个主题提交；更早的同步区间见 `82f7dd14f717bef480879f73cba288791b9b9663`。

## 选择性同步

以下语义已映射到当前模块 owner：

- #5801：缓冲 Chat Completions SSE 在首个终端事件前发生 HTTP/2/EOF 读取故障时，转换为稳定的上游流故障并允许账号 failover；Messages 路径继续返回原始读取错误。
- #5810：新增 `POST /v1/responses/input_tokens`（含 `/responses` 和 Codex alias）。官方 OpenAI 账号优先调用上游；自定义 relay、Grok 和不支持该端点的账号使用本地 tokenizer 估算，不进入生成/计费流水线。
- #5815：Grok 4.6/4.6-latest 保留 `xhigh`，旧模型将 `xhigh` 降为 `high`，无效值 fail-closed。
- #5844：当前请求已经包含内联图片时，移除 Grok 自动 `view_image` 工具；显式选择或历史轮次不受影响。
- #5845：WS HTTP bridge 后续轮次的 429 在下游语义输出前允许跨账号重试；重试 payload 去除 `previous_response_id` 并重建已完成输入。重试历史受 512 项/2 MiB 上限约束，超限时关闭 failover，避免无界内存和不安全重放。
- #5847：请求头覆写资格扩展到 Kimi/Zhipu/DeepSeek API-key 账号；认证、逐请求会话和 WebSocket 头仍由阻断名单保护。
- #5834：代理出口探测支持有序自定义 URL/parser（`ip-api`、`ipify`、`chatgpt-trace`），最多 8 个目标；空配置保持原有内置回退。
- #5868/#5881：tool-search 输出统一为字符串 `function_call_output`，完成的发现工具提升到有效函数目录并做同名 schema 冲突拒绝；请求体上限和原有 SSE 转换边界保持不变。
- #5729：Responses→Chat bridge 在链式工具调用中回放本轮 `reasoning_content`，不增加外部 I/O。
- #5876：本地 `model_not_found` 标记为模型配置业务限制，并清理同一请求中陈旧的账号/上游错误标记，避免错误计入 SLA。
- #5925（兼容性子集）：将 OpenAI 图片 `size` 在现有 `grok_media` owner 内转换为 xAI `resolution`/`aspect_ratio`，保留原始 `size` 作为本地计费输入；显式 xAI 几何字段优先，multipart 与 JSON 共用同一转换。

## 不重复或暂缓

- #5762 的“统计四次扫描改为一次 `GROUPING SETS`”没有直接移植：当前 owner `usage_log_repo_stats.go` 已经使用一次端点组合聚合查询，并已有 requested/mismatch 索引与缓存/rollup 优化；再次套用上游 SQL 会恢复旧目录结构。
- #5816/#5817 的 Composite CN provider 扩展依赖本项目尚未采用的独立 `kimi`/`zhipu`/`deepseek` 平台常量和迁移编号。固定增加调度桶会把每组快照从 12 个扩大到 18 个，增加 Redis/内存和重建写放大；本次只保留协议兼容审查，不引入半套路由。
- #5842 的 adaptive CN API protocol 会改变新建账号默认协议并引入多端点连接测试，属于存量配置行为变化，暂不在没有完整 CN 路由 owner 的情况下合入。
- #5851 的可配置 Fast/Flex 和长上下文倍率涉及存量账单变化及新的 schema 字段。本项目已有 `ApplyServiceTierMultiplier` 设计，已覆盖渠道标准价与 tier 倍率；不重复引入上游 migration 228 或把分组长上下文门控从 AND 改为 OR。
- 纯测试/样式或不同前端 owner 的 #5837、#5838、#5839、#5875 未复制旧 `src/components` 路径。
- #5925 的其余目录/默认模型与存量价格迁移、独立 reasoning token 计费、Realtime 预握手、Grok 429 冷却/同号重试、compaction 分类和连接池配置没有直接合并：这些改动依赖上游旧的 `internal/handler`/`internal/service` owner，且会改变现有默认模型、账单或账号状态。当前项目已有等价的加密内容重试、内容策略拒绝、流式 idle、有界 WS 重放和模型级容量边界；重复移植会造成双重重试或滚动升级行为漂移。

## 性能与升级影响

新增热路径逻辑均有边界：`responses/input_tokens` 对自定义 relay 避免一次必失败网络请求；WS 跨账号重试历史有项目级项数/字节上限；tool-search promotion 只在请求声明 `tool_search` 时解析 JSON；代理探测目标数量有上限；Grok 图片几何转换只做一次有界 JSON 解析和固定候选比例扫描，没有外部 I/O。没有新增数据库迁移、后台定时任务或无界队列。旧 API JSON/SSE/WS 形状保持兼容，旧配置在空 `proxy_probe.urls` 时行为不变；#5925 被暂缓的默认模型、账单和冷却策略不会在滚动升级中突然改变。

## 验证与发布入口

代码与测试是最终事实来源。提交前执行根目录文档检查、后端 Docker 测试、前端 typecheck/Vitest、生产镜像启动检查。发布后以推送提交的精确 SHA 分别核对 `CI`、`Security Scan` 和 `Docker Image`，不得用本地 Docker 成功代替远端 Actions 证据；上游审查边界固定为 `4033387fd49b15b676d2e5c9fd3833f156050f57`。
