# Codex Remote Control Client

本模块实现桌面 controller 客户端的设备密钥注册、刷新与 WebSocket challenge。旧版 `shared/remotecontrol` 的 server enrollment/pairing 路径保持独立。

- `client.go`：`/codex/remote/control/client/enroll/{start,finish}`、`refresh/{start,finish}`；显式 step-up 授权；账号、设备身份摘要、origin/path、nonce、有效期、session token 摘要与 scopes 校验；WebSocket 首次 challenge 签名。
- `store.go`：按 API base、account user、installation 隔离的公开密钥引用持久化。文件模式 0600、原子替换；不保存 session token、step-up token 或私钥。
- `DeviceKeys` 为平台端口，生产适配器是 `platform/devicekey`。`shared/remotecontrol/device_key.go` 维护无状态 wire 结构、域分隔载荷和本地 P-256/SHA-256/DER 验签。

应用入口为 `service.NewCodexRemoteControlClient`：先解析凭据 owner（包含 Spark shadow），绑定账号出口 HTTP client、安装身份和 native resources，再构造 manager。只有显式 `ConnectSession` 才会请求注册；没有 step-up callback 时不发出 enrollment 请求。`Dial` 在设备 challenge 签名完成后返回连接，调用方负责后续读循环与连接生命周期。

Native provider 调用官方 `remote-control-device-key.node`，由其管理 Linux TPM、macOS Secure Enclave 或显式允许的 OS nonextractable fallback。不生成可导出软件私钥。签名证明公钥的持有；硬件来源还依赖受信任 provider 和注册关系，不能仅凭一次 ECDSA 验签断言 TPM/SE 来源。

本地协议测试使用明确标记的软件 fixture，验证完整请求链与拒绝条件。真实 native 测试需要显式设置 `SUB2API_TEST_DEVICEKEY_RESOURCES`，创建自己的测试 key、签名验签后删除；普通测试不会访问硬件或上游账号。没有真实 step-up 授权时，不执行上游远程控制注册。
