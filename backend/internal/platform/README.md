# Platform

平台层提供所有业务模块共同依赖的运行能力，但不承载产品业务流程。

| 目录 | 作用 |
| --- | --- |
| `config/` | 环境变量、配置默认值、校验和配置注入 |
| `middleware/` | 非 HTTP 专属的凭据加密、验证码和限流组件 |

HTTP 请求链中间件位于 `transport/http/server/middleware`，不要与本目录混放。
