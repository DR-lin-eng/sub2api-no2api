# 上游同步审查记录（2026-09-05）

## 审查边界

- 本项目发布基线：`origin/main` `bd8bf53551ce787759000fb72cfccf29fb755183`。
- 上次已关闭的上游冻结点：`52374af94031f04df8de6fc91deb77a179e04b06`。
- 本轮上游主线冻结点：`b1748c4ea99ce2120401a269142aa071e18a84da`。
- `52374af9..b1748c4e` 共 91 个提交、30 个 first-parent 提交，其中 24 个合并 PR。
- 本项目继续以 `transport -> application -> domain`、模块化 repository 和 feature owner 为事实源；不恢复上游 legacy `internal/handler`、`internal/service`、`frontend/src/views` 树，也不引入三语 README 的赞助内容。

## 选择性接入

| 上游 PR | 当前 owner / 处理 | 性能与平滑升级结论 |
| --- | --- | --- |
| #6438 / #6379 Codex 图片输入与 Fast 能力清单 | `application/service/openai_codex_models_service.go` 对 API-key 上游清单只补充已知 GPT 家族缺失的 `input_modalities` / `service_tiers`；显式字段（含空数组）保持权威 | 仅 `/models` 冷路径按模型 O(n) 处理；不新增 I/O、缓存或默认 Fast 策略 |
| #6433 WS 空闲连接回收 | `application/service/openai_ws_pool.go` 在既有 sweep 中回收无法无 reader 消费 pong 且空闲 90 秒的连接 | 不新增 goroutine/ticker；只遍历已有连接表，租赁、等待和 pinned 连接不回收 |
| #6435 API-key 空指令 | Responses API-key 请求不再注入 OAuth/Codex base instructions；OAuth 路径保持原行为 | 减少 API-key 请求体改写与固定提示词 token；无配置或协议迁移 |
| #6415 PostgreSQL 启动恢复 | `infrastructure/repository/ent.go` 只对 `57P03` 与 SQLSTATE `08*` 做最多 8 次指数退避；Compose 同时执行 SQL readiness | 仅启动路径；认证、迁移校验和数据错误继续立即失败，滚动升级不隐藏永久错误 |
| #6417 WS 容量降载码 | ctx-pool ingress 只改写已提交响应后发给客户端的副本，账号状态仍检查原始错误 | 常数分支；首输出前仍走本项目更强的透明 failover |
| #6394 账号统计计价 | 模型文件定价改为复用统一 `BillingService` 管线 | 去掉第二套公式，修复 image token 子集重复计价；没有查询或队列变化 |
| #6454 Anthropic fallback beta | `gateway_request.go` 一次读取四个 beta-gated 字段；缺匹配 beta 才剥离 `fallbacks` / `fallback_credit_token`，Bedrock 始终剥离不支持字段 | 普通请求无 JSON 重写；不默认开启会换模型或改账单的 beta |
| #6474 Fable 5.1（兼容部分） | 同步后端默认模型、Antigravity/Bedrock 映射、管理端白名单、用量标签和 OpenCode 模型描述 | 静态目录变化；不夹带该 PR 的渠道 1h 缓存价格 migration 与账务字段 |
| #6469 API-key Chat 缓存身份 | 缺显式 key 时复用既有稳定 seed，并在注入 body 前按 API Key 隔离；Responses-shaped body 不猜 key | 不新增网络/数据库访问；仅缺 key 的 GPT-5 Chat 路径计算一次 hash，后续轮次稳定 |
| #6470 WS 早关终态 | passthrough relay 在 active turn 未见终态时把正常 close/EOF 视为失败 | O(1) 状态判断；已有终态和空闲连接关闭语义不变 |
| #6480 模型定价布局 | 将修复映射到 `admin-groups` / `admin-channels` feature widgets：宽对话框、可换行标题和自适应价格栅格 | 纯布局，无请求、状态或数据契约变化 |

## 已有实现覆盖，不重复移植

| 上游 PR | 当前结论 |
| --- | --- |
| #6407 / #6450 delegation / automation bootstrap | `746440eb6` 已用共享 `apicompat.NormalizeCodexCallOutputBootstrap` 覆盖 HTTP、WS、passthrough，并比上游旧 handler 实现增加唯一成员和歧义校验 |
| #6432 disabled catalog accounts | 当前 `/models` 由调度器选择可调度账号，不构造上游 legacy 的跨账号 group catalog；持久停用账号不会被选择 |
| #6458 scheduler passthrough projection | `infrastructure/repository/scheduler_cache.go` 已保留 `openai_passthrough` 与 `openai_oauth_passthrough`，并有序列化回归测试 |
| #6460 `model_not_found` 被 cooldown 覆盖 | 当前 `DiagnoseModelAvailabilityForPlatform` 直接查询持久资格和模型支持，并在明确支持但全限流时才返回 429；本地 404 还会清除旧 attempt 的 Ops 上下文 |

