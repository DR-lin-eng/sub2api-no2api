# Media Studio

媒体工坊 feature 是后续图片、视频等生成能力的用户端入口壳层。

- `presentation/pages/`: 路由入口，只装配页面级布局和局部选择状态。
- `presentation/widgets/`: 媒体工坊的展示区块、能力卡片和工作区占位 UI。
- `presentation/composables/`: 本地预览元数据与纯选择逻辑，不发起 HTTP 请求。

当前版本只提供预览壳子，不创建任务、不持久化状态、不接入后端 API。后续接入真实生成时，再在 `data/datasources/` 中增加协议和统一 HTTP 调用，并补充相邻测试。
