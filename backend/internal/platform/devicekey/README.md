# Native Device Keys

`provider.go` 封装桌面安装目录的 `cua_node/bin/node` 与 `native/remote-control-device-key.node`，提供 create/public/sign/delete。私钥留在 native 模块的 TPM、Secure Enclave 或不可导出 Keychain 存储。

接口采用 `ecdsa_p256_sha256`。签名输入是调用方构造的域分隔 UTF-8 JSON，输出是标准 base64 编码的 ASN.1 DER ECDSA 签名。公开密钥是标准 base64 SPKI DER。上层必须校验实际请求绑定，不能把任意 payload 当作已授权 challenge。

`hardware_only` 不接受 `os_protected_nonextractable`。桌面 enrollment 使用显式 `allow_os_protected_nonextractable` 策略；Linux native 仍由 TPM 实现。native 文件缺失、硬件不可用或超时均返回错误，不切换到 Go 软件密钥。stdout 有界，错误不返回 native stderr 或密钥引用。
