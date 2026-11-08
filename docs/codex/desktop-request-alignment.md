# OpenAI OAuth 与 Codex 桌面请求对照

## 2026-11-07 增量对齐

本轮来源为用户提供的 `20261107/逆向分析报告.md`，样本分别为 macOS
`26.1103.41825 / build 25679` 和 Linux `26.1103.32239 / build 25471`。
macOS 内置 CLI 仍为 `0.167.0-alpha.3.2`，其二进制与上一轮一致；两平台不能视为同一 build。
已读取报告和认证证据的哈希见 [来源清单](../../diagnostics/desktop-alignment-20261107/SOURCE_HASHES.json)，
Docker 验证与限制见 [本轮记录](../../diagnostics/desktop-alignment-20261107/README.md)。

| 报告中的行为 | 当前适配 |
| --- | --- |
| Codex 凭证连续性还比较套餐类别 | 请求构造、主动刷新和 401 恢复同时校验已知账号、用户、个人/工作区类别。Team 与 Business/Enterprise 等已确认 workspace 类别可兼容，Plus→Pro 可兼容；跨类别返回 `account_changed`，当前轮次不在新身份上重放。空 claim 不补造身份；未知非空套餐不归入 workspace 集合。 |
| 访问 token 自身的 `exp` 与 300 秒提前窗口 | 结合当前 bearer 的 JWT `exp` 与对应凭据的 `expires_at` 取较早值，刷新判断和缓存 TTL 共用该期限。请求路径窗口由 3 分钟改为 5 分钟，旧缓存内临近/已过期 JWT 重新进入既有带锁刷新。旧账号快照的期限不用于否定另一条更新的缓存 token。 |
| 刷新和现有 WS 的身份分别维护 | WS 兼容键增加套餐类别；相同 opaque bearer、账号及用户下的类别变化也重建空闲连接。同类别套餐升级不因套餐名称变化而强制换连接。HTTP、SSE、透传和 WS 继续共享一次有界 401 恢复与主体改变时停止重放的规则。 |
| 新版 UA 的应用字段变化 | 保留显式配置 UA 的独立 engine/app 版本；不把桌面 `26.*` 写入 engine `version`。既有默认稳定版自动同步继续由当前版本 owner 管理。 |
| 额度发现与 APNs 是独立协议 | 已有实现使用 `GET /celsius/ws/user` 返回的 `websocket_url`，再订阅 `codex-work-usage-push-v1`；异常保持 HTTP 回退。本项目启用策略不能被表述为官方 App 每个账号默认启用 push。 |

网关在请求路径等待现有锁保护的刷新；桌面 JS 在未过期的 5 分钟窗口内返回旧 token 并后台刷新。
这是服务器凭据管理的有意差异，避免增加脱离请求生命周期的刷新 goroutine；已过期 token 不作为成功结果返回。
静态解析 JWT 只用于缩短本地使用期限及拒绝身份矛盾，不代替上游验签或授予模型权限。

本次未将 APNs 注册/注销、`exec-server --concurrent-requests`、Statsig 曝光或完成事件元数据
加入模型请求。报告确认执行器并发参数的直接消费点是本地 argv，未证明它属于 Responses body/header。
5 秒通知注销信号、35 秒额度订阅信号和 Codex 300 秒 idle 分属不同阶段。

专用 Images 网关现支持默认的 App `/f/conversation` 主生图流程，以及管理端旧版回退开关。
Library `/wham/images/*` 仍是独立的单图最小 JSON 合同。App 模型、原生参数、按张计价、
浏览器验证和旧版升级边界见 [App 生图与旧版回退](../CHATGPT_APP_IMAGES.md)。

## 2026-11-06 增量对齐

本轮对照用户提供的四份静态报告，样本从 `26.1023.51421 / build 22530`、
`26.1027.30824 / build 22943`、`26.1027.51404 / build 23880` 更新至
`26.1103.31037 / build 25241`；最后一份的 macOS 内置 CLI 为 `0.167.0-alpha.3.2`。
报告对象的 bundle id 是 `com.openai.codex`。Electron surface、模型 Rust engine、
云端 app-server、验证页和遥测分别拥有请求合同，桌面版本不能直接替换所有 engine 版本。
六份输入文件的 SHA256 与报告补丁复原结果见
[来源清单](../../diagnostics/desktop-alignment-20261106/SOURCE_HASHES.json)，
仓库验证见 [本轮记录](../../diagnostics/desktop-alignment-20261106/README.md)。

