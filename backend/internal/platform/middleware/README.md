# Platform Middleware

该 package 保留名称 `middleware`，但只放不依赖具体路由的运行平台组件。

| 文件 | 作用 |
| --- | --- |
| `credential_cipher.go` | 敏感凭据加解密和密钥轮换 |
| `local_captcha.go` | 本地验证码挑战 |
| `local_window_limiter.go` | 进程内窗口限流 |
| `rate_limiter.go` | Redis 分布式限流器 |

请求链鉴权和日志中间件位于 `transport/http/server/middleware`。
