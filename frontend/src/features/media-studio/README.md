# Media Studio

媒体工坊 feature 提供图片、视频和批量媒体生成能力的用户端工作台。

- `presentation/pages/`: 路由入口，装配 `AppLayout` 和 `MediaStudioCanvas`，连接 `useMediaStudioController`。
- `presentation/widgets/`: `MediaStudioCanvas` 组合输入框、模式选择、参数面板、会话列表。
- `presentation/composables/`:
  - `useMediaStudioPreview`: 模式元数据（图片、视频、批量）和纯选择逻辑。
  - `useMediaStudioController`: 状态管理、API Key 加载、模型列表、图片/视频生成、任务轮询和受保护媒体预览。
- `data/datasources/`: `mediaStudioDatasource` 封装模型、图片生成、视频生成/状态/内容协议。

当前版本支持同步图片生成，也兼容注入异步图片任务流程；视频模式可提交任务、轮询状态并通过所选 API Key 获取受保护内容；批量模式复用现有 `BatchImageGuide` 工作区。提示词和生成结果只保留在当前页面内存中，本地存储仅保存 Key 选择与生成参数，不持久化敏感内容。
