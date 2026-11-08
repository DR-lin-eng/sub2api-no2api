# OpenAI OAuth 实时额度与用量详情

正常的 OpenAI OAuth 账号在首次模型调用发出时进入后台额度同步，后续每次调用续期；距离最后一次调用满 10 分钟时关闭上游 WS。同步与是否打开管理页面、分页或虚拟滚动无关。账号菜单中的“使用统计”继续提供按需查询的上游用量详情，本地请求、Token 和成本统计使用本站用量日志。

## 后台实时链路

`OpenAIUsageSyncService` 在允许运行 worker 的节点随应用自动启动。启动后扫描正常账号，但只恢复 `last_used_at` 距当前不足 10 分钟的账号；从未调用或已经空闲的账号不会握手。HTTP 模型请求的实际发送路径，以及 Responses WS 的首轮/后续轮次，在发送时以进程内通知激活账号，不等待请求完成或计费落库。通知不会阻塞模型请求，也不会直接访问上游。

后续调用只更新最后活跃时间，额度查询、模型目录、管理页面访问、订阅续期和 WS 心跳都不算模型调用。账号变为非 active、被删除、凭据/出口变更时，后台扫描会停止或重建对应连接；只有最近 10 分钟有调用且保持 active 的账号才会继续同步。后台每 30 秒复核数据库、每秒检查空闲过期，本节点首次调用通过合并通知立即唤醒扫描。

握手队列最多同时执行 1000 个初始化流程，只有收到首个有效快照或该次连接失败才归还握手名额；已经建立的长连接不占握手名额。后台没有旧的 256 账号观察者上限，1000 个正常账号中未被调用的账号不会因为扫描或打开页面产生上游连接。连接失败按 30 秒到 5 分钟退避重试，活跃期结束即停止重试。

多实例通过 Redis 的 compare-and-renew 租约选出一个后台同步 owner，45 秒租约每 10 秒续期；失去租约或 Redis 续期出错会取消该 owner 的全部连接，其他 worker 节点可接管。首次调用还复用原有异步 last-used 批量写入，因此经其他节点处理的调用在批量落库及下一轮后台扫描后可被 owner 发现。仅本节点持有 owner 时可立即唤醒；跨节点发现可能延迟一个批量写周期加最多 30 秒扫描。`worker_enabled=false` 节点不负责上游同步，但仍可接收管理页面连接和记录调用活跃时间。

后端先读取 `/wham/usage` 并持久化基线，再通过 `/celsius/ws/user` 获取签名连接地址，发送 connect、topic subscribe、presence，注册 `/codex/notification/subscriptions` 并按服务端周期续订。topic 为 `codex-work-usage-push-v1`，初始连接和快照等待共用 35 秒上限；首个有效快照后进入独立于页面请求的后台连接生命周期。

只接受 `usage.snapshot` v1 中的 `usage_by_client.electron`。账号、用户、套餐和序号/时间戳必须通过校验。上游连接断开、解析失败或漏消息会结束本轮并进入后台重试；WS 失败时仍保留 HTTP 基线和原来的主动查询/被动刷新路径。快照最多每五秒合并写入账号 extra，写前复查 active 状态和 principal；Spark 影子账号使用母账号凭据并更新影子行的 Spark 窗口。这些写入是额度投影，不是计费结算队列。

管理端 WS 只是观察者，不能触发上游握手或延长账号活跃期，关闭页面也不会关闭后台连接。owner 本节点直接转发共享快照，其他节点只读取已持久化快照。管理员连接仍受现有认证、Origin 检查和 15 分钟生命周期限制；每账号最多 16 个页面观察者，此外保留一个后台持有者。应用退出时先停止后台同步，再关闭令牌、Redis 和数据库依赖。

额度 reset 边界会调用一次 HTTP 刷新，签名 WS URL、token 和认证头不返回浏览器；只接受 ChatGPT/OpenAI 域下的 WSS 发现结果。

## 详细查询

入口为管理端 OpenAI 账号的 `usage/query`，只接受固定资源名，不能传入任意上游 URL。

| 资源 | 数据 | 刷新与边界 |
| --- | --- | --- |
| `daily` | 个人每日模型用量、原始产品面归属 | 日期跨度不超过 90 天，1 分钟缓存/前台刷新 |
| `workspace_daily` | 工作区按模型的未缓存输入、缓存输入、输出与总 Token | 独立范围，model breakdown 和 codex/work modes，1 分钟 |
| `remaining_balance` | 工作区账单余额 | 每次打开/恢复前台刷新，前台每分钟查询 |
| `monthly` | 当前用户月度额度 | no-store/no-cache，每次挂载/恢复前台刷新，后台停止轮询 |
| `threads` | 会话费用、点数、分组与 Token | 最多 1000 个唯一 ID，每批 100 个、最多两批并行；特定 incomplete 503 二分重试，1 分钟 |
| `top_threads` | root/descendant 的 5h、周额度及余额消耗 | 最多 100 root、合计 1000 个不重叠 ID，5 分钟 |
| `plan_history` | 最近 7 天窗口消耗、模型/来源/触发方式/产品面分解 | 5 分钟，保留 coverage/accounting/approximate 信息 |
| `balance_history` | 300 或 10080 分钟窗口的余额观测 | 5 分钟，自动沿 cursor 最多取 20 页；可继续加载 |

