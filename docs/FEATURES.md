# 功能总览与文档索引

本文面向部署管理员、功能使用者和维护者，按当前实现登记功能入口。页面路径来自 [前端路由](../frontend/src/core/routes/index.ts)，接口以 [后端路由](../backend/internal/transport/http/server/routes/) 为准；目录 README 解释代码归属，专题文档解释配置和操作流程。发布记录和 `openspec` 提案不替代当前功能说明。

## 使用边界

- 浏览器用户 API 使用 `/api/v1` 和登录会话；模型请求使用网关 API Key。两类凭据不能互换，管理自动化另见 [Admin API](ADMIN_API.md)。
- 页面可见性受角色、站点模式、功能开关和分组权限影响。前端隐藏菜单不代表后端授权；具体访问以路由中间件和业务校验为准。
- 模型展示、模型能力声明、可调度账号和计费配置是不同层次。模型出现在目录中，不代表当前 Key 一定能调用。
- 下表的“嵌入”表示组件或数据能力，不暗示存在独立路由。精确参数和响应字段应继续追踪 datasource、DTO 和 handler。

## 用户与公共功能

| 功能 | 页面或使用位置 | 当前能力与文档 |
| --- | --- | --- |
| 首次安装 | `/setup` | 数据库、Redis 连接测试与管理员初始化：[setup](../frontend/src/features/setup/README.md) |
| 公共首页与法律文档 | `/home`、`/legal/:documentId`、未知路由 | 公开设置、受控首页内容、登录协议和 404：[common](../frontend/src/common/README.md) |
| 注册与登录 | `/login`、`/register`、认证回调 | 邮箱、OAuth、二次认证、会话恢复和密码找回：[auth](../frontend/src/features/auth/README.md) |
| 个人资料 | `/profile` | 密码、通知邮箱、身份绑定和 TOTP：[profile](../frontend/src/features/profile/README.md) |
| Passkey | 登录流程与个人资料内嵌 | WebAuthn 凭据注册、登录、重命名和撤销：[passkeys](../frontend/src/features/passkeys/README.md) |
| 用户仪表盘 | `/dashboard` | 本人用量、趋势、模型分布和 Key 统计：[dashboard-user](../frontend/src/features/dashboard-user/README.md) |
| API Key | `/keys`、`/key-usage` | 创建编辑、有序分组绑定、用量查询与客户端配置：[keys](../frontend/src/features/keys/README.md) |
| 可用分组 | Key 编辑器等内嵌 | 当前用户可绑定分组及专属倍率：[groups-user](../frontend/src/features/groups-user/README.md) |
| 用量记录 | `/usage` | 本人请求、费用、错误和筛选导出：[usage](../frontend/src/features/usage/README.md) |
| 订阅 | `/subscriptions` | 有效订阅、配额进度和平台用量：[subscriptions](../frontend/src/features/subscriptions/README.md) |
| 支付与兑换 | `/purchase`、`/orders`、`/redeem` | 购买、订单、支付恢复和卡密兑换：[billing](../frontend/src/features/billing/README.md)、[支付专题](PAYMENT_CN.md) |
| 邀请返利 | `/affiliate` | 邀请记录、返利额度和转入余额：[affiliate](../frontend/src/features/affiliate/README.md) |
| 公告 | 应用壳层内嵌 | 定向公告、弹窗队列、已读状态：[announcements](../frontend/src/features/announcements/README.md) |
| 活动中心 | `/activity-center`、详情与参与记录 | 抽奖、签到、兑换膨胀、自定义活动：[activity-center](../frontend/src/features/activity-center/README.md)、[活动专题](ACTIVITY_CENTER.md) |
| 在线客服 | `/support` | 长期会话、图片、回复引用、已读状态：[support-chat](../frontend/src/features/support-chat/README.md)、[客服专题](SUPPORT_CHAT.md) |
| 模型广场 | `/model-plaza` | 模型筛选、分组与价格展示：[model-plaza](../frontend/src/features/model-plaza/README.md) |
| 渠道与自定义页 | `/available-channels`、`/monitor`、`/custom/:id` | 可用渠道、状态页组合与受控自定义页面：[channels-user](../frontend/src/features/channels-user/README.md) |
| 只读渠道监控 | `/monitor/public`，登录状态页内嵌 | 可用率、延迟、模型时间线和共享视图：[channel-monitor-user](../frontend/src/features/channel-monitor-user/README.md) |
| 批量图片 | `/batch-image`，媒体工坊内嵌 | 批任务、重试、预览和下载：[batch-image](../frontend/src/features/batch-image/README.md)、[批量图片](BATCH_IMAGE_MVP.md) |
| 媒体工坊 | `/media-studio` | 图片生成、视频任务和批量工作区：[media-studio](../frontend/src/features/media-studio/README.md)、[配置与边界](CUSTOM_MODELS_AND_MEDIA.md) |

