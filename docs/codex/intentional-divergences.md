# Codex OAuth 模拟的有意差异

本文记录 Sub2API 纯 Go A/B 实现相对固定 `openai/codex` 源码快照的有意差异，避免后续维护者把网关
所需的主体隔离当成兼容 bug，或把尚未实现的传输层外观误认为已覆盖。

## 对照基线

- 上游源码：`openai/codex`
- 固定 revision：`9ded177ce7c1c0bd2047f902936c177612ab3434`
- `codex-api/src/requests/headers.rs` 只生成 `session-id` 与 `thread-id`，不生成 `session_id`。
- `codex-api/src/endpoint/responses.rs` 将 `x-client-request-id` 设为 `thread_id`。
- `core/src/client.rs` 在调用方未提供时将 `prompt_cache_key` 设为 `session_id`。

本项目因此在 full simulation 出站时只生成横线形式的 session/thread 头，并使
`session_id == thread_id == prompt_cache_key`、`x-client-request-id == thread_id`。window projection
采用 `thread_id:window_number` 形状，编号从 1 开始。下游提供的 Codex 保留身份头先被删除，再从同一
attempt plan 重建。full simulation 的 direct `x-codex-installation-id` 只在 Compact projection 保留，普通
Responses/WS 通过 `client_metadata` 投影；`client_metadata` 顶层只保留源码兼容投影；调用方自定义键会转入
`x-codex-turn-metadata` 的有界扁平 extra 字段，并按源码规则限制键和值长度。
每个 OpenAI OAuth 账号在首次创建或首次 full simulation 请求时生成一个随机 `context_window_id`，写入
`accounts.extra.codex_context_window_id`，后续请求固定复用；它不会采用下游提供的窗口 ID。root_turn_id
始终与当前 turn_id 对齐，parent/fork/turn 关联 ID 按虚拟 principal 重新派生，合法的 subagent 分类和对应兼容头会保留。

full simulation 的 session、thread、turn 以及上下文窗口 ID 使用 UUIDv7 的毫秒时间戳布局；installation ID
继续使用 UUIDv4，以保持安装身份与会话时间身份的边界。

## 默认 OAuth 出站身份

即使 full simulation 关闭，普通 Codex OAuth 请求也不能直接复用下游会话标识。HTTP 与 WebSocket
从 `session-id` / `session_id`、`thread-id`、`client_metadata` 和 `prompt_cache_key` 中选择稳定信号，
再按账号的 `CodexVirtualClientKey` 命名空间派生 `session-id`、`thread-id`，并固定
`x-client-request-id == thread-id`。没有任何客户端会话信号时，session/thread 回退到账号稳定派生值，
不再每次请求生成新的随机主体。账号没有持久化 `openai_device_id` 时，installation/device 标识也按同一
账号 key 稳定派生；客户端传入的 installation、session、thread、turn、window、approval 值不具备权威性。
普通 HTTP body 可安全重建时，`client_metadata` 的 session/thread/installation 投影同步改写；账号指纹计划
仍是最终覆盖者。旧的 `session_id` / `conversation_id` 只作为网关内部兼容投影保留，不得把下游原值直接
带到另一个上游账号。

出站 UA 按账号 `credentials.user_agent`、全局 `openai_codex_user_agent`、默认 CLI 身份的顺序解析；
`ForceCodexCLI` 开启时使用全局/默认身份。显式配置的官方客户端 UA 保留名称、引擎版本、OS、架构、
终端和末尾的应用构建号，`originator` 从 UA 首段配对，`version` 取首个 `/` 后的引擎版本。
例如 `Codex Desktop/0.153.4 (Windows 10.0.26200; x86_64) unknown (Codex Desktop; 26.903.71938)`
中的 `0.153.4` 和 `26.903.71938` 分属引擎与应用，不能互相覆盖；`codex-tui` 等显式身份也不会自动
归一为 `codex_cli_rs`。固定/自动同步版本只负责生成未配置完整 UA 时的默认 CLI 身份。
无效或非官方 UA 继续回退为规范身份。将 `gateway.disable_codex_identity_enforcement` 设为 `true`
后使用请求 UA 配对身份，`version` 仍与 UA 的引擎版本保持一致。

