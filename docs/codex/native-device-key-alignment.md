# Ghidra 原生证明报告与代码对照

本轮以用户提供的 `Ghidra反编译专项报告.md` 为线索，重新核对同版本 `26.1007.71326` 的 main/transport bundle 与 Linux/macOS native 模块。此前已检查 `/tmp/dk_sav.c`；主机重启后临时反编译输出已丢失，证据清单分别标明当前可复核文件与历史摘要。报告、二进制摘要和 JS 字节位置见 [证据清单](native-device-key-evidence.json)。

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
| 原生模块加载失败 | Codex callback 返回真实 `error_code=4` 与实际 signals；旧 Live/raw DeviceCheck 继续拒绝不可用证明。Intel Mac 的 error 2 也保留实际 signals。 |

## 报告中需要校正的结论

1. `ui()` 编码的是标准 CBOR map，首字节为 `0xa0 + 字段数`；`f` 是包含 CBOR map 的 byte string。不是固定 `0xa0` 加自定义 TLV。
2. 同版本 `cte` 明确先执行 `platform !== darwin → li(1, …, signals)`；Windows 是 error 1，Intel macOS 才是 error 2。报告表格中的 Windows error 2 与该 bundle 不符，代码保留源码结果。
3. 一次 P-256 签名证明某个公钥的持有，不独立证明硬件来源；TPM/SE 的可信判断还依赖 native provider 和已注册设备身份。
4. 当前 `codex_attestation_decomp.c` 只含 `matched strings: 0` / `referencing functions: 0`，不能作为已完成 CLI 反编译的证据。
5. `Codex Micro` 目前仅有 HID 发现符号与字符串证据。网关没有对应设备操作入口，本轮不据此新增模型请求头、虚构硬件字段或推断产品已发布。
6. 报告 3.6 把 `refresh/finish` 标为 GET；同版本 `a6` 明确对 enroll/refresh start/finish 使用 POST，当前实现按源码发送 POST。新增的 `sky.node` 画面、输入监控与 CUAService 分析属于桌面执行层，没有给出新的网关 HTTP 字段。

## 验证与外部条件

本地 HTTP/WS fixture 已验证注册、刷新、真实 ECDSA 验签与错误 challenge 在签名前拒绝。身份摘要、enrollment 与 connection 载荷通过从原 JS 提取的纯函数生成 golden，并与 Go 字节输出比较。

真实 native 模块已加载并尝试 hardware-only 创建；当前环境未能创建硬件 key。测试对此记录 unavailable，未视为硬件签名成功。真实上游 enrollment 需要用户取得有效的新鲜 step-up 授权；本地 fixture 不代表上游已接受。既有 Apple DeviceCheck 成功与真实 `X-OAI-IS-Update` 的验证边界继续保留。