## 暂缓项

| 上游 PR | 暂缓原因与重新打开条件 |
| --- | --- |
| #6353 定价目录重构与 override file | 涉及账务目录、远端覆盖、阶梯价和默认价卡；需单独核对历史账单、自定义渠道优先级和回滚，不夹带进兼容同步 |
| #6463 Kimi 原生 Responses | 当前 fork 的 CN provider 已按模块化兼容链路收口，上游实现依赖 legacy `PlatformKimi` / `api_protocol` owner；需独立设计 endpoint 探测、计费和前端协议迁移 |
| #6443 / #6444 分组 Fast 与免费 Fast | 新增 Group schema、auth snapshot 和真实计费策略；当前已有管理员 Fast policy，但没有等价的组级免费账务契约 |
| #6447 / #6425 推理强度 deny 与模型范围映射 | 新增持久字段、DTO 与较大 UI；需在当前 feature owner 内独立设计默认 downgrade、旧缓存反序列化和 composite 目标模型语义 |
| #6474 渠道 1h cache-write 自定义价格 | 上游 migration 号与本项目历史冲突，且会改变渠道/账号统计计价；若接入须从本项目下一迁移号开始并完成四张表的升级/降级对账 |
| #6179 Ops 代理归因 | 涉及全部上游 attempt 站点与持久 JSON 契约。当前队列已把每请求事件限制为 16 个并限制 body 大小；代理历史快照应作为独立 Ops 变更验证所有协议路径 |

## 性能与升级边界

- 请求热路径没有新增数据库查询、外部请求、goroutine、ticker 或无界缓存。
- WS 清理由已有 30 秒 worker 承担；新条件跳过 leased、waiter 和 pinned 连接。
- Anthropic beta 清洗使用一次 `gjson.GetManyBytes` 读取四个顶层字段，只有存在且 header 不匹配时才分配新 body。
- API-key Chat 缓存 seed 只在 GPT-5/Codex、缺显式 key、非 Responses-shaped 请求命中；复用既有解析后的 Chat DTO，不再反序列化请求。
- 本轮没有 Ent/Wire 生成代码、生产 migration、后台任务或版本回写；旧数据库和旧配置可直接滚动升级/回退。

## 验证结果

- Docker Go 1.26.6 与基线同范围 unit 通过：`application/service 177.571s`、`infrastructure/repository 5.160s`、`transport/http/handler 31.402s`、`shared/apicompat cached`；基线分别为 `181.701s`、`5.770s`、`32.740s`、`0.065s`。
- Docker 全量 backend unit（`go test -vet=off -p=1 -tags=unit ./...`）全部包通过；聚焦 race、go vet 与 golangci-lint 均通过，lint 为 `0 issues.`。
- Docker Node 24 / pnpm 11.17.0 全量 Vitest：348 个文件、2,135 项测试通过；宿主 typecheck、eslint、文档检查与 source-layout 检查通过。
- Docker 镜像 `sub2api-upstream-sync-20260905:review` 构建成功，manifest `sha256:fc66a2f4256a97cfe82d5a7325e227996a80e78f0c1ec65100102a9531720e13`，42,430,029 bytes；基线镜像 42,430,630 bytes。
- PostgreSQL 18 + Redis 8 首次启动后 `/health` 为 `{"status":"ok"}`，`/ready` 为 `ready=true,current_version=0.1.192,draining=false`；287 个 migration，最新 `236_add_custom_model_request_templates.sql`。同镜像重启及基线镜像切换到候选镜像后均 healthy，migration 数保持 `287 -> 287`。
- Docker benchmark 交错复核：WS pool baseline `1062-1134 ns/op, 320 B/op, 3 allocs/op`，修改后 `1056-1147 ns/op, 320 B/op, 3 allocs/op`；pricing 单缓存 baseline `297.6-307.2 ns/op, 192 B/op, 4 allocs/op`，修改后 `304.2-310.0 ns/op, 192 B/op, 4 allocs/op`。波动范围重叠，分配数不变。

## 差异关闭

功能提交通过 Docker、前端、布局、文档和真实 PostgreSQL/Redis 栈验证后，使用 tree-preserving tracking merge 记录固定上游 SHA `b1748c4ea99ce2120401a269142aa071e18a84da`。后续只审查该 SHA 之后的新提交；本表标记为已有实现或暂缓的项目仅在 owner、协议或重新打开条件发生变化时复查。

## 发布证据

远端 SHA、Actions 与 GHCR 证据在实际推送后补记。
