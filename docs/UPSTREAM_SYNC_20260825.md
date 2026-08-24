# 上游同步审查记录（2026-08-25）

## 审查边界

- 本项目源码基线：`origin/main` `e647956a5e468e098d1a070e43691b4150b433f6`。
- 上次已记录的上游冻结点：`d45135d87df16d48637f04ccd245727bc955ba54`（PR #6068）。
- 本轮上游主线冻结点：`e2d9b823f63dc4e8f4014be3fd24a0a73e339867`（v0.1.181）。
- 范围：`d45135d87..e2d9b823` 的 33 个主线合并 PR、79 个提交。先按当前模块化 owner、协议行为、迁移占用和热路径成本审查，再做窄范围语义移植；不把上游旧 `internal/handler`/`internal/service` 树整体快进到本项目。

## 选择性接入

下列上游 PR 的行为可落到当前 owner，已按本项目命名、边界和兼容约定重写：

| 上游 PR | 当前 owner / 处理 | 性能与升级结论 |
| --- | --- | --- |
| #6116 Gemini tool schema | `application/service/gemini_messages_compat_service.go`：移除嵌套 `deprecated`，把 JSON 标量 enum 归一为 Gemini 字符串 enum，复杂值丢弃 | 只在已有 schema 清洗边界做有界递归；无 I/O、迁移和队列 |
| #6150 Grok CLI identity | `shared/xai` 与 Grok gateway：CLI pin `0.2.120`，统一 workspace UA，保留 operator override 与官方代理最终 header | 仅改出站身份版本；没有新增请求或重试 |
| #6143 rejected status | `application/service/openai_responses_rejected_field_retry.go`：同类型 input item 一次清理全部被拒 `status`，无类型时只清理报告索引 | 将多次重试压缩为一次，仍受原 6 次预算和 body hash 去重约束 |
| #6148 / #6084 Responses Lite tools | `openai_responses_lite_tools.go`：校验 `parallel_tool_calls` 类型；存在 top-level 或 `input.additional_tools` 工具时固定为 `false`，错误返回准确 `param` | 一次有界 map/数组扫描，避免上游 400 和重复 failover；不改变非 Lite 请求 |
| #6081 deferred tools | `shared/apicompat` 与 Grok tool sanitizer：没有 `tool_search` 时去掉孤立 `defer_loading` | 避免兼容上游 schema 拒绝；不改变带 `tool_search` 的声明 |
| #6139 model-list read limit | `platform/config`、`application/service`、`shared/antigravity`：新增 `gateway.models_list_read_max_bytes`，默认 8 MiB，覆盖 OpenAI Codex manifest、普通模型同步和 Antigravity quota probe | 保持 `LimitReader(max+1)`，防止控制面响应造成内存放大；旧配置缺字段时使用安全默认值 |
| #6118 streaming output items | `openai_gateway_response_handling.go`：缓存 `response.output_item.done` 原始 item，终态 output 缺失/截断时按 `output_index` 重建 | 每个流只保留有界 done item map；不增加上游调用，保留未知字段和 item identity |
| #6080 Chat tool-call identity | 新增 `openai_gateway_cc_tool_call_identity.go`，在 raw Chat SSE 转发前删除后续 delta 的空 `id`/`function.name` | O(1) 字符串快速门 + 有 tool-call 时有界 JSON patch；不触碰非流式响应 |
| #6061 cgroup memory metrics | `ops_metrics_collector.go`：cgroup 无具体 limit 时整体回退 host tuple，禁止 container used/host total 混算 | 只修正观测值，消除 Docker 下虚假的低内存占用；不改变调度或请求路径 |
| #6073 proxy IPv6 parser | `features/admin-proxies/presentation/pages/ProxiesPage.vue`：批量代理解析支持带方括号 IPv6 | 前端本地解析，无额外请求；裸 IPv6 仍拒绝以避免 host/port 歧义 |
| #6122 DOMPurify | 前端依赖与 workspace lock 升至 `3.4.14`，补齐 pnpm override | 安全依赖更新，不改变运行时页面协议 |

## 已处理、重复或暂缓

以下差异完成了函数级/路径级审查，但没有重复移植：

- #6079 Codex analytics/parent affinity、#6127 OAuth transport plugin：引入大块旧调度、隐私和 transport owner；当前已有 Codex identity、continuation、WS pool 和 scheduler 实现。整体移植会改变账号选择、缓存亲和及多实例行为，暂缓。
- #6121 auto-reset credit、#6111 request billing、#6137 CN Anthropic usage billing、#6089 weekday pricing、#6109 model plaza/billing：涉及上游独立账单/用户产品 owner 或大量旧迁移；本项目已有不同结算边界，暂不改变滚动升级口径。
- #6129 fast service tier：当前 `application/service/openai_fast_policy*`、WS/Chat/Responses 转换和计费观测已有等价实现，按行为和测试确认后跳过。
- #6119 Go 1.27：只是上游工具链/生成代码/CI 大升级；本项目锁定 Go 1.26.6，避免在功能同步中引入不可逆工具链漂移。
- #6095 OAuth upstream model sync、#6124 scheduler exclusion diagnostics、#6060 global WS `force_http`、#6075 user concurrency UI、#6117 account priority UI、#6133 Ops error detail UI：当前模块化主线已有等价 owner 或产品差异；保留本项目现有行为，不复制旧页面路径。
- #6136 flaky tool-schema allocation 仅改上游测试 fake；不进入生产树。
- #5905 custom tool alias、#6081 之外的 deferred/tool-search 变体、#6116 之外的 Gemini provider 变更：当前桥接已有 custom/namespace/tool-search 兼容；仅接入能证明不改变现有输出契约的窄修复。

## 升级与性能边界

- 本轮没有新增数据库迁移、后台任务、无界队列或普通请求外部 I/O。
- 所有新增 body/schema 操作都有明确上限：模型列表默认 8 MiB、Responses rejected retry 复用原 6 次预算、stream done item 仅保留当前响应的 output index、工具扫描按输入数组长度线性执行。
- 旧 JSON、SSE、WebSocket、计费和账号调度契约保持不变；新配置字段缺省时回落安全默认值，旧 Compose/配置文件可直接滚动升级。
- `#6068` parent affinity 仍只作为历史审查记录，不以 tracking merge 掩盖未移植的旧调度行为；本轮 tracking merge 会在本记录发布后明确标注所有暂缓项。

## 验证入口

代码与测试是最终事实来源。提交前执行：

```sh
cd backend && go test -tags=unit ./internal/application/service ./internal/shared/apicompat ./internal/shared/xai ./internal/shared/antigravity ./internal/platform/config
cd backend && go test -tags=unit ./internal/infrastructure/repository ./internal/transport/http/handler ./internal/transport/http/server/routes
cd frontend && pnpm install --frozen-lockfile && pnpm exec vitest run src/features/admin-proxies/__tests__/ProxiesPage.ipv6.spec.ts
```

发布验证还包括 Docker 后端/前端构建、PostgreSQL/Redis `/ready` 回归、`make check-docs`、`git diff --check`，以及推送提交精确 SHA 的 `CI`、`Security Scan` 和 `Docker Image` workflows。任何本地 Docker 结果都不替代 Actions 证据。
