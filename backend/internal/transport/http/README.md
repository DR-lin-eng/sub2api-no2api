# HTTP Transport

HTTP 传输由 handler 和 server 两部分组成。

| 目录 | 作用 |
| --- | --- |
| `handler/` | 参数绑定、协议响应、流式连接和 application 调用 |
| `server/` | Gin 实例、路由注册、HTTP 中间件链和服务生命周期 |

handler 不注册路由，routes 不实现业务，middleware 不访问 repository。WebSocket 与 SSE 也遵循相同边界。
