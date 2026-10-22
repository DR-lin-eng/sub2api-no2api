# Platform

平台层提供所有业务模块共同依赖的运行能力，但不承载产品业务流程。

| 目录 | 作用 |
| --- | --- |
| `config/` | 环境变量、配置默认值、校验和配置注入 |
| `middleware/` | 非 HTTP 专属的凭据加密、验证码和限流组件 |
| `devicekey/` | 官方 native TPM/SE/Keychain 密钥 create/public/sign/delete 桥 |
| `liveattestation/` | macOS 原生 DeviceCheck、独立 raw token 与 Codex callback/Live envelope、不可变账号/安装/线程 scope 与本地 provider 选择；Linux 明确报告 unsupported 平台，不伪造 Apple token |

HTTP 请求链中间件位于 `transport/http/server/middleware`，不要与本目录混放。
