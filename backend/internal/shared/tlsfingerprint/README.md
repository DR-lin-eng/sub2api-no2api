# tlsfingerprint

TLS 指纹 profile 与自定义拨号器。文件索引：`dialer.go`、`profile.go`；`dialer_*_test.go` 覆盖捕获和集成场景。

Profile 由账号服务在请求开始时解析一次。OpenAI/Codex 未显式绑定时使用每个账号独立的稳定伪随机
wire 变体；同一账号在 HTTP、SSE 和 WebSocket 重连中保持同一变体，临近账号不会共享同一组
ClientHello 套件子集和排序。显式绑定的 profile 作为基础参数，OpenAI OAuth 仍派生账号独立变体。
生产环境使用持久化 JWT secret 对账号变体种子执行 HMAC 派生，种子不会进入 ClientHello。连接池同时把本地账号 ID、出口
路由和 `FingerprintKey` 纳入隔离键；非本地 OpenAI principal 的 upstream pool 可跨本地 failover 记录复用。
OpenAI/Codex 的内置基础 Profile 取 Rustls aws-lc-rs 的
默认 cipher suites、key-exchange groups 和 `h2`/`http/1.1` ALPN，不能宣称固定 JA3，因为
官方 Rustls 会随机化 ClientHello 扩展顺序。

`CaptureWireProfile` / `WireProfileFingerprint` 提供确定性的 profile-input golden fixture，覆盖
cipher suites、曲线、ALPN、supported versions、key share、PSK modes 和扩展列表；它是回归捕获摘要，
不等同于真实 ClientHello 字节或 JA3。
