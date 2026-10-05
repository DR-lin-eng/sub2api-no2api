# 2026-09-23 Codex 新版请求与 Sub2API 风控对照

## 对照基线

- 对照对象：用户引用的 Codex 更新任务 `01a0cc64-2d92-7f91-bffe-e875c4a8decc`。
- 参照 Codex HEAD：`b7add4df3d95e2d41249e54d3c3a1bd14680848a`。
- 参照最新提交：`Stabilize retry timing and WebSocket tests (#47437)`，时间为 `2026-09-23T03:08:43Z`。
- 当前 Sub2API：detached HEAD `f2284c586e8d182423b37eacf664d28b39cb41f5`，版本 `0.1.214`。

## 本次对齐结论

已把新版 Codex 中与能力授权有关的 `access_programs` 纳入 server-owned 风控边界：当前 Sub2API 没有公开 app-server 的 `cyber_access_program`、模型 entitlement 或 ChatGPT-auth 专用投影链路，因此客户端注入的顶层 `access_programs` 不再进入 OpenAI HTTP/WS 上游。

同时保留上一轮已经完成的 raw `x-codex-turn-metadata` 修复：无效 JSON 丢弃、workspace 脱敏、核心身份/沙箱/审批字段剥离、session/thread 账号隔离，以及 HTTP/WS 使用同一 fingerprint plan。

## 请求增量与风控边界

| 新版 Codex 请求/字段 | 当前 Sub2API 对照 | 风控结论 |
|---|---|---|
| `thread/start.serviceName` | 当前网关没有 app-server `thread/start` 路由；OAuth 出站身份由现有 UA/originator 配对和 `codex_cli_only` 门控处理 | `serviceName`、`originator`、`threadSource`、`turnTrigger` 都不能作为 Work entitlement。真实 Work/workspace/model entitlement 仍需上游服务端判定 |
| `turn/start.disabledPluginIds`、`thread/settings/update.disabledPluginIds` | 当前网关没有 Codex app-server plugin runtime，也没有插件能力投影/审批键存储 | 如果产品不提供 app-server 插件能力，这些请求不属于当前 `/v1/responses`/WS 表面；不能把客户端字段当作禁用或授权证据 |
| `turn/start.turnTrigger` | 当前只在 Guardian/memgen 语义头生成时做严格组合校验 | 已覆盖当前语义边界；`turnTrigger` 仅是有界观测，不授予权限 |
| `turn/start.toolOutput` | 当前支持 Responses `function_call_output` 和 WS/HTTP continuation；没有 app-server `toolOutput` 独立提交协议及“非空 input 互斥、name 非空”校验 | app-server parity 未实现；继续使用当前 Responses 工具续链时，需要保持现有 tool-call/continuation 校验 |
| `turn/start.serviceTierForTurn` | 当前支持公共 Responses `service_tier` 和 fast policy；没有 app-server 级的 turn-only tier 覆盖字段 | 当前账户/分组策略仍是最终约束；不能把客户端 tier 选择作为额度或权限证明 |
| `thread/start.daybreakEnabled`、`turn/start.cyberAccessProgram` | 当前没有对应 app-server RPC/字段投影；已有 cyber policy、会话阻断和 verification recommendation 观测，但不等于 entitlement | 客户端只能表达意图；不能授予 Cyber。顶层 `access_programs` 现在被 HTTP/WS relay 删除 |
| `response.metadata.openai_verification_recommendation` | 已识别 `trusted_access_for_cyber`，原样保留事件，不触发切号/重试/模型改写 | 已覆盖 |
| `client_metadata` 有界额外键/核心键 | full simulation 已有 16/64/128 限制与核心键投影；旧路径的 raw `x-codex-turn-metadata` header 曾可绕过 body sanitization | 已修复：OAuth header-only metadata 现在拒绝无效 JSON，删除 app-server-owned identity/sandbox/approval 字段，复用 workspace redaction 和有界 extra policy，并重新绑定 session/thread |
| 顶层 `access_programs` | 旧的 raw HTTP/WS passthrough 会保留未知顶层字段；当前没有 Sub2API 自己的 cyber entitlement 投影 | **本次已修复**：HTTP builder、HTTP passthrough、managed WS ingress、WS v2 pool path 和 WS v2 passthrough 的顶层 `access_programs` 都被删除；`input`/工具参数内同名用户数据不受影响 |
| `application network policy` | 当前已有 Workspace Routing 的 HTTPS origin、routing override、redirect fail-closed 和官方 discovery 边界；没有 Codex app-server 对 telemetry/executor/auth bootstrap 的全量 exact-host policy | 对当前 OAuth routing 已覆盖；若要宣称完整 app-server parity，仍需独立实现 exact-host allowlist、reload cancel、load-fail-closed |
| `account/rateLimits/read`、MCP event stream、plugin reconcile、project/thread list/timeline/realtime 等 v2 RPC | 当前没有这些 app-server 处理器 | 不影响当前 Responses/WS 转发的直接鉴权，但不能对外宣称已支持新版 app-server 协议 |

