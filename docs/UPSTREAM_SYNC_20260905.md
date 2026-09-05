# 上游同步审查记录（2026-09-05）

> 本页保留当天两个独立批次。当前批次为下文“第二批：31 个主线 PR”；第一批证据只属于其原始冻结点。

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


---

## 第二批：31 个主线 PR（当前批次）

### 冻结边界与处理规则

- 初始 fork 基线：`0741f5d103b1e0daebafd68fa9ba845dbd257fe7`，版本 `0.1.193`。发布前主线新增续接账号归属修复 `71da6ec33bc08edb44c9cc581feb65e9486101c6`，已正常合入并保留；最终文件回滚和镜像升级基线均以该提交为准。
- 上次已关闭上游：`b1748c4ea99ce2120401a269142aa071e18a84da`。
- 本轮功能冻结点：`578785ee7fb35030b094b69624efe25670a36f5f`，76 个提交、31 个 first-parent 合并 PR。
- 2026-09-05 再次 fetch 后仅新增 `ab99d56e9626e6cd731592dae8553c9758a0efa2`，只把上游 VERSION 从 `0.2.0` 改为 `0.2.1`。已审核该版本差异并保留 fork 版本 `0.1.193`；功能冻结点仍为 `578785ee`，最终 ancestry 关闭到已审查的 `ab99d56e`，不接入其 VERSION 内容。
- 原工作区不变；选择性移植到当前 owner，禁止恢复 legacy service/handler/views 树、赞助内容或旧迁移号。
- “关闭”表示审查决策已记录，不表示暂缓功能已实现。暂缓项仅在本表重新打开条件满足时单独处理，避免重复扫描已重构代码。

### 选择性移植与性能审查

| PR | 当前 owner 与处理 | 性能及兼容性 |
| --- | --- | --- |
| #6580 | `frontend/src/common/widgets/layout/AppSidebar.vue` 显式展开/折叠覆盖 active-route 默认值 | 单次 Map 查询，无路由或持久格式变化 |
| #6599 | `transport/http/handler/gemini_v1beta_handler.go` 在认证和平台验证后本地返回启用的自定义模型列表；分组编辑器提示 `/v1beta/models` | 自定义路径省去账号租赁和上游 I/O；强制 Antigravity 路由保持原行为 |
| #6594 | `infrastructure/repository/ops_repo_metrics.go` 为度量整数使用专用 nullable helper | 指针 nil 保留 NULL，显式 0 写入 0；ID helper 不变，无 schema 变化 |
| #6531 | `application/service/openai_messages_continuation.go` 精确识别 unavailable-for-user 续接错误 | 复用已有一次回放边界，不增加重试次数 |
| #6550 | `payment_order_lifecycle.go` 已有定时协调同时覆盖 Alipay/WeChat；保留旧方法包装 | 单次查询仍 LIMIT 20、既有顺序处理；不新增定时器，不改变已支付幂等与退款规则 |
| #6581 | `openai_opencode_session.go` 在请求构造、passthrough 和 Chat fallback 应用 caller session header | 无 header 时立即返回；仅 API-key + HTTPS opencode.ai；其他上游不转发 |
| #6606 / #6640 | `shared/claude/cli_version.go` 启动时解析可选 CLI 版本；billing/header/identity 统一版本及三位后缀 | 未设置保持内置版本；拒绝低版本和非三段稳定 semver；只有已识别 billing 块重算，普通 body 不哈希；messages/count_tokens wire body 回归 |
| #6553 | `shared/apicompat/codex_bootstrap.go` 严格识别 heartbeat bootstrap | 沿用既有长度、重复字段、call-id 和历史上下文约束；XML 只接受一个 automation_id、无属性和额外节点 |
| #6593 | `shared/apicompat/chatcompletions_responses_bridge.go` 推广完成的工具发现并保留 tools 形态的历史结果 | 不采用上游再次解码整个历史的实现；只解码 discovery 项、复用当前冲突与去重规则；本函数用 byte trim 去掉两次整段字符串复制 |
| #6507（部分） | 当前 GLM Chat owner 保留 GLM-5.3 显式 low | 仅已映射 GLM-5.3 命中，不改变其他 GLM 或无显式参数的请求 |
| #6636 | WS ingress、V2、HTTP bridge、passthrough 统一记录 error/response.failed；handler 按逻辑 turn 保留记录 guard | 首写获胜、换号不重复记账；仅当前已存在 failover 链继续，完成后仍屏蔽后续 turn；错误 body 先截 4096 字节再复制 |

### 已有实现，差异关闭