## 管理功能

| 功能 | 页面或使用位置 | 当前能力与文档 |
| --- | --- | --- |
| 管理仪表盘 | `/admin/dashboard` | 全站汇总、趋势和排名：[admin-dashboard](../frontend/src/features/admin-dashboard/README.md) |
| 用户管理 | `/admin/users` | 身份、分组、余额、平台额度和批量限制：[admin-users](../frontend/src/features/admin-users/README.md) |
| 分组管理 | `/admin/groups` | 分组、倍率、模型范围和组合路由：[admin-groups](../frontend/src/features/admin-groups/README.md)、[组合分组](COMPOSITE_GROUPS.md) |
| 上游账号 | `/admin/accounts` | 账号、授权、导入、批量操作、测试和额度快照：[admin-accounts](../frontend/src/features/admin-accounts/README.md) |
| 账号巡检 | `/admin/account-inspection` | 策略、手动或自动执行、异常与额度分布：[admin-account-inspection](../frontend/src/features/admin-account-inspection/README.md) |
| 代理 | `/admin/proxies` | 代理配置、导入和连通性测试：[admin-proxies](../frontend/src/features/admin-proxies/README.md) |
| IPv6 出口 | `/admin/egress` | 地址池、绑定、探测和 HE 隧道：[admin-egress](../frontend/src/features/admin-egress/README.md)、[出口专题](IPV6_EGRESS.md) |
| 渠道定价 | `/admin/channels/pricing` | 模型价格、分组关联和时段价格：[admin-channels](../frontend/src/features/admin-channels/README.md) |
| 渠道监控配置 | `/admin/channels/monitor` | 主动或被动监控、模板、手动测试和历史：[admin-channel-monitor](../frontend/src/features/admin-channel-monitor/README.md) |
| 自定义模型 | `/admin/custom-model-config` | 能力、精确或前缀匹配、请求模板：[custom-model-config](../frontend/src/features/custom-model-config/README.md)、[配置专题](CUSTOM_MODELS_AND_MEDIA.md) |
| 订阅管理 | `/admin/subscriptions` | 分配、批量分配、延期、撤销、恢复和额度重置：[admin-subscriptions](../frontend/src/features/admin-subscriptions/README.md) |
| 支付管理 | `/admin/orders` 及仪表盘、套餐子页 | 订单、履约重试、退款、套餐与支付提供商：[admin-orders](../frontend/src/features/admin-orders/README.md) |
| 卡密管理 | `/admin/redeem` | 生成、导出、状态维护和批量编辑：[admin-redeem](../frontend/src/features/admin-redeem/README.md) |
| 优惠码 | `/admin/promo-codes` | 注册优惠码、使用上限、有效期和使用记录：[admin-promo](../frontend/src/features/admin-promo/README.md) |
| 全站用量 | `/admin/usage` | 管理员请求明细、成本、筛选、导出和清理任务：[admin-usage](../frontend/src/features/admin-usage/README.md) |
| Ops | `/admin/ops` | 并发、流量、错误、日志、告警和结算队列：[admin-ops](../frontend/src/features/admin-ops/README.md) |
| 风控与入口拒绝 | `/admin/risk-control`、`/admin/security-audit/ingress` | 内容审核、入口拒绝记录与 Cloudflare 联动：[admin-risk-control](../frontend/src/features/admin-risk-control/README.md) |
| Prompt 审计 | `/admin/prompt-audit` | 审计端点池、策略、运行状态和事件管理：[prompt-audit](../frontend/src/features/prompt-audit/README.md) |
| 操作审计 | `/admin/audit-logs` | 管理操作记录、详情和现场 TOTP 清理：[admin-audit](../frontend/src/features/admin-audit/README.md) |
| 多实例 | `/admin/multi-instance` | 节点、负载、任务与滚动发布：[admin-cluster](../frontend/src/features/admin-cluster/README.md)、[部署专题](../deploy/MULTI_INSTANCE.md) |
| 备份 | 系统设置内嵌 | S3、备份、恢复、定时计划和图片存储：[admin-backup](../frontend/src/features/admin-backup/README.md) |
| 系统设置 | `/admin/settings` | 站点、认证、功能开关、网关策略与运行设置：[admin-settings](../frontend/src/features/admin-settings/README.md) |