HTTP 普通转发、透传、Compact 与 WS 默认使用同一身份解析结果。full simulation 的 Linux 画像仅在
C 与实验性传输开关同时开启且账号未配置 UA 时启用。WS 连接兼容键包含 `User-Agent`、`originator`、`version` 和账号 TLS Profile；
身份设置改变后的新请求不会复用旧握手，身份不变时保留原连接亲和性。
旧的 session/full 指纹模式也从同一请求计划设置 HTTP 与 WS 握手的 `x-client-request-id`，
不再由 WS 继承入站原值；后续 WS 帧仍按既有协议在 body metadata 中投影每轮身份。

上游 WebSocket 可能以 `type:error` 或 `response.failed` 返回 `server_is_overloaded`、`slow_down`
或仅包含过载消息。网关只在首个语义输出前把它转换为携带原始事件体和握手响应头的 503
`UpstreamFailoverError`；语义输出已经提交后绝不重放。OAuth ingress 和 passthrough 对
`response.created` / `response.in_progress` 使用有界前导缓存，避免非语义元数据过早破坏换号安全性。
WS 握手返回 401/403 且尚未产生语义输出时，会先静默切换到 HTTP Responses/HTTP bridge；
拨号器的 `expected handshake response ... 401` 只进入运维日志。只有 HTTP 也失败时才进入正常账号
failover，service 层不会先写 JSON，因此不会再和外层 `response.failed` 终止事件拼接。

透传路径的恢复重试使用请求级总 attempt budget，不会在每次切换账号后重新获得完整的同账号重试次数。
带显式 `store:true`、图片生成意图、`previous_response_id` 或工具输出的请求不做无法证明幂等的传输重放。
首语义输出前的 SSE keepalive 可以先提交 200；此时账号响应元数据通过预声明的 HTTP trailer 发送，
`x-codex-turn-state` 同时写入按账号隔离的本地/共享会话状态，后续 OAuth 请求可自动回带。流设置缓存
采用 stale-while-revalidate，设置库不可用不会阻塞转发；传输超时的账号 runtime block 延迟到重试预算
耗尽，恢复成功会只清理同一 `transport_timeout` 原因的封禁。

管理员还可以在数据库运行时设置中显式开启 Turn State 重放。该模式优先于客户端回带和会话缓存：每个
OpenAI OAuth HTTP 请求从当前账号可用池随机取一个值并覆盖注入；原生 WS 在每次新连接握手时随机取值，
同一连接的后续帧沿用原握手头；托管连接池把 state 摘要纳入握手兼容键，不会把选择了不同 state 的请求
复用到同一上游连接。手工值没有账号绑定；从质量
巡检一键同步的值只对采集账号可用。质量巡检只把通过阶段的响应头纳入同步候选，并可用独立开关随机注入
同一池做开启/关闭对照。原始 state 只出现在管理员接口和数据库设置，不进入公开质量页面或诊断日志。

自动监测模式与上述兼容随机池使用独立开关。管理员逐行配置关注的实际上游模型；系统只对这些模型观察
HTTP 响应头和 Responses SSE/WS 元数据 state 的 Unicode 字符数；`/v1/messages` 兼容桥也使用同一自动池。目标字符长度可在面板设置（默认 292，范围 1–8192）；业务监控只有严格等于目标长度且不同于上一值时才写入，
共享缓存；日志只记录长度、命中、新旧与代理布尔值。普通 OAuth 请求在最终模型归一化后使用对应的
`CodexBaseInstructionsForModel`，仅替换缺失值或通用默认值。首次缺少正确值时立即探测，已有正确值 45 分钟没有更新时
刷新探测；HTTP 账号用模型专用 instructions 发最短合法请求并在响应头到达后关闭正文，WSv2 账号使用零输出 ping。
事件唤醒配合 5 秒扫描，单实例并发 32 个账号/模型目标，每个目标在账号正常出口并行竞速最多 4 次且每次最多
15 秒；首个长度正确、头值合法且未明确过期的结果会取消其余尝试并恢复重放，即使该值与旧值相同。正常
业务监控随后拿到新的正确值会立即替换；整轮失败后 5 秒继续下一轮且不限制轮数，直到成功、关闭监测或账号
失去资格。代理竞速探测默认关闭；管理员开启后可选定专用代理 ID，未指定时使用健康代理池。探测不写用户用量日志，多实例以共享锁避免重复探测。
关注模型优先使用自动池且不接受未经验证的入站 state；未关注模型
不参与自动逻辑，并可继续使用旧随机池。

