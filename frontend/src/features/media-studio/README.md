# Media Studio

媒体工坊 feature 是后续图片、视频和批量媒体生成能力的用户端工作台壳层。

- `presentation/pages/`: 路由入口，只装配 `AppLayout`、当前媒体模式和页面级布局。
- `presentation/widgets/`: 工作台 UI，按 Header、模式栏、参数面板、预览画布、任务/历史侧栏拆分。
- `presentation/composables/`: 本地预览元数据与纯选择逻辑，不发起 HTTP 请求。

当前版本只提供预览壳子，不创建任务、不持久化状态、不接入后端 API。后续接入真实生成时，再在 `data/datasources/` 中增加协议和统一 HTTP 调用，并补充相邻测试。
