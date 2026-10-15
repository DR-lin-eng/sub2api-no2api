# Ghidra 原生证明报告与代码对照

本轮以用户提供的 `Ghidra反编译专项报告.md` 为线索，重新核对同版本 `26.1007.71326` 的 main/transport bundle 与 Linux/macOS native 模块。此前的临时反编译输出已丢失；2026-10-15 更新报告后，已重新读取保存的 CLI v1/v2 产物和 macOS CreatePrivateKey 输出，证据清单分别记录现存文件、历史摘要与公开源码来源。报告、二进制摘要和 JS 字节位置见 [证据清单](native-device-key-evidence.json)。

## 已补齐的代码

| 发现 | 实现与边界 |
| --- | --- |
| TPM/SE 设备密钥 | `platform/devicekey` 调用官方 native API，提供 create/public/sign/delete；不重新实现 TPM keystore，也不导出或伪造硬件私钥。 |
| `ecdsa_p256_sha256` | SPKI DER P-256 公钥校验；对 payload 的 SHA-256 做 ECDSA ASN.1 DER 验签，使用标准 base64。 |
| 设备身份摘要 | 严格按 `algorithm,keyId,protectionClass,publicKeySpkiDerBase64` 顺序序列化 JSON，再 SHA-256/base64url。 |
| 注册与刷新 | 独立 controller client 模块完成 start/finish、显式新鲜 step-up、账号身份校验、设备公钥绑定与 refresh 必需的 `device_identity_hash`。 |
| WS `requiresDeviceKeyProof` | `x-codex-client-session-token`、client ID、v3、cursor；首个 challenge 校验 token 摘要、有效期、scopes、账号、origin/path，发送 `device_key_proof`。 |
| 签名载荷 | 固定 domain `codex-device-key-sign-payload/v1`，保持 desktop `CKe/wKe` 的字段顺序与 camelCase；HTTP proof 使用 snake_case，WS proof 使用 camelCase。 |
| 持久化 | 只保存按账号/安装/API 隔离的公开 enrollment 引用；session/step-up token 不落盘。已撤销 enrollment 只在服务端明确状态下清理并重新要求授权。 |
| CLI 回调生命周期 | `CodexHeader` 对所选本地 provider 使用 100ms 总预算；提前取消不调用 provider，等待超时取消子 context，返回 s=1；取消/失败/畸形响应分别为 s=3/2/4。没有 provider 时省略头。 |
| opaque callback token | 外层 `{v:1,s:0,t:<string>}` 保留宿主返回的字符串，包括空字符串；这只证明回调返回，不证明 Apple 证明成功。Live 使用独立的 native v1 校验。 |
| 原生模块加载失败 | Codex callback 返回真实 `error_code=4` 与实际 signals；旧 Live/raw DeviceCheck 继续拒绝不可用证明。Intel Mac 的 error 2 也保留实际 signals。 |

## 报告中需要校正的结论

1. `ui()` 编码的是标准 CBOR map，首字节为 `0xa0 + 字段数`；`f` 是包含 CBOR map 的 byte string。不是固定 `0xa0` 加自定义 TLV。
2. 同版本 `cte` 明确先执行 `platform !== darwin → li(1, …, signals)`；Windows 是 error 1，Intel macOS 才是 error 2。报告表格中的 Windows error 2 与该 bundle 不符，代码保留源码结果。
3. 一次 P-256 签名证明某个公钥的持有，不独立证明硬件来源；TPM/SE 的可信判断还依赖 native provider 和已注册设备身份。
4. 新的 v1 文件为 20 个匹配字符串、7 个函数；v2 为 49 个原始地址、3 个 panic Location、2 个函数，`FUN_039d5bc8` 包含 `attestation.rs`、`thread_state.rs` 和 Tokio timer 证据，替代此前 0 命中的未完成记录。v1 的 `0x432edc0` 实际出现 `ThreadNameUpdatedNotification` 与相邻合并字符串，不能单独证明回调参数包含 `thread_id`。
5. `Codex Micro` 目前仅有 HID 发现符号与字符串证据。网关没有对应设备操作入口，本轮不据此新增模型请求头、虚构硬件字段或推断产品已发布。
6. 报告 3.6 把 `refresh/finish` 标为 GET；同版本 `a6` 明确对 enroll/refresh start/finish 使用 POST，当前实现按源码发送 POST。新增的 `sky.node` 画面、输入监控与 CUAService 分析属于桌面执行层，没有给出新的网关 HTTP 字段。

## CLI 生命周期的源码交叉核对

公开 `openai/codex` revision `3ee47c10513f1bb57d869ea650aba28638e3d11c` 的 [attestation.rs](https://github.com/openai/codex/blob/3ee47c10513f1bb57d869ea650aba28638e3d11c/codex-rs/app-server/src/attestation.rs) 使用固定 **100ms**，`timeout_seconds` 只是日志字段，不是秒级配置证据。线程 ID 用于选择该线程第一个声明 `requestAttestation` 的连接；实际发送的是 `AttestationGenerateParams {}`，且请求的附带 thread_id 为 `None`。没有合适连接或宿主已释放时不生成头。此公开 revision 没有证明与被分析二进制同一构建，不能把源码字段伪称为完整反编译结果。

当前网关使用本机 provider，不承担 app-server 多宿主连接管理；仅在已选择的本机 provider 上落实相同等待、取消和封装语义。没有新增线程 ID 请求头、虚构 capability 注册、HMAC 或会话签名。无 provider 时的“省略头”和 provider 返回设备不可用 CBOR 时的 `s=0` 是两个不同结果。

HTTP、Compact、托管/透传 WS 与预热复用发送点处理。等待包含获取本地执行槽的时间；全进程最多 64 个未完成 callback，超时后的迟到结果只留在该调用的有界 response channel，不会用于其他请求。不响应取消的第三方 provider 可继续占槽至其返回，后续请求仍在预算内超时；这是 Go 无法强制终止任意函数时的资源保护。现有 native 子进程使用 context 取消。

## 验证与外部条件

本地 HTTP/WS fixture 已验证注册、刷新、真实 ECDSA 验签与错误 challenge 在签名前拒绝。身份摘要、enrollment 与 connection 载荷通过从原 JS 提取的纯函数生成 golden，并与 Go 字节输出比较。新增生命周期探针以相同输入对照 baseline/modified/rollback；单元与 race 测试覆盖提前/等待中取消、不合作 provider、并发饱和、迟到结果隔离和 opaque token，网关 fixture 覆盖 HTTP/Compact 与实际 WS dial 调用点。

真实 native 模块已加载并尝试 hardware-only 创建；当前环境未能创建硬件 key。测试对此记录 unavailable，未视为硬件签名成功。真实上游 enrollment 需要用户取得有效的新鲜 step-up 授权；本地 fixture 不代表上游已接受。既有 Apple DeviceCheck 成功与真实 `X-OAI-IS-Update` 的验证边界继续保留。

本机新增原生发送探针在 100ms 截止时实际返回 s=1，未取得 Apple 证明。设备模块缺失时的 CBOR 编码探针单独调用 `GenerateCodexToken`，与外层 callback 等待测试分开，避免把冷启动超过预算误判为编码错误。