管理端另有独立页面和当前节点只读接口 `GET /api/v1/admin/state-diagnostics`。它按 URL-safe
Base64 解码 `X-Codex-Turn-State`，展示版本、字符数、原始字节数、签发时间和按签发时间加 1 小时推算的到期时间；
该“到期”不是协议明确字段，解析失败时不会把编码字符串长度冒充原始字节数。`encrypted_content` 只统计可解码的
原始字节，使用同账号/模型的最短样本作为基线，+16 B 只作为观察线索。接口不返回任何 opaque 正文，轮换错误
只保留计数和错误类别；诊断快照是单进程状态，部署多节点时每个节点分别观察。旧的
`/api/v1/admin/settings/codex-simulation/observability` 路径仅作为兼容别名保留。

## 网关必须存在的差异

官方客户端在一个本地 installation 内直接拥有会话；Sub2API 则让多个下游调用方共享上游账号池。
为了不暴露内部用户标识、也不让两个主体复用不可移植的 continuation，A 使用以下内部层级：

```text
canonical request body
  -> HMAC(api_key/group, ingress project, conversation signal) = request root
  -> HMAC(root, upstream principal) = session/thread/turn/window/cache identity
```

`chatgpt_account_id` 是首选 principal。缺失时回退到 `local:<account ID>` 的独立命名空间，绝不把空值
折叠成一个共享主体；反过来，两个本地账号记录指向同一 `chatgpt_account_id` 时有意视为同一主体。
同一请求、同一主体的 retry 复用 turn；切换主体会得到不同 turn。入口专用
`X-Sub2API-Codex-Project-ID` 只参与 root HMAC，绝不发送上游。

request root 从账号选择前的 canonical body 建立一次，canonical bytes 不被 attempt 逻辑修改。每个
attempt 从它单独派生；需要重建 JSON 时使用结构化解析与确定性 Go JSON 编码，不能依赖手写字符串替换。

## Continuation 所有权

官方 Codex 能直接消费自己进程保存的 encrypted/previous-response 状态；网关既不能也不会解密或伪造
`encrypted_content`。B 只保存 `root -> owner` 与 `response -> owner`，不建立 item-owner 数据库：

- full body 可以在跨主体前结构化清理，保留明文与可证明完整的 call/output 对；
- incremental body 不能跨主体；enforce 在选号前按已记录的 owner principal 过滤候选；现有
  `previous_response_id -> account_id` 或当前节点的原始连接仍可用时同时锁定本地账号并把它注入
  Scheduler V2 优先候选；跨节点缺少原连接时仍安全回退到 principal-only 过滤，避免先选中其他主体并
  产生可避免的 409；
- 同主体 WS incremental 必须复用原始上游 WS 连接，连接繁忙由现有池等待；
- 未知 owner 可以先尝试；若上游返回 `invalid_encrypted_content`，写入 external tombstone；
- enforce 下经过 owner-principal 约束后仍发生的主体/连接不匹配是请求终态，不包装为
  `UpstreamFailoverError`；
- shadow 只读状态、分类和记录假设，不写 owner、不拒绝请求。

账号配置为 WS `passthrough` 时，B enforce 会在该请求入口收敛到 `ctx_pool`。直接 relay 没有可跨请求
寻址和等待的池连接 ID，继续使用它将无法证明“原始连接”仍是同一 socket；A 与 B shadow 不需要该约束，
仍保留 passthrough 模式。