| 报告证据 | 当前网关处理 |
| --- | --- |
| 五项保留头继续清理；主进程没有 `X-OAI-IS-Update` 消费或 `integrityStateStore` | HTTP、Compact、透传与 WS 保留入站白名单；完整性头和 `x-local-routing-key` 也从外层/turn metadata 中移除。Responses/Compact 的实际发送点不再加载旧 `ois1` 缓存，也不保存响应更新，避免请求构造后又恢复已删除的旧协议。独立设备证明仍由原有显式开关和真实 provider 提供。 |
| 环境配置 fetch 校验 account/user，再重建 workspace 路由 | WHAM discovery 在任何请求或缓存命中前核对当前 token 的已知账号和用户；发现请求携带 `Cache-Control: no-store` 并拒绝重定向。网关使用 10 秒发现预算和按 token 隔离的进程内路由缓存；发现接口拒绝访问（401/403）时使用默认官方 Codex 路由 60 秒，换 token 立即重新发现，模型鉴权仍由实际模型接口确认。其它发现错误及非法路由不降级。这里没有把 Electron 的 30 秒环境配置预算当成模型总时限。 |
| 401 至多刷新一次；主体变化撤销旧连接 | 保留一次受分布式锁保护的刷新。若刷新改变当前轮次已知的 workspace/user，返回账号级 `account_changed`，不在 HTTP、SSE、透传、WS 握手或 WS 错误事件路径中重发当前轮次；新凭据保留给后续独立请求。 |
| durable 连接必须跟随当前认证主体 | WS 握手兼容键加入 credential principal 摘要、最终 `ChatGPT-Account-Id` 和 FedRAMP 头。相同身份继续复用；相同本地账号/opaque token 但 workspace 或用户改变时替换旧空闲连接。既有 token、目标 URL、出口、安装/窗口和 continuation 隔离继续生效。 |
| 新增 `x-local-routing-key`、`x-ms-request-id`、`x-openai-legal-models` 字面量 | 分别是本地删除字段、Azure 响应诊断和 MCP schema 属性；不作为模型新增发送头。`X-OpenAI-Product-Sku: codex` 的新增位置是环境配置/云附件 ASB RPC，Responses 不额外注入。 |
| Statsig POST `https://ab.chatgpt.com/v1/initialize` 的 `credentials:omit` | 网关没有该遥测调用面，不新增请求或浏览器 cookie。该特例不改变 Responses 的 cookie 合同。 |
| `sampleThread`、`disableMcp`、`interruptOnCancel` 和本地活跃聊天恢复 | 属于桌面 app-server/预测/本地数据库，网关没有对应调用面，不写入模型 JSON。用户 `input`、工具参数和 opaque 加密历史不受 metadata 清理影响。 |

`prompt_cache_key` 的初始算法在这些二进制报告中仍未闭合；网关继续使用已回归的稳定会话/cache
身份规则，不根据这份报告每轮随机改键，也不把 `codexAppSessionId` 当作 thread/cache 身份。
完整 UA 的账号/全局优先级及引擎版本、桌面构建号分离契约继续见
[有意差异](intentional-divergences.md)。

报告第 8 节确认 WS 优先、HTTP 回退和默认 **300 秒流空闲时限**，但没有闭合所有回退条件。
当前网关的 WS 单次读预算默认 900 秒，首语义输出保护默认 90 秒、high effort 为 180 秒，
客户端轮间空闲预算为 300 秒；它们分别控制不同阶段。此轮不把这些值统一改成 300 秒，
也不对已接受或已输出的请求增加无条件 HTTP 重放。现有预热、续接、父 context 取消和安全回退测试
用于验证本地合同；没有生产抓包或真实上游延迟/风控验收。

## 证据范围

