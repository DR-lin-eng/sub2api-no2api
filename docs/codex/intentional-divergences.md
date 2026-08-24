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
采用 `thread_id:window_number` 形状。下游提供的 Codex 保留身份头先被删除，再从同一 attempt plan
重建。full simulation 的 direct `x-codex-installation-id` 只在 Compact projection 保留，普通
Responses/WS 通过 `client_metadata` 投影；`client_metadata` 顶层只保留源码兼容投影；调用方自定义键会转入
`x-codex-turn-metadata` 的有界扁平 extra 字段，并按源码规则限制键和值长度。
parent/fork/turn/root 关联 ID 会按虚拟 principal 重新派生，合法的 subagent 分类和对应兼容头会保留。

## 默认 OAuth 出站身份

即使 full simulation 关闭，普通 Codex OAuth 请求也不能直接复用下游会话标识。HTTP 与 WebSocket
从 `session-id` / `session_id`、`thread-id`、`client_metadata` 和 `prompt_cache_key` 中选择稳定信号，
再按 API Key 与所选上游账号命名空间派生 `session-id`、`thread-id`，并固定
`x-client-request-id == thread-id`。普通 HTTP body 可安全重建时，`client_metadata` 的 session/thread
投影同步改写；账号指纹计划仍是最终覆盖者。旧的 `session_id` / `conversation_id` 只作为网关内部
兼容投影保留，不得把下游原值直接带到另一个上游账号。

出站身份策略开启时，命中已知容量降载桶的 `codex-tui` 会在最终身份解析边界改写为
`codex_cli_rs`；版本、OS、架构和终端指纹保留，User-Agent 首段与 `originator` 始终配对。
将 `gateway.disable_codex_identity_enforcement` 设为 `true` 后，该归一化也随之关闭，保留完整回滚语义。

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
- incremental body 不能跨主体；
- 同主体 WS incremental 必须复用原始上游 WS 连接，连接繁忙由现有池等待；
- 未知 owner 可以先尝试；若上游返回 `invalid_encrypted_content`，写入 external tombstone；
- enforce 下的主体/连接不匹配是请求终态，不包装为 `UpstreamFailoverError`；
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

Profile 从 `identity_secret + principal` 确定性派生，但只在部署宿主的平台族内选择源码真实的终端组合。
OpenAI OAuth 的稳定数据库 profile 分配也优先使用 `chatgpt_account_id`，避免同一虚拟 principal
在本地账号 failover 后切换 TLS 外观；Spark shadow 只保存不含凭据的
`codex_virtual_client_key` 来继承该 namespace。这样未来引入原生传输 sidecar 时不会出现 UA 声称
macOS、传输层却固定呈现 Linux 的长期矛盾。

当多个 OAuth 记录共享同一非本地虚拟 principal、出口路由和 TLS profile 时，账号级 upstream pool 也使用
该 principal 的不可逆短 key；缺少 upstream principal 的 `local:` 账号仍保持本地账号隔离。

当前 A/B 是纯 Go 请求语义实现，明确不模拟以下内容；这与独立的、按账号启用的 TLS
Profile 传输层开关是两个边界。OpenAI/Codex 的 models、usage、quota 辅助请求现在也复用账号级
HTTP/TLS upstream；未接入该端口的测试桩仍保留旧客户端 fallback：

ChatGPT HTTPS 请求还共享只允许 Cloudflare 基础设施 cookie 的进程级 jar；jar 仍按 host/domain/path
执行 CookieJar 匹配，账号、session、auth cookie 会被拒绝，不会因为多个虚拟客户端共用 jar 而互相泄露。

Remote Control 的 URL、enroll/refresh/pair 请求、protocol-v3 WebSocket header 和 envelope 合同已收敛到
`internal/shared/remotecontrol`，LifecycleManager 已覆盖 enrollment refresh、pairing、状态、清理和 WS dial，
账号适配器会把 token 以密文写入 Account.Extra；调用方可以把它绑定到账号级 HTTP/TLS transport。网关请求路径
本身仍不自动启动后台 Remote Control socket，真实 enrollment/heartbeat 需要外部控制器调用该 manager。

full simulation 会清理下游直接注入的 `x-oai-attestation`、residency 和 host-device-kind 头，避免把调用方
的运行时证明带到另一个 OAuth principal；真实平台证明仍只由现有 Live/Agent Identity 专用路径提供。

- A/B 本身不改写 TLS ClientHello、HTTP/2 SETTINGS、Header 顺序和连接层时序；
- Codex Rust 网络栈的字节级传输特征；
- attestation、residency 或本项目无法真实证明的客户端能力。

这些属于 A/B 的暂缓 phase C。启用 A/B 不应被描述为完成了传输层等价；启用账号 TLS
Profile 后也只能保证 Rustls provider 参数和连接隔离，不能保证跨平台 byte-for-byte JA3。

`CaptureWireProfile` / `WireProfileFingerprint` 只提供确定性的 ClientHello-input golden 摘要；真实 socket
字节 capture、HTTP/2 SETTINGS/HPACK 和连接时序仍需独立的 capture harness。

## 开关与轮换

推荐在管理员面板“网关服务 -> Codex OAuth A/B/C 模拟”中配置。面板保存到数据库设置
`codex_simulation_settings`，当前节点立即发布，其他节点通过后台任务最多在 5 秒内刷新；OAuth 请求路径
只读取内存快照，不查询数据库。数据库记录存在时会覆盖旧
YAML。点击“强制恢复原版行为”会调用无请求体的 `POST .../codex-simulation/restore-original`，即使旧记录
损坏或页面 TTL 输入无效，也会显式保存 `full_simulation_enabled=false`、`c_level_simulation_enabled=false` 与 `continuation_mode=off`，因此
不受旧文件中启用值影响。已有模拟 WS 会在下一轮关闭并通过重连进入原版路径。首次启用时后端自动生成
共享身份密钥，管理 API
只公开 `identity_secret_configured`，不会返回密钥内容。

下列 YAML 只保留为数据库尚无记录时的兼容默认值：

```yaml
gateway:
  codex_simulation:
    full_simulation_enabled: false
    c_level_simulation_enabled: false
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