owner、response 和 generation 使用现有 Redis string-state 接口，TTL 默认 7 天；Redis 错误时回退到
有界进程内缓存。成功 turn 刷新 owner，只有成功 Compact 才写入下一 generation。Compact 成功后的
状态写入不是上游响应事务的一部分：进程在响应成功与状态确认之间崩溃时，允许 window metadata 漂移，
但不能把已成功请求改写为客户端错误。长连接还绑定 settings epoch；secret、continuation mode 或 TTL
变化后，下一次 incremental turn 要求重连，避免一条 WS 跨越两个虚拟客户端状态。并发 Compact 在现有
string-state 接口下不提供跨实例原子计数。

## 平台与传输层

身份 Profile 从 `identity_secret + principal + 已解析 UA` 确定性派生，UA 配置与普通请求共用解析器。
OpenAI OAuth 的稳定数据库 profile 分配也优先使用 `chatgpt_account_id`，避免同一虚拟 principal
在本地账号 failover 后切换 TLS 外观；Spark shadow 只保存不含凭据的
`codex_virtual_client_key` 来继承该 namespace。UA 中的平台字段和 TLS ClientHello 参数是不同层级；
保留 Desktop UA 不代表传输栈变为官方原生客户端。

在 Linux amd64 上，C 与实验性传输开关同时开启且账号未配置 UA 时，full simulation 会从插件提供的五个抓包画像（Fedora/Arch/Ubuntu/Debian，
`xterm-256color`、`alacritty`、`kitty`、`screen`）中按 principal 稳定选择一个；Codex 版本仍由网关
canonical version 同步决定。账号显式 UA 继续优先；任一开关关闭或宿主不是 Linux amd64 时，
沿用共享 UA 解析结果，保留全局或账号的完整客户端身份。

当多个 OAuth 记录共享同一非本地虚拟 principal、出口路由和 TLS profile 时，账号级 upstream pool 也使用
该 principal 的不可逆短 key；缺少 upstream principal 的 `local:` 账号仍保持本地账号隔离。

当前 A/B 是纯 Go 请求语义实现；实验性传输开关仅在 C-level 下按账号启用。默认路径明确不模拟以下内容；这与独立的、按账号启用的 TLS
Profile 传输层开关是两个边界。OpenAI/Codex 的 models、usage、quota 辅助请求现在也复用账号级
HTTP/TLS upstream；未接入该端口的测试桩仍保留旧客户端 fallback：

ChatGPT HTTPS 请求还共享只允许 Cloudflare 基础设施 cookie 的进程级 jar；jar 仍按 host/domain/path
执行 CookieJar 匹配，账号、session、auth cookie 会被拒绝，不会因为多个虚拟客户端共用 jar 而互相泄露。

Remote Control 的 URL、enroll/refresh/pair 请求、protocol-v3 WebSocket header 和 envelope 合同已收敛到
`internal/shared/remotecontrol`，LifecycleManager 已覆盖 enrollment refresh、pairing、状态、清理和 WS dial，
账号适配器会把 token 以密文写入 Account.Extra；调用方可以把它绑定到账号级 HTTP/TLS transport。网关请求路径
本身仍不自动启动后台 Remote Control socket，真实 enrollment/heartbeat 需要外部控制器调用该 manager。

## 最新 Codex 请求头收敛

OpenAI OAuth 的 Responses HTTP、Compact、透传和 Responses WebSocket 路径现在共享同一组出站头策略：

- `x-codex-parent-thread-id`、`x-openai-subagent` 从隔离后的 `client_metadata`/turn metadata 重建；
  调用方原始值不会直接跨 API key 或上游账号复用。
- Guardian 的 `x-codex-guardian: reviewer|classifier` 只有在对应 `x-openai-subagent` 与
  `thread_source`/`turn_trigger` 组合一致时才生成；普通请求会清除伪造值。
