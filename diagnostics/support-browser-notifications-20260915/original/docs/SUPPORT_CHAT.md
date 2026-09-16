# 在线客服

在线客服提供用户与管理员之间的单一长期会话。用户入口为 `/support`，管理员收件箱为 `/admin/support`；`support_chat_enabled` 关闭时两侧路由都由后端功能守卫拒绝。当前实现没有独立客服角色，管理员即客服处理方。

## 会话与消息

每个用户最多有一个客服会话，首次访问或发送消息时按用户创建/取得。用户只能读取和写入自己的会话；管理员可以在收件箱分页查看会话、搜索用户或消息、筛选未读并打开任意会话。

消息类型包括文本、图片、贴纸和余额转账。文本可引用已有消息；发送请求带幂等键，重复请求返回同一成功消息，不能用相同键创建不同消息。普通消息写入和会话未读状态更新原子完成，数据库写入成功后才通过 WebSocket 广播。

用户和管理员分别维护已读时间。管理员可以设置自己的未读提醒，该提醒不增加真实用户消息的未读数。撤回是有权限和状态限制的服务端动作，客户端收到撤回事件后仍可通过历史接口取得一致状态。

## 图片、快捷回复与转账

图片上传先保存受保护资产，再由消息引用资产 ID；消息列表通过认证接口读取资产。单个资产最大 5 MiB，单条消息最多引用 4 个资产；前端的选择器、粘贴和拖放应使用同一限制。图库和贴纸由管理员维护，快捷回复最多 50 条并支持导入、排序和更新。

余额转账由服务端客服转账用例执行，消息中的 `balance_transfer` 元数据是结果展示，不是余额事实源。转账成功后才广播消息和刷新双方视图；余额变更仍以余额/账务服务记录为准。

## 实时与保留

HTTP 历史接口是事实源，WebSocket 只负责当前进程的消息、撤回和已读广播。断线后前端应重新连接并按游标/分页合并历史，不能只依赖丢失的实时事件。多实例部署时，当前 Hub 不自动提供跨实例广播；扩展时需增加明确的跨节点投递和去重方案。

启用保留清理后，后台任务每 10 分钟尝试运行，单次使用最多 2 分钟、每批最多 500 条且最多 100 批，并使用共享租约避免多个实例重复清理。管理员修改策略后，任务在批次间重新读取；设为 0 表示永久保留普通客服消息。未挂接资产按独立清理规则处理，不能把备份保留策略当作客服保留策略。

## 入口与事实源

- 用户 HTTP：`/api/v1/chat/conversation`、`/messages`、`/assets`、`/read`、`/unread-count`；用户 WebSocket：`/api/v1/chat/ws`。
- 管理 HTTP：`/api/v1/admin/chat/*`，包括会话、消息、撤回、资产、转账、图库、贴纸、快捷回复和 WebSocket。
- 前端 owner：[support-chat](../frontend/src/features/support-chat/README.md)。
- 领域规则：[chat module](../backend/internal/modules/chat/README.md)；保留与转账编排在 `backend/internal/application/service/support_chat_*.go`。

完整字段和错误码以 handler、DTO、datasource 与模块类型为准。敏感图片 URL、认证 token 和余额细节不能写入日志或普通前端持久存储。

## 验证

从仓库根目录执行：

```sh
cd backend && go test ./internal/modules/chat
cd backend && go test ./internal/application/service -run 'TestSupportChat'
cd backend && go test ./internal/transport/http/handler/... -run 'TestChat'
cd frontend && pnpm exec vitest run src/features/support-chat
```

涉及 PostgreSQL 权限、资产访问或多实例行为时，还需运行 chat repository integration tests。
