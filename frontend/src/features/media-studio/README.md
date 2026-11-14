# Media Studio

媒体工坊 feature 提供图片、视频和批量媒体生成能力的用户端工作台。

- `presentation/pages/MediaStudioPage.vue`: 按已登录用户 ID 挂载独立工作区，切换账号或登出时卸载旧工作区并中止其视频生成请求。
- `presentation/widgets/`: `MediaStudioWorkspace` 组合创作面板、会话与作品库；`MediaStudioGallery` 每页最多 12 个作品，仅加载当前页的素材。
- `presentation/composables/`: controller 编排模型/分组与浏览器任务；jobs 维护带分组归属的轮询和视频结果；cost estimate 单独维护预估请求生命周期。
- `data/datasources/`: gateway 协议、浏览器素材转换及按用户隔离的 IndexedDB。所有存储操作在发起时捕获 owner ID，无身份时拒绝读写。

图片生成和编辑继续提交既有图片 API。后端媒体配置通过 `openai_images_protocol` 宣告 App 或原生协议能力；
新版 App 分组隐藏其未声明支持的原生尺寸、质量和复杂度，并将模型名明确展示为路由/计费标识。
仅包含原生账号的 OpenAI 分组保留原生控件。模板和重试仍保留各自所属的参数 owner。
App 流恢复由后端负责，没有新增客户端异步任务入口；版本选择与限制见 [App 生图](../../../../docs/CHATGPT_APP_IMAGES.md)。
视频继续提交上游任务、轮询及认证读取内容，完成后将 Blob 写入浏览器缓存。参考素材在浏览器编码，随生成请求提交；
单个参考素材上限 20 MiB，合计 32 MiB，同时遵循模型配置的更低限额。

GPT Image 2.6 的原生通道提供 SVG/GIF 格式、GIF 帧数和帧时长、可选 SVG 作者与推理强度；参数随生成、编辑、预估、历史和重试保留。SVG 仅支持无参考图的单文件生成，ChatGPT App 分组隐藏原生格式控件。开启实际张数计费时 GIF 按帧计费，Token 计费保留上游实际用量。

GPT Image 2.6 提供独立复杂度选项及 `xhigh`/`max` 质量；参数随历史和重试保留，并参与既有模板覆盖规则。GIF/SVG 结果保留真实 MIME 和下载后缀，SVG 通过图片元素展示。模型目录、参数边界及作者计费见 [2.6 适配说明](../../../../docs/GPT_IMAGE26.md)。

会话与提示词使用按用户区分的 localStorage key，生成素材、参考 Blob、作品库使用同用户独立 IndexedDB。不得读取或把旧的无用户归属历史自动分配给当前用户。localStorage 仅保存素材 cacheKey，不保存媒体字节或 API Key。最多保留 20 条历史会话及每会话 100 条消息；淘汰时清理对应缓存。作品库独立保留至多 100 个作品、512 MiB，并保留删除排除记录，避免已删除作品被会话补录复活。上游只提供 URL 且浏览器不能读取时保留外部引用，其有效期由上游决定。