- Memory consolidation 只有在 `x-openai-memgen-request: true`、`memory_consolidation` subagent 和
  `request_kind=memory` 的语义同时成立时才生成。
- `parent_response_id`、`guardian_credits_requested`、`history_ingest_requested`、`analytics_enabled`、
  `forked_from_ordinal_exclusive`、`turn_trigger` 等新版 metadata 键保留在结构化投影中，并保持官方类型。
- Responses Lite 的 `x-openai-internal-codex-responses-lite` 会在 HTTP/兼容桥上继续透传；原生 WS
  握手同时从入站握手头或 `client_metadata.ws_request_header_x_openai_internal_codex_responses_lite` 重建，
  并纳入连接兼容键，避免 Lite 与普通握手复用同一 socket。
- Workspace routing 由网关使用当前 OAuth token 调用 `wham/accounts/check` 发现；只接受 HTTPS origin 与
  `us`、`us_cr`、`NO_CONSTRAINT`。`us/us_cr` 生成 `x-openai-account-routing-override`，
  `NO_CONSTRAINT` 不生成该头但仍禁用重定向。发现失败对当前账号 fail-closed 并进入账号级切换。
- WS 连接兼容键包含最终目标 URL、routing override、认证摘要、Guardian/memgen/subagent 和 timing 头，
  不会把不同 workspace、token 或语义会话复用到同一握手。

为避免把 OAuth Bearer token 发送到未经验证的管理员自定义 Relay，当前 discovery 仅在官方
`chatgpt.com` Codex origin 启用；自定义/全局 Relay 保留原有 Relay 路由，不自动发送
`/wham/accounts/check`。若 Relay 将来提供明确的受信 discovery 合同，再单独接入该路径。

这些字段由网关根据已验证的 OAuth 账号和请求语义生成；它们不属于 API key 账号的任意 Header override
或低风险 passthrough 白名单。`x-oai-attestation`、managed residency 和 host-device-kind 仍不伪造。

full simulation 会清理下游直接注入的 `x-oai-attestation`、residency 和 host-device-kind 头，避免把调用方
的运行时证明带到另一个 OAuth principal；真实平台证明仍只由现有 Live/Agent Identity 专用路径提供。

新版 Codex 的 `response.metadata` 事件可能携带
`metadata.openai_verification_recommendation: ["trusted_access_for_cyber"]`。网关只识别这个已知数组枚举，
在请求上下文中保留去重后的观测值，并原样转发事件；它不会把验证建议误判为 `cyber_policy`，也不会因此
触发账号切换、重试或模型改写。客户端可据此显示自己的结构化 `model/verification` 通知。HTTP header 中
同名字段、标量值和未知枚举会被忽略，遵循新版 Codex 的解析边界。

Codex turn metadata 中的 workspace 投影不再把本地绝对路径、remote URL 的 userinfo/query/fragment 或
workspace 内的 token/secret/password 字段带到另一个 OAuth principal。路径替换为固定的
`workspace:redacted`，remote 仅保留协议、主机和仓库路径；无效 JSON 仍交给原有协议校验处理。
OAuth 请求头中的 `x-codex-turn-metadata` 也不再绕过这条边界：有 fingerprint plan 时使用同一份
server-side identity projection；普通 OAuth 路径优先采用已经重写过的 `client_metadata`，对 header-only
metadata 删除 app-server 所有的 installation/turn/window/sandbox/approval 字段，限制额外键和值，并把
session/thread 重新绑定到当前 OAuth 账号。无效 header 会被丢弃；`thread_source`、`turn_trigger` 等字段
只保留为有界观测，不构成 Work、Cyber 或审批授权。
Responses body 顶层的 `access_programs` 同样视为 app-server/server-owned capability 选择：当前
Sub2API 没有对外暴露 `cyber_access_program` 的 RPC、模型 entitlement 或 ChatGPT-auth 专用投影链路，
因此 HTTP 与 WebSocket relay 都会删除客户端注入的顶层 `access_programs`。同一边界也删除
`serviceName`、`daybreakEnabled`、`cyberAccessProgram`、`disabledPluginIds`、`serviceTierForTurn`、
`turnTrigger`、approval/sandbox policy 等 app-server envelope 字段；它们不能靠客户端字段授予 Work、
Cyber、插件或沙箱能力。嵌套在 `input` 或工具参数中的同名用户数据不受影响；上游返回的模型目录
`available_access_programs` 仍可作为发现信息，但不能反向授予推理权限。

