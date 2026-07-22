# HTTP Handlers

handler 负责协议边界：校验请求、提取调用上下文、调用 application service，并将结果写为 HTTP/SSE/WS 响应。

## 文件索引

| 前缀/目录 | 职责 |
| --- | --- |
| `admin/` | 管理端 handler |
| `dto/` | API 输入输出 DTO 和映射器 |
| `quotaview/` | 配额展示投影 |
| `auth*` | 登录、OAuth、会话和当前用户接口 |
| `gateway*` | Anthropic/Claude 网关入口 |
| `openai*` | OpenAI/Codex/Responses/Images/WS 入口 |
| `payment*`, `batch_image*`, `image_task*` | 支付和图片任务接口 |
| `ops*`, `usage*`, `channel_monitor*` | 运维、用量和监控接口 |
| `wire.go`, `handler.go` | handler 聚合与依赖注入 |

`gateway_handler.go` 只保存 handler 状态与构造器，端点和协议辅助拆在 `gateway_handler_messages.go`, `gateway_handler_models.go`, `gateway_handler_usage.go`, `gateway_handler_errors.go`, `gateway_handler_count_tokens.go`, `gateway_handler_billing_errors.go` 和 `gateway_handler_support.go`。

`openai_gateway_handler.go` 只保存 handler 状态、构造器和共享映射辅助；Responses、Messages、WebSocket、调度、错误处理及网络安全策略分别位于同名前缀的 `responses`, `messages`, `websocket`, `responses_scheduling`, `support` 和 `cyber_policy` 文件。

本包不得导入 repository、Ent、GORM 或 Redis。长 handler 应把协议写入器、失败映射和子流程拆到同名前缀文件。
