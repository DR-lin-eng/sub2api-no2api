# HTTP Middleware

本目录包含只与请求链相关的鉴权、日志、安全头、限流接入和恢复逻辑。

## 文件索引

| 前缀 | 职责 |
| --- | --- |
| `jwt_auth*`, `api_key_auth*`, `admin_auth*`, `auth_subject*` | 身份解析和权限上下文 |
| `request_*`, `client_request_id*`, `server_timing*` | 请求元数据、日志和时序 |
| `backend_mode_guard*`, `step_up*`, `session_binding*` | 运行模式和敏感操作保护 |
| `ingress_reject*`, `security_headers*`, `cors*` | 入口防护和响应安全 |
| `recovery*`, `logger.go`, `middleware.go`, `wire.go` | 公共链路和注入 |

通用限流器、验证码和凭据加密位于 `internal/platform/middleware`。