- A/B 本身不改写 TLS ClientHello、HTTP/2 SETTINGS、Header 顺序和连接层时序；
- Codex Rust 网络栈的字节级传输特征；
- attestation、residency 或本项目无法真实证明的客户端能力。

实验性传输开关打开后会尝试启用 uTLS 的 X25519MLKEM768、每连接扩展顺序随机化以及 req/v3 的
HTTP/2 SETTINGS/WINDOW 参数；它们仍然不能保证跨平台 byte-for-byte JA3，也不能替代真实抓包验证。
开关关闭时保留现有 Go transport。诊断文件只写入脱敏的账号、persona、routing-hint 是否出现和恢复状态，
路径为 `pricing.data_dir/plugin-diag/codex-persona.log`。

`CaptureWireProfile` / `WireProfileFingerprint` 只提供确定性的 ClientHello-input golden 摘要；真实 socket
字节 capture、HTTP/2 SETTINGS/HPACK 和连接时序仍需独立的 capture harness。

## 开关与轮换

推荐在管理员面板“网关服务 -> Codex OAuth A/B/C 模拟”中配置。面板保存到数据库设置
`codex_simulation_settings`，当前节点立即发布，其他节点通过后台任务最多在 5 秒内刷新；OAuth 请求路径
只读取内存快照，不查询数据库。数据库记录存在时会覆盖旧
YAML。点击“强制恢复原版行为”会调用无请求体的 `POST .../codex-simulation/restore-original`，即使旧记录
损坏或页面 TTL 输入无效，也会显式保存 `full_simulation_enabled=false`、`c_level_simulation_enabled=false`、`turn_state_replay_enabled=false`、`turn_state_auto_replay_enabled=false` 与 `continuation_mode=off`，因此
不受旧文件中启用值影响。已有模拟 WS 会在下一轮关闭并通过重连进入原版路径。首次启用时后端自动生成
共享身份密钥，管理 API
只公开 `identity_secret_configured`，不会返回密钥内容。
系统级 `codex_prewarm_continuation_force_enabled` 开启后会覆盖账号级关闭值，并让后续创建或导入的
OpenAI OAuth 账号自动保存 `codex_prewarm_continuation_enabled=true`。
同一面板允许逐行维护最多 100 个 Turn State，并通过
`POST .../codex-simulation/sync-turn-states` 同步最近一次质量巡检的健康结果；设置保存和跨节点刷新继续复用
`codex_simulation_settings` 的当前节点立即发布、其他节点最多 5 秒刷新语义。

下列 YAML 只保留为数据库尚无记录时的兼容默认值：

```yaml
gateway:
  codex_simulation:
    full_simulation_enabled: false
    c_level_simulation_enabled: false
    experimental_transport_enabled: false
    identity_secret: ""
    continuation_mode: off # off|shadow|enforce
    state_ttl_seconds: 604800
```

A、B 和 C 默认关闭。A 还要求账号自身 `codex_fingerprint_mode=full`；B 与 C 与该账号开关独立。手工使用 YAML
启用 A 或 B 时，`identity_secret` 至少 32 字节且所有实例必须一致。面板生成的数据库密钥由共享
PostgreSQL 设置提供给所有实例。轮换 secret 会有意改变全部 root/principal 派生值，并使现有 owner、
response、generation 状态无法命中；应把它视为一次全量会话重置。

B shadow 与 A generation 是两个独立边界：shadow 不写 B owner，但若 A 同时启用，成功 Compact 仍会
按 A 的规则推进 generation。
