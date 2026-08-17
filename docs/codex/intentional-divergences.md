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
`session_id == thread_id == prompt_cache_key`、`x-client-request-id == thread_id`。下游提供的 Codex
保留身份头先被删除，再从同一 attempt plan 重建。

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
但不能把已成功请求改写为客户端错误。并发 Compact 在现有 string-state 接口下不提供跨实例原子计数。

## 平台与传输层

Profile 从 `identity_secret + principal` 确定性派生，但只在部署宿主的平台族内选择源码真实的终端组合。
这样未来引入原生传输 sidecar 时不会出现 UA 声称 macOS、传输层却固定呈现 Linux 的长期矛盾。

当前 A/B 是纯 Go 请求语义实现，明确不模拟以下内容：

- TLS ClientHello、HTTP/2 SETTINGS、Header 顺序和连接层时序；
- Codex Rust 网络栈的字节级传输特征；
- attestation、residency 或本项目无法真实证明的客户端能力。

这些属于暂缓的 phase C。启用 A/B 不应被描述为完成了传输层等价。

## 开关与轮换

推荐在管理员面板“网关服务 -> Codex OAuth A/B 模拟”中配置。面板保存到数据库设置
`codex_simulation_settings`，当前节点立即发布，其他节点通过后台任务最多在 5 秒内刷新；OAuth 请求路径
只读取内存快照，不查询数据库。数据库记录存在时会覆盖旧
YAML。点击“强制恢复原版行为”会调用无请求体的 `POST .../codex-simulation/restore-original`，即使旧记录
损坏或页面 TTL 输入无效，也会显式保存 `full_simulation_enabled=false` 与 `continuation_mode=off`，因此
不受旧文件中启用值影响。已有模拟 WS 会在下一轮关闭并通过重连进入原版路径。首次启用时后端自动生成
共享身份密钥，管理 API
只公开 `identity_secret_configured`，不会返回密钥内容。

下列 YAML 只保留为数据库尚无记录时的兼容默认值：

```yaml
gateway:
  codex_simulation:
    full_simulation_enabled: false
    identity_secret: ""
    continuation_mode: off # off|shadow|enforce
    state_ttl_seconds: 604800
```

A 和 B 默认关闭。A 还要求账号自身 `codex_fingerprint_mode=full`；B 与该账号开关独立。手工使用 YAML
启用 A 或 B 时，`identity_secret` 至少 32 字节且所有实例必须一致。面板生成的数据库密钥由共享
PostgreSQL 设置提供给所有实例。轮换 secret 会有意改变全部 root/principal 派生值，并使现有 owner、
response、generation 状态无法命中；应把它视为一次全量会话重置。

B shadow 与 A generation 是两个独立边界：shadow 不写 B owner，但若 A 同时启用，成功 Compact 仍会
按 A 的规则推进 generation。
