# tlsfingerprint

TLS 指纹 profile 与自定义拨号器。文件索引：`dialer.go`、`profile.go`；`dialer_*_test.go` 覆盖捕获和集成场景。

Profile 由账号服务在请求开始时解析一次。未显式绑定时使用虚拟客户端 key 的 rendezvous hashing，
因此同一 principal 在 HTTP、SSE 和 WebSocket 重连中保持同一 Profile；连接池同时把本地账号 ID、出口
路由和 `FingerprintKey` 纳入隔离键；非本地 OpenAI principal 的 upstream pool 可跨本地 failover 记录复用。
OpenAI/Codex 的内置 Profile 取 Rustls aws-lc-rs 的
默认 cipher suites、key-exchange groups 和 `h2`/`http/1.1` ALPN，不能宣称固定 JA3，因为
官方 Rustls 会随机化 ClientHello 扩展顺序。

`CaptureWireProfile` / `WireProfileFingerprint` 提供确定性的 profile-input golden fixture，覆盖
cipher suites、曲线、ALPN、supported versions、key share、PSK modes 和扩展列表；它是回归捕获摘要，
不等同于真实 ClientHello 字节或 JA3。