2026-10-14 对照 `ChatGPT_Codex_桌面端_深度分析报告.md`，分析对象为桌面版本
`26.1007.71326`、build `20398`。该报告是静态取证，头部清单包含 agent、浏览器、
消息、验证、远程控制和遥测等不同调用面，不能当作每次 Responses 请求都必须发送的头集合。
既有 Codex 源码基线及主体隔离约定见 [有意差异](intentional-divergences.md)。

完整请求头、逐端点模板、真实值来源和仍未补齐的字段见
[深度报告请求还原补充](ChatGPT_Codex_桌面端_深度分析报告_请求还原补充.md)
及 [机器清单](desktop-request-inventory.json)。上一轮使用固定 fixture/local mock，未把真实证明值
请求到 OpenAI；清理入站字段不等于已有可信真实值重建，也不等于完整 desktop 请求已还原。
补查确认的 surface 默认 originator 为 `Codex Desktop`，UA 工厂使用 appVersion/平台/架构；
DeviceCheck 实际头是 `x-sentinel-dc`；Expected-Account-Id/Attach-* 是本地消费字段。

## 请求边界与改进

| 报告机制 | 网关实现与验证边界 |
| --- | --- |
| `expectedPrincipal` 校验账号一致性 | `openai_chatgpt_headers.go` 校验可解码 OAuth token 的账号 claim 与凭据所属账号；认证构造与 HTTP/WS 账号头共用检查。关闭 workspace discovery 或配置可信 relay 也不能绕过。Spark shadow 使用母账号。矛盾值在出站前返回账号级 `account_changed`，不记录 token 或账号标识。 |
| JSON 请求、Responses SSE | 普通 HTTP 与透传统一生成单值 `Content-Type: application/json`；普通 Responses 使用 `Accept: text/event-stream`，即使下游要求非流式；Compact 使用 `Accept: application/json`。删除所有大小写变体后重建，避免重复值和透传偏好改变上游协议。 |
| 账号、安装、会话、轮次身份 | 复用现有 `CodexVirtualClientKey`、安装 ID 和请求内投影，不额外随机生成 device/session。UA、originator、version 按现有账号/全局配置成组解析。HTTP/WS 与 body/header 投影保持同一 attempt。 |
| 报告列出的 CLI engine 身份 | 精确补充 `codex_rmcp_cli`、`codex_http_cli`、`codex_app_server_cli`；显式配置这些 UA 时保留对应 originator、引擎版本与独立应用版本。保持默认身份及严格前缀/控制字符校验；这些名字只是客户端标识，不提供授权或 entitlement。浏览器 UA 不用于构造模型接口的 engine 画像。 |
| 证明与完整性状态归属客户端 | 从外层 `client_metadata` 及 turn metadata 清除 attestation、设备 ID、远程控制 session token、认证 cookie 与账号 routing/expected 头的注入。2026-11-06 起，Responses/Compact 的 HTTP、透传和 WS handshake 不再回带/更新旧 `X-OAI-IS` 状态；低层 CAS helper 只保留给其他既有调用面。普通与 full projection 共用清理，保留 `input`/工具参数中同名用户数据。 |
| 删除调用方 routing override | 继续使用既有安全白名单与 `applyOpenAIWorkspaceRoutingHeader`，仅服务端 discovery 结果可重建 routing override。管理员配置的可信 relay 仍是有意支持的出站目标。 |
| 授权与刷新 | 保留 PKCE、`auth.openai.com` 端点、既有客户端 ID、刷新锁和一次有界 `token_expired` 恢复；本轮不替换登录协议或刷新 token。 |

JWT 解码只是检测已知矛盾，不能代替服务端验签或认证。Opaque token、无账号 claim、API key、
PAT 和 Agent Identity 保持既有上游验证路径，不能从元数据获得额外能力。

## 需要真实协议证据的差异

Apple DeviceCheck、`x-oai-attestation`、`X-OAI-IS` 状态机与远程控制设备密钥均依赖真实客户端
或服务端签发状态。本项目不生成伪造值，也不把其他 principal 的值移植为当前账号证明。
报告限定 DeviceCheck 的消息/验证端点，不能据此要求所有 Codex Responses 都携带它。
CES、Statsig、Sentry 是产品遥测，静态报告没有证明它们是模型接口的必需请求。