| PR | 事实来源 / 保留行为 |
| --- | --- |
| #6492 / #6536 | 当前 channel mapping / scheduler owner 已区分 requested、mapped、canonical model；保留别名调度与本项目的渠道规则，不替换为 legacy 算法 |
| #6397 | 保留当前 WS replay 独立字节所有权、512 项 / 2 MiB 上限；不引入上游 unsafe 共享切片别名 |
| #6529 | 当前 passthrough 在普通 reasoning 规范化前转入独立透传链路；不追加上游旧函数或重复重写 none |
| #6620 / #6626 / #6628 / #6572 | GPT-6 Astra、none/minimal -> low、prompt cache 与能力补齐已经在本项目相应 owner 和测试覆盖；保留之前性能短路重构 |
| #6539 | 共享 `codex_bootstrap` 已允许明确 FCO bootstrap 与合法历史上下文并存；已有 delegation historical-context 回归 |

### 暂缓 / 部分移植的重新打开条件

| PR | 原因与重新打开条件 |
| --- | --- |
| #6535 | 定价文件 hot reload 需与此前 #6353 的价格覆盖优先级、远端目录和历史账单共同设计；价格 owner 方案及对账矩阵确认后重开 |
| #6602 | 账号 pinned manifest 涉及 Group schema、auth snapshot 和缓存契约；独立迁移及旧缓存回退方案齐备后重开 |
| #6514 | reasoning pricing 涉及账务字段、迁移与历史账单；按 fork 新迁移号完成升级及降级对账后重开 |
| #6555 | upstream request ID 持久化与当前 usage / Ops owner 和迁移历史交叉；统一字段契约后重开，不直接搬上游生成代码 |
| #6638 | 图片 URL 回填需要接入当前 egress、URL allowlist、媒体任务大小和下载超时边界；独立资源预算设计后重开 |
| #6590 | 当前 manifest 代理按实际选中账号和上游能力传递，不采用上游跨账号生成 catalog；在本地生成 catalog 的 owner 正式引入时再评估 capability 推断 |
| #6542 | 批量改并发错误 code 涉及本项目 priority admission、Responses SSE、Anthropic 和 Gemini 的共享调用方；协议矩阵单独验证后重开，保留现有错误格式 |
| #6510 | 上游按 session hash 无条件删除注册，缺少当前并行请求租赁/代际所有权；同一 session 另一成功请求可能仍占用。需 token/compare-delete 释放设计、并发 race + Redis 验证后重开，不引入该竞态 |
| #6507（Anthropic 部分） | 上游 native Anthropic forwarder 在本项目没有同构 owner；其强制 thinking=enabled 会改变显式 disabled 语义。待当前 CN transport 契约设计后接入 |
| #6571 | ultrafast 同时改变服务等级、计费倍率和管理员策略；不当成静态清单变更，独立费率/开关/旧客户端矩阵确认后重开 |
| #6557 | 上游 AccountListItem/lite 与本项目按需详情、分组和管理动作状态契约不一致；独立测量大账号列表载荷、查询及详情动作后重开 |

### 平滑升级约束

- 本批不新增数据库迁移、不改 Ent/Wire 生成结果、不改默认版本或数据库内容；新环境变量为空时保持旧默认值。
- 支付协调复用原 worker 和上限，不新增每请求数据库访问、后台 goroutine、ticker 或无界缓存。
- 基线 `activitycenter/campaign.go` 1204 行触发现有 1200 行门槛。只将类型/状态两个纯判断函数原样移到 `campaign_state.go`，不放宽检查规则。
- 发布前分别验证 Docker unit、前端、lint、性能、独立 PostgreSQL/Redis 的升级/回退及文件 rollback；性能数字只解释对应 microbenchmark，不能以测试总耗时代替真实吞吐证据。

### 本批验证和发布

最终命令、输入、字面输出、退出状态、文件 hash、Docker 升级/回退保存到 `diagnostics/upstream-sync-20260905/VERIFICATION.txt`。发布 SHA、exact-SHA Actions 和 GHCR manifest 在独立发布记录中另行核对；本地测试不代替远端发布证据。

- 本轮回滚 patch 以发布前最新 fork 主线 `71da6ec33` 为基线，撤销本轮选择性同步时保留其 continuation owner affinity 修复。
- `openai_gateway_websocket.go` 是主线新增修复与本轮同步的唯一重叠文件；保留首包 bootstrap 规范化、账号归属约束、账号重检，以及本轮 cyber-policy logical-turn 状态清理，合并后重新运行完整 unit 和交叉路径 race 测试。
- 256 KiB 历史 microbenchmark 在同机交错运行；记录全部样本范围，不把共享 Docker 宿主的单次延迟或 suite 总耗时当作生产吞吐保证。
