# OpenAI OAuth 实时额度与用量详情

账号列表中当前加载页的每个 `status=active` OpenAI OAuth 账号各自建立一条 ChatGPT WS，额度卡可接收实时快照。`error`、`inactive` 等非正常状态不会建立连接，账号状态变为非正常时立即关闭。账号菜单的“使用统计”新增“上游用量详情”，展开后按需查询；本地请求、Token、成本统计仍使用本站用量日志。

## 实时链路

前端页面级连接管理器按账号 ID 维护 WS，不依赖表格单元格是否因虚拟滚动而挂载。后端管理员接口也复查账号状态，只允许 active 账号订阅；同一账号的多个观察者共享一条上游连接。后端先读取既有 `/wham/usage` 作为基线，再通过 `/celsius/ws/user` 获取签名连接地址，发送 connect、topic subscribe 和前台 presence，注册 `/codex/notification/subscriptions` 并按服务端周期续订。topic 为 `codex-work-usage-push-v1`，初始连接和快照等待共用 35 秒上限。

只接受 `usage.snapshot` v1 中的 `usage_by_client.electron`。账号、用户和套餐必须与认证查询基线一致；stream、sequence、generated/published 时间用于排除旧消息、重复及漏消息。身份不匹配、序号缺口、协议错误、订阅失败、连接断开或 presence 超时结束推送，界面继续原来的被动快照、主动查询和列表刷新。有效快照直接更新界面，不为每条推送额外查询 HTTP。

每个进程同一账号最多一个上游连接、最多 16 个观察者，进程最多维护 256 个账号订阅。最后一个观察者离开即取消连接；失败账号冷却一分钟，前端按 1–5 分钟退避重连。隐藏页面释放连接，恢复前台时重建。管理员连接最长 15 分钟，随后重新走已有认证中间件。跨实例各自维护订阅，没有新增数据库表或 Redis 队列。

额度窗口和 individual spend limit 的 reset 边界触发一次现有 HTTP 查询。快照的窗口投影最多每五秒合并写入账号 extra，供列表和调度使用；这不是计费结算队列。Spark 影子账号仍使用母账号凭据，影子行写入 Spark 窗口。签名 WS URL、token 和认证头不返回浏览器；只允许 ChatGPT/OpenAI 域下的 WSS 发现结果，其他地址使用原刷新路径。

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

已有门禁问题仍在：`make check-docs` 指向缺失的 `docs/PR169_MEDIA_STUDIO_REVIEW.md`；`make -C backend check-layout` 报 `pricing_service.go` 1218 行、`payment_fulfillment_test.go` 1201 行，均超过 1200 行限制。这些链接与文件长度在本次改动前已存在，相关业务文件未修改。
