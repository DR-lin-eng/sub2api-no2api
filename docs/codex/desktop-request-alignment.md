# OpenAI OAuth 与 Codex 桌面请求对照

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
| 证明与完整性状态归属客户端 | 从外层 `client_metadata` 及 turn metadata 清除 attestation、设备 ID、远程控制 session token、认证 cookie 与账号 routing/expected 头的注入；HTTP、透传和直接 WS handshake 只回带当前 OAuth 账号保存的合法 `X-OAI-IS`，响应 update 通过 expected-state CAS 写回。普通与 full projection 共用清理，保留 `input`/工具参数中同名用户数据。 |
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

X-OAI-IS 已改为 immutable send receipt、principal/installation/origin namespace、共享 Redis 原子 CAS；托管 WS/prewarm 和直接 WS 都观察握手响应，避免重新解释旧握手或跨账号值。24 小时缓存保留和本地 4096 项上限是服务运行约束。

DeviceCheck 已连接到准确的 Desktop messages/challenge 路径，并在 messages 发送前注册 /devicecheck、验证与复用 _devicecheck cookie。真实 native 测试确认本机设备不支持 raw Apple token；local mock 流程和真实 Redis 竞态通过，真实 Apple 注册尚未验证。

用户授权账号的真实非流式、SSE 和 WS 模型请求完成；wham/usage HTTP 200。成功模型请求未返回 integrity update，因此没有把本地 CAS 测试称为真实 X-OAI-IS 闭环。最新状态以深度补充报告附录 E 和机器清单为准。


Ghidra 后续新增的 controller TPM/SE 证明、CBOR/Windows 平台表修正和 native-load 失败语义见 [原生层对照](native-device-key-alignment.md)。硬件证明仅进入远程控制 enrollment/WS challenge，不附加到普通模型请求；成功硬件签名及真实上游 enrollment 仍需实际环境。

2026-10-15 CLI 反编译更新：所选本地 attestation callback 现在使用 100ms 预算，提前取消不会调用 provider，无 provider 时省略头；opaque token 与 Live native v1 校验分离。超时取消子 context，迟到结果不跨请求复用，未完成 callback 有 64 个全进程上限。线程选宿主与空回调参数的公开源码证据、二进制版本边界见 [原生证明对照](native-device-key-alignment.md)。
