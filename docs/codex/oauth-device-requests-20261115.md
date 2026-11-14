# OpenAI OAuth 证明与 Lenovo 归因（2026-11-15）

管理员设置 → 网关 → 请求转发行为提供两个独立开关：

| 开关 | 行为 | 默认 |
| --- | --- | --- |
| `openai_oauth_attestation_enabled` | 第一方 OAuth Responses/Compact HTTP 和 WS 握手在发送前生成 `x-oai-attestation` | 未保存时沿用 `gateway.openai_oauth_attestation_enabled`，部署默认关闭 |
| `openai_lenovo_attribution_enabled` | 使用中的普通 OAuth 账号在到期时独立请求 Lenovo 预装归因，并保存其响应和重试计划 | 关闭 |

两者通过已有管理员设置 API 持久化。保存后当前实例立即生效，其他实例最多 60 秒刷新；请求热路径共享进程缓存。旧前端或部分更新省略字段时保留原值；GET 与 PUT 的响应均回填保存结果。关闭 Lenovo 开关后不再启动获取，发送前再次检查开关。已发送的请求可完成并保存结果。

## 请求与响应

Lenovo 归因使用独立的 `POST https://chatgpt.com/backend-api/incentives/lenovo/preload-attribution`，JSON body 包含当前账号首次生成并持久化的 `installation_id`。不同本地账号拥有不同安装 UUID，即使它们共用 workspace 或 Codex 虚拟身份。请求使用该账号当前 OAuth token、账号头及既有代理/IPv6 出口，15 秒超时、禁止重定向，不刷新 token，不参与模型计费或模型账号错误策略。

只有已保存合法 64 位十六进制 `extra.lenovo_device_serial_digest` 时才发送 `device_serial_digest`。该摘要应来自真实序列号按 App 协议计算的结果；本功能不采集原始序列号、不生成虚构序列号或摘要。没有摘要时使用报告中的 installation-only body。

`X-Lenovo-Device-Recorded` 由上游返回，处理范围严格限定为本次归因 POST：

| 结果 | 后续行为 |
| --- | --- |
| 204，已发摘要，单个响应头严格等于 `1` | 保存已记录状态，7 天后可重查 |
| 204，已发摘要，头缺失或其他值 | 保存未记录状态，6 小时后可重查 |
| 204，未发摘要 | 保存本次完成状态，不建立序列号重查计划，也不声明设备已记录 |
| 其他小于 500 的状态（含 408/429） | 停止当前身份的获取；不把此错误应用到模型账号 |
| 5xx / 网络异常 | 保存重试计数和下次时间；延迟依次 0、1、5、30、900 秒，之后保持 900 秒 |

新请求活动触发到期任务，没有常驻的每日注册循环。全实例最多 8 个后台获取任务、4096 个本地调度缓存项，先准入再创建 goroutine，不建立无界队列。

## 持久化与隔离

`accounts.extra.openai_lenovo_attribution` 保存版本、安装 UUID、账号/用户/摘要的哈希绑定、HTTP 状态、合法响应值、检查时间、重试计数/时间、终止状态和请求租约。不保存 access token、raw serial 或摘要本身。两个开关保存在 `settings` 的同名键中。

发送前先用 PostgreSQL 原子 CAS 固定安装 ID 和 45 秒租约。其他实例只能复用已存结果或等待租约过期；进程中断不会每次重启更换 UUID。返回后再次比较凭据、代理、摘要和租约，迟到响应不能覆盖新身份或另一任务的结果。写入故障时不发送；完成状态保存故障时保留有限租约等待后续恢复。

普通账号编辑在行锁下保留当前归因状态；通用 Extra 补丁和批量编辑不能用陈旧副本覆盖此状态，新建/复制账号不导入别人的归因结果。凭据主体或摘要变化后重新获取，安装 UUID 保持该本地账号的持久身份。影子账号、PAT、Agent Identity、SIWC、API Key、未知 account/user 以及自定义中转不参与此功能。

没有新增数据库列或迁移。滚动升级时，应在全部节点升级后再开启归因功能；旧节点不认识本版本的 Extra 写入保护。回退前关闭两个开关；保留的账号状态可供再次升级后恢复。

## 与最新报告的关系

依据用户提供的 20261115 报告第 23/24 节及 Lenovo 专项记录，分析对象为 App `26.1111.21308 / build 27194`、Mac CLI `0.169.0-alpha.11.1`。原报告第 1—23 节保留历史版本结论，第 24 节才是本版本差异；不能用字面量计数变化推断 attestation/WS 已被删除。

App 的归因自动触发条件包含 Windows、打包应用和 Lenovo partner。当前网关由管理员独立开关主动启用兼容请求，不实现 Windows partner 探测，也不把虚拟安装 ID 当作真实 Lenovo 硬件证明。真实设备注册、奖励资格和上游验收仍由服务端决定。

本版本没有提供新的 `x-oai-attestation` 算法；维持宿主 opaque token 的 `{v,s,t?}`、100ms 预算和 Linux unsupported CBOR。`s=0` 是回调成功，不能当作 Apple 证明成功。`X-Codex-Surface` 与 `X-OpenAI-Attach-*` 是本地控制字段，出站前清除。Cloud Portals、checkpoint、Lenovo 响应头和远程 app-server WS 响应字段不注入普通模型请求。App Images 与归因 POST 使用更新后的桌面 surface 版本，Rust engine UA 独立管理。

## 验证

在 `backend/` 执行：

```sh
go test -tags=unit ./internal/modules/lenovoattribution ./internal/platform/liveattestation ./internal/application/service ./internal/transport/http/handler/admin
go test -tags=integration ./internal/infrastructure/repository -run 'TestLenovoAttributionPostgresCASRestartAndStaleEdits'
```

在 `frontend/` 执行：

```sh
pnpm exec vitest run src/features/admin-settings
pnpm run typecheck
pnpm run lint:check
```

构造/本地 HTTP 测试与 PostgreSQL 回归使用合成账号和响应，不能作为真实 OpenAI 接受该归因、成功设备记录或资格的证据。
