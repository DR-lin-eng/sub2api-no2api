# Media Studio

媒体工坊 feature 提供图片、视频和批量媒体生成能力的用户端工作台。

- `presentation/pages/`: 路由入口，装配 `AppLayout` 和 `MediaStudioCanvas`，连接 `useMediaStudioController`。
- `presentation/widgets/`: `MediaStudioCanvas` 组合输入框、模式选择、参数面板、会话列表。
- `presentation/composables/`: 
  - `useMediaStudioPreview`: 模式元数据（图片、视频、批量）和纯选择逻辑。
  - `useMediaStudioController`: 状态管理、API Key 加载、模型列表、异步图片生成提交、轮询和本地会话持久化。
- `data/datasources/`: `mediaStudioDatasource` 封装 `/v1/models`、`/v1/images/generations/async`、`/v1/images/tasks/:id` 协议。

当前版本支持图片生成（提交异步任务、轮询、本地会话存储）；视频和批量入口保留为 `available: false`，后续接入。