## 本轮继续完善：账号稳定伪装值

- `x-codex-installation-id` 不再接受客户端已有值；优先使用账号持久化的 `openai_device_id`，缺失时按 `CodexVirtualClientKey` 稳定派生。
- 没有客户端 session/thread 信号时，普通 OAuth 的 session/thread 使用账号稳定派生值，不再每个请求随机漂移。
- HTTP、HTTP passthrough、managed WS、WS v2 pool 和 WS v2 passthrough 的普通 OAuth body 都执行同一套 `client_metadata` 清洗。
- 客户端伪造的 turn/window/approval 字段仍不具备权限含义；需要模拟的 installation/session/thread 投影由服务端稳定重建。

## 本次修改

1. `backend/internal/application/service/openai_responses_access_programs.go`
   - 新增统一的顶层 `access_programs` 清洗器，HTTP 使用 JSON bytes 版本，WS map payload 使用 map 版本。
   - 只删除顶层 server-owned capability 字段，不扫描/破坏 `input` 或工具参数中的同名业务数据。
2. `backend/internal/application/service/openai_gateway_request_build.go`
   - OpenAI HTTP upstream builder 在请求体进入上游前删除顶层 `access_programs`。
3. `backend/internal/application/service/openai_gateway_passthrough.go`
   - passthrough 进入策略、计费和 retry body 前先清除顶层 `access_programs`。
4. `backend/internal/application/service/openai_ws_forwarder_ingress.go`
   - managed WS 首帧/后续 `response.create` 解析路径清除顶层 `access_programs`。
5. `backend/internal/application/service/openai_ws_forwarder_v2.go`
   - WS v2 pool payload map 清除顶层 `access_programs`。
6. `backend/internal/application/service/openai_ws_v2_passthrough_adapter.go`
   - passthrough 首帧和每个文本帧都应用同一顶层 capability 清洗。
7. `backend/internal/application/service/openai_responses_access_programs_test.go`
   - 覆盖顶层字段删除、嵌套用户字段保留、无效 JSON 和 map payload。
8. `docs/codex/intentional-divergences.md`
   - 记录 `access_programs` 不属于当前网关可验证授权证明的边界。

上一轮的 metadata/fingerprint 相关修改仍保留在同一工作区，见 `git diff` 和本目录验证产物。

## 仍未完善且会影响风控的事项

- **高优先级、需集成环境补证据**：当前本地 Docker 只能验证请求构造、编译、镜像运行和 mock/fixture 拒绝边界，不能证明真实 Work、workspace、model tier、Cyber entitlement 的上游拒绝路径。需要 staging/upstream integration capture 验证“伪造 `serviceName`/`originator`/`access_programs` -> 后端拒绝”。本次已在网关侧先行删除客户端 `access_programs`，但没有冒充真实上游 entitlement 结果。
- **中优先级、协议未对齐**：如果要兼容新版 app-server，而不只是 `/v1/responses`/WS relay，必须新增并授权 `thread/start`、`turn/start`、`thread/settings/update`、`thread/metadata/update`、`turn/settings/update` 以及 MCP/plugin/realtime 相关 RPC；当前没有这些入口。
- **中优先级、能力投影未对齐**：当前不具备 Codex app-server 的 plugin/MCP runtime projection 和带 `plugin_id` 的 approval key 隔离。若未来接入 hosted apps/MCP，需要在后端做 capability catalog、disabled-plugin state 和 approval persistence 的服务端约束。
- **中优先级、网络策略未完全等价**：Workspace Routing 的 fail-closed 不等于 app-server 的 application network policy。工具、telemetry、executor、auth bootstrap 的 exact-host allowlist 和 reload cancel 尚未在本项目形成同一策略链。
- **外部文档漂移**：参照任务报告指出上游 `codex-rs/app-server/README.md` 的 plugin filtering 说明滞后；这不是当前 Sub2API 代码缺陷，但同步文档时不能把旧说明当作运行时事实。

## 验证结论

- 新增 access-program focused tests：通过。
- Docker service/handler 全回归：通过；service `138.106s`，handler `31.045s`，其余 handler 子包通过，exit `0`。
- Docker `go vet ./...`：通过。
- Docker compile-all：通过。
- Docker 文档检查：通过。
- Docker `deploy/Dockerfile` 最终构建：通过，镜像 `sub2api:codex-risk-audit-20260923-final` 已生成，manifest `sha256:94d5ab0104c374b5fbd975177429b1d83f97ab25114c8b4af9540392f7aae602`。
- Docker 最终镜像运行时：通过；临时 PostgreSQL/Redis 依赖启动后，`/ready` 返回 `{"ready":true,...,"current_version":"0.1.214"}`，容器随后清理。
- fixture baseline/modified/rollback：全部 exit `0`；工作区 `MODIFIED_FILE.json` 保持修改状态。
- `git diff --check`：通过。
- 未对外宣称真实上游 entitlement 已验证；该项仍是集成环境待办。

详细命令、输入、字面输出、退出码和回滚证据见同目录 `VERIFICATION.txt`。
