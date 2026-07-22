# Routes

路由层只声明路径、方法、中间件和 handler 绑定。

| 文件 | 路由域 |
| --- | --- |
| `admin.go` | `/api/v1/admin` |
| `auth.go` | 登录、OAuth 和会话 |
| `gateway.go` | LLM、OpenAI/Codex、图片和兼容入口 |
| `payment.go` | 支付与回调 |
| `user.go` | 用户侧 API |
| `common.go` | 健康检查和公共路由辅助 |

新增路由必须放入所属域文件；不要在路由闭包中实现业务逻辑。