只有选择工作区范围才调用工作区模型查询，不依据未知套餐名猜测权限。上游 401/403/404/503 显示资源不可用，不能解释为零用量。其他错误和分页中断显示查询失败，保留已有结果。

缓存按账号、凭据和出口隔离，最多 512 项，相同并发查询合并。Token、点数、美元和额度百分比保持独立；micros 金额展示时除以一百万，basis points 展示百分数时除以一百。`data_as_of`、`data_freshness_ts` 与查询时间分别显示。完整明细可展开，包含未知新字段与会话分组。

余额曲线最多采样 64 点，在观测之间沿用上次余额，仅在能推断重置时估计恢复满额，明确标为近似曲线。Top Chats 需要提供真实会话 ID：网关没有 Codex 客户端的本地 root chat/子代理目录，不会把本站 request ID 当成 thread ID。每行可提供主会话与逗号分隔的 descendant ID；界面可按 5h、周额度或余额消耗排序。

## 实现与验证

- 后端协议、连接和资源查询：`backend/internal/application/service/openai_usage*`。
- 管理入口：`backend/internal/transport/http/handler/admin/openai_usage.go` 与 admin routes，复用管理员认证和 Origin 检查。
- 前端：`frontend/src/features/admin-accounts/` 中的 usage datasource、composables、额度卡和详情组件。

仓库根目录执行 `make check-docs`；后端在 `backend/` 执行 `go test -tags=unit ./internal/application/service ./internal/transport/http/handler/admin ./internal/transport/http/server/middleware`；前端在 `frontend/` 执行 `pnpm exec vitest run src/features/admin-accounts`、`pnpm run typecheck` 和 `pnpm run lint:check`。

协议依据为提供的桌面静态分析报告和提取 JS。本地模拟服务端覆盖实际 WS 订阅、推送、断连回退及 HTTP 请求契约。这些证据不代表已连接真实账号的生产推送，也不证明所有套餐开放详细查询。

### 本次验证记录（2026-11-07）

- 本地 Docker 后端回归通过：application service、admin handler 与 middleware 中匹配 `Test.*(OpenAI|Codex|Quota|Usage|RequiredAdminPermission)` 的测试，包含真实测试 WS 的订阅/推送/断连、重新授权后的共享连接隔离、身份/序号校验、分页和两路批量查询。
- 本地 Docker 前端通过 63 个测试文件、532 条测试，以及全前端 typecheck 和 lint；覆盖失效连接回退、前后台切换、旧请求取消、重新授权、Token/点数/美元单位和近似曲线。新增额度字段的 DTO 另做类型检查。
- 逐账号连接回归覆盖 2 个 active 账号分别建连、推送隔离，以及 error/inactive 账号不建连；账号状态变为非正常时会关闭对应连接并清除行状态。
- 正式多阶段 Dockerfile 构建通过，镜像为 `sub2api:openai-usage-20261107`。在禁止外网的临时网络中，应用自动初始化 PostgreSQL/Redis 后进入 healthy。
- 10 项启动检查通过：health/ready、嵌入式前端及 JS 资源、加密凭据登录、新接口匿名 401，以及初始化管理员受现有 `ADMIN_COMPLIANCE_ACK_REQUIRED` 423 门禁保护。上游业务数据和 WS 行为由模拟服务端的 Docker 测试验证，启动检查没有访问真实上游账号。
- Playwright 使用 Docker 前端和模拟数据检查了桌面/手机布局、实时窗口、每日模型、余额/月额度、会话/子代理分组、金额换算、Top Chats 与曲线。

上述为首次交付时的历史验证记录。2026-11-08 的后台活跃窗口修订已通过 `make check-docs`。`make -C backend check-layout` 仍报 `pricing_service.go` 1218 行、`payment_fulfillment_test.go` 1201 行，均超过 1200 行限制。这些链接与文件长度在本次改动前已存在，相关业务文件未修改。


### 后台活跃窗口修订（2026-11-08）

Docker 回归覆盖：不打开页面时首次模型调用触发同步、页面退出后继续更新、断线自动重连、HTTP 基线回退、活跃期续期及空闲后关闭、1000 个正常账号中闲置账号不握手、1001 个近期调用账号最多 1000 个初始化并行且其余排队、多实例单 owner 和失去租约后退出、以及 Lua 续租的 owner 校验。测试用缩短的时间窗口验证相同生命周期，生产活跃窗口固定为 10 分钟。

首次调用通知覆盖 HTTP（含自定义中继模型端点）和 Responses WS 各轮；用量查询和模型目录不触发活跃通知。Wire 由 Docker 中的 `go generate ./cmd/server` 生成，清理链在令牌服务、Redis 和数据库退出前停止后台 WS。

本次最终验证：上述后台/用量/额度/续租/清理相关 Docker 回归全部通过，正式镜像 `sub2api:usage-background-20261108` 构建通过（含前端类型检查及嵌入构建）。`make check-docs`、`git diff --check` 通过；布局门禁仍只有前述两个既存超长文件。本轮未使用真实上游账号做线上连接验证。
