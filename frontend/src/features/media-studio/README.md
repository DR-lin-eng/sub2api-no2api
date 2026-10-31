# Media Studio

媒体工坊 feature 提供图片、视频和批量媒体生成能力的用户端工作台。

- `presentation/pages/MediaStudioPage.vue`: 按已登录用户 ID 挂载独立工作区，切换账号或登出时卸载旧工作区并中止其视频生成请求。
- `presentation/widgets/`: `MediaStudioWorkspace` 组合创作面板、会话与作品库；`MediaStudioGallery` 每页最多 12 个作品，仅加载当前页的素材。
- `presentation/composables/`: controller 编排模型/分组与浏览器任务；jobs 维护带分组归属的轮询和视频结果；cost estimate 单独维护预估请求生命周期。
- `data/datasources/`: gateway 协议、浏览器素材转换及按用户隔离的 IndexedDB。所有存储操作在发起时捕获 owner ID，无身份时拒绝读写。

图片生成与编辑沿用主线现有提交、协议和轮询行为，本轮不新增异步图片端点或恢复流程。视频仍提交上游任务、轮询和认证读取内容，完成后 Blob 写入浏览器缓存。参考素材只在浏览器转换，随生成请求提交，不单独上传或归档到服务器；单个参考素材上限 20 MiB，合计 32 MiB，同时遵循模型配置的更低限额。

会话与提示词使用按用户区分的 localStorage key，生成素材、参考 Blob、作品库使用同用户独立 IndexedDB。不得读取或把旧的无用户归属历史自动分配给当前用户。localStorage 仅保存素材 cacheKey，不保存媒体字节或 API Key。最多保留 20 条历史会话及每会话 100 条消息；淘汰时清理对应缓存。作品库独立保留至多 100 个作品、512 MiB，并保留删除排除记录，避免已删除作品被会话补录复活。上游只提供 URL 且浏览器不能读取时保留外部引用，其有效期由上游决定。
