# Support Chat

本 feature 同时持有 `/support` 用户会话和 `/admin/support` 管理员收件箱。功能开关、消息和保留规则见 [在线客服](../../../../docs/SUPPORT_CHAT.md)。

- [data/datasources/supportChatDatasource.ts](data/datasources/supportChatDatasource.ts)：会话、消息、图片资产、已读、图库、快捷回复、转账及 WebSocket 连接信息。
- [presentation/pages](presentation/pages/)：用户/管理员页面、历史加载和操作编排。
- [presentation/composables](presentation/composables/)：WebSocket、未读轮询与历史合并。
- [presentation/widgets](presentation/widgets/)：输入框、图片、消息列表、会话列表和快捷回复。
- [presentation/utils/sanitizeChatHtml.ts](presentation/utils/sanitizeChatHtml.ts)：消息 HTML 清洗。

每个用户只有一个长期会话，当前客服入口使用管理员权限。消息以服务端 ID 和幂等键去重；上传使用有界并发，文件选择、粘贴和拖放应遵循同一大小/数量限制。受保护图片由认证请求取回，不将 token 放进图片 URL。退出页面释放 object URL、socket 和轮询。

从 `frontend/` 执行 `pnpm exec vitest run src/features/support-chat`；后端入口及事务、撤回、保留测试见 [chat 模块](../../../../backend/internal/modules/chat/README.md)。
