# Support Chat

本模块持有每个用户与客服之间的单一长期会话，不按工单或每次连接拆分会话。当前管理员作为客服；使用与部署边界见 [在线客服](../../../../docs/SUPPORT_CHAT.md)。

## 职责与入口

- [types.go](types.go)：会话、消息、资产引用、未读状态与 repository 端口。
- [service.go](service.go)：收发、引用回复、幂等、撤回、已读和保留清理。
- [asset.go](asset.go)、[quick_reply.go](quick_reply.go)：图片资产、图库、表情和快捷回复。
- [hub.go](hub.go)：当前进程内的 WebSocket 客户端登记与事件广播。
- [wire.go](wire.go)：依赖装配；具体数据库访问在 [repository](../../infrastructure/repository/) 的 `chat*` 文件中。

## 不变量

用户消息和资产读取始终限制在本人会话。消息持久化与会话未读更新必须原子完成；写入成功后才广播。重复幂等键不得让不同消息被当成同一条成功发送。管理员“标为未读”是私人提醒，不增加真实用户未读消息数。

余额转账由 [support_chat_transfer.go](../../application/service/support_chat_transfer.go) 编排，不允许普通消息 payload 自行生成转账事实。普通消息保留清理由 [support_chat_retention.go](../../application/service/support_chat_retention.go) 调度，财务消息和图库的保留规则单独处理。

Hub 不提供跨实例消息总线；WebSocket 事件不能代替数据库历史查询。扩展多节点实时能力时必须明确投递、补拉和去重协议。

## 验证

从 `backend/` 执行：

```sh
go test ./internal/modules/chat
go test ./internal/application/service -run 'TestSupportChat'
go test ./internal/transport/http/handler/... -run 'TestChat'
```

存储权限与原子性另见 repository 的 `chat_security_integration_test.go`；前端入口见 [support-chat README](../../../../frontend/src/features/support-chat/README.md)。