官方 [WebSocket Mode](https://developers.openai.com/api/docs/guides/websocket-mode)
明确公开 Responses WebSocket 不使用 `stream`、`background`；当前私有 ChatGPT Codex WS 路径
仍按固定源码兼容约定保留 `stream:true`。没有私有端点的实际 capture 时，本轮不把公开 API
规则直接覆盖到该路径；连接复用、prewarm 和 continuation 的既有测试继续验证。

官方 [Codex app-server 集成](https://developers.openai.com/siwc/token-sharing-open-source/codex-app-server)
说明 clientInfo 的 name/version 参与 originator/User-Agent，且成功完成 inference 才能验证模型访问。
本轮的 fixture/本地 forwarding 测试只能证明请求构造与网关行为，不能证明真实账号的风控结果、
硬件证明有效性、模型 entitlement 或与官方桌面端的 TLS/HTTP2 指纹完全相同。

## 验证入口

在 `backend/` 执行：

```sh
go test -tags=unit ./internal/application/service -run '^TestOpenAIOAuthDesktopAlignment' -count=1 -v
go test -tags=unit ./internal/application/service/... ./internal/transport/http/... ./internal/shared/openai/... -count=1
```

事务记录包含同一 fixture 的 baseline、modified、rollback 输出与退出状态，原始文件 hash，
可重建补丁及仅对副本执行的回滚。结果必须区分构造测试、实际本地 forward 与真实上游验证。

## 当前可执行链与验证结果

普通 Codex attestation 已接入 HTTP/WS 实际发送点；其回调和原有 Live provider 分开处理。Linux 返回真实 unsupported-platform CBOR，macOS native unavailable 返回对应 error_code。s=0 回调成功并不代表 Apple 证明成功。

早期 X-OAI-IS 实现使用 immutable send receipt、principal/installation/origin namespace、共享 Redis 原子 CAS，24 小时缓存保留和本地 4096 项上限属于该旧兼容实现。2026-11-06 增量对齐已将 Responses/Compact 从该更新状态机移除；这段历史验证不能作为当前模型请求继续发送该头的依据。

DeviceCheck 已连接到准确的 Desktop messages/challenge 路径，并在 messages 发送前注册 /devicecheck、验证与复用 _devicecheck cookie。真实 native 测试确认本机设备不支持 raw Apple token；local mock 流程和真实 Redis 竞态通过，真实 Apple 注册尚未验证。

用户授权账号的真实非流式、SSE 和 WS 模型请求完成；wham/usage HTTP 200。成功模型请求未返回 integrity update，因此没有把本地 CAS 测试称为真实 X-OAI-IS 闭环。最新状态以深度补充报告附录 E 和机器清单为准。


Ghidra 后续新增的 controller TPM/SE 证明、CBOR/Windows 平台表修正和 native-load 失败语义见 [原生层对照](native-device-key-alignment.md)。硬件证明仅进入远程控制 enrollment/WS challenge，不附加到普通模型请求；成功硬件签名及真实上游 enrollment 仍需实际环境。

2026-10-15 CLI 反编译更新：所选本地 attestation callback 现在使用 100ms 预算，提前取消不会调用 provider，无 provider 时省略头；opaque token 与 Live native v1 校验分离。超时取消子 context，迟到结果不跨请求复用，未完成 callback 有 64 个全进程上限。线程选宿主与空回调参数的公开源码证据、二进制版本边界见 [原生证明对照](native-device-key-alignment.md)。

2026-10-16 账号绑定更新：请求发送前由服务端根据 credential owner、账号 installation identity、origin 和已投影 thread identity 创建 immutable scope。HTTP/Compact、透传/托管 WS、预热、DeviceCheck cookie/raw token 与 Live 生成都使用该 scope；入站伪造的 installation/thread header 不会选择 provider。不同逻辑账号得到不同 provider session 和 v3 integrity namespace，但这些 session 仍来自同一部署设备，不能被解释为每个账号都有独立物理 Apple 设备。
