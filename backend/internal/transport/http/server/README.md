# HTTP Server

本包构造 Gin/HTTP 服务并管理请求入口生命周期。

| 文件/目录 | 作用 |
| --- | --- |
| `http.go` | `http.Server` 构造和服务级依赖 |
| `router.go` | Gin engine 与全局中间件顺序 |
| `routes/` | 按 API 域注册路由 |
| `middleware/` | HTTP 请求链中间件 |
| `api_contract_test.go` | 跨路由 API 契约回归 |

中间件顺序是行为契约；修改 `router.go` 时必须运行 server、routes 和 handler 测试。