管理端的公告、活动、客服和返利页面由上表用户功能中的同名 feature 共同持有，并非另建一套 owner。它们分别位于 `/admin/announcements`、`/admin/activity-center/campaigns`、`/admin/support` 和 `/admin/affiliates` 下。

## 网关与后台能力

| 能力 | 功能文档 | 实现入口 |
| --- | --- | --- |
| Anthropic、OpenAI、Gemini 等协议、SSE 和 WebSocket | [关键请求链路](REQUEST_LIFECYCLES.md) | [gateway 路由](../backend/internal/transport/http/server/routes/gateway.go) |
| 调度、账号选择、多分组与失败切换 | [代码地图](CODE_MAP.md)、[候选索引](SCHEDULER_CANDIDATE_INDEX_OPTIMIZATION_CN.md)、[CPA 号池](CPA_POOL_DYNAMIC_LOAD_BALANCING_CN.md) | [application service](../backend/internal/application/service/README.md) |
| 用量结算与余额、订阅投影 | [关键请求链路](REQUEST_LIFECYCLES.md)、[支付集成](ADMIN_PAYMENT_INTEGRATION_API.md) | [repository](../backend/internal/infrastructure/repository/README.md) |
| 异步图片提交、查询与内容 | [异步图片 API](ASYNC_IMAGE_TASKS.md) | [Images handler](../backend/internal/transport/http/handler/) |
| Codex OAuth 行为 | [有意差异](codex/intentional-divergences.md) | [请求链路](REQUEST_LIFECYCLES.md) |

垂直后端模块另有完整目录说明：[activitycenter](../backend/internal/modules/activitycenter/README.md)、[chat](../backend/internal/modules/chat/README.md)、[egress](../backend/internal/modules/egress/README.md)、[payment](../backend/internal/modules/payment/README.md)、[securityaudit](../backend/internal/modules/securityaudit/README.md)。并非所有功能都已经迁入 `modules`；存量服务仍以代码地图为准。

## 维护与验证

新增 `frontend/src/features/<domain>` 或 `backend/internal/modules/<domain>` 时，同步新增该目录的 `README.md` 并在本页登记。说明至少包括职责、真实入口、权限或状态边界、已有测试位置；不要为没有测试的目录编造验证命令。新增专题同时登记到 [文档中心](README.md)。

从仓库根目录执行：

```sh
make check-docs
git diff --check
```

文档检查覆盖目录 README 的存在、索引登记和维护文档的本地链接；它不证明页面交互、权限或业务行为已经通过运行测试。修改业务时仍须按 [代码地图](CODE_MAP.md) 和子目录说明运行对应测试。
