# 自定义模型与媒体工坊

本文说明管理员如何声明模型能力，以及用户媒体工坊如何消费这些能力。两者共享模型名称和能力概念，但自定义配置不是账号凭据、模型价格或网关可用性的替代物。

## 自定义模型配置

管理员页面为 `/admin/custom-model-config`，后端路由为 `/api/v1/admin/custom-model-configs`。配置可以使用完整模型名精确匹配，或使用前缀匹配；运行时先使用精确匹配，再使用最长匹配前缀。能力值包括已实现的图片、视频和音频等标识，视频还可指定对应 API 类型。

请求模板单独管理 `name`、描述和 `request_adapter`，模型配置通过 `template_id` 关联。模板适配器只在支持的网关/媒体调用位置消费；保存模板不会自动让任意上游支持该协议。

`custom_model_config_enabled` 是运行时总开关。关闭时保留管理端配置，但网关运行时不使用它。管理端的 `runtime=1` 查询只返回当前启用的运行时配置，供媒体能力缓存按需加载。后端运行时缓存约 5 秒刷新；前端能力判定缓存 60 秒并合并并发刷新请求，保存或删除后应主动失效。

## 媒体工坊

用户页面为 `/media-studio`，由 `media_studio_enabled` 控制。用户端先读取媒体配置和模型列表，再创建会话；图片可以同步返回或转入异步任务，视频提交任务后轮询状态并通过 API Key 读取受保护内容，批量图片复用 [批量图片](BATCH_IMAGE_MVP.md) 工作区。

媒体工坊支持的模型来自后端分组、账号能力和自定义能力解析。配置模型能力只影响能力筛选和请求适配，不创建账号、不绕过 API Key/分组权限，也不改变计费。用户页面中的提示词和生成结果只保留当前页面内存；本地存储只保存 Key 选择和生成参数，不保存敏感提示词或媒体内容。

管理员还可以在 `/api/v1/admin/media-studio/group-routes` 配置分组的媒体路由。该设置决定媒体请求使用哪些分组；它与普通模型网关路由和自定义模型配置分开维护。

## 失败与安全边界

异步图片/视频的“已受理”不等于生成完成，页面必须轮询终态并处理失败、取消和超时。受保护媒体读取必须走认证请求或受控签名 URL，不能把 access token 拼进资源地址。模型列表中的未知能力应回退到本地化未知状态，不能拼接未经验证的翻译 key。

能力缓存不可在应用启动时为匿名用户强制请求管理员接口；它应在实际媒体或管理场景按需加载。请求模板和媒体配置的 JSON 大小、字段和 URL 校验由后端 handler/service 执行，前端校验只用于交互反馈。

## 事实源与验证

- 前端 owner：[custom-model-config](../frontend/src/features/custom-model-config/README.md) 与 [media-studio](../frontend/src/features/media-studio/README.md)。
- 后端入口：`backend/internal/transport/http/handler/admin/custom_model_config_handler.go`、`media_studio_handler.go` 与对应 application service。
- 异步图片接口：[异步图片任务](ASYNC_IMAGE_TASKS.md)；通用模型、账号和计费链路：[关键请求链路](REQUEST_LIFECYCLES.md)。

从仓库根目录执行：

```sh
cd backend && go test ./internal/application/service -run 'TestCustomModel|TestMediaStudio'
cd backend && go test ./internal/transport/http/handler/admin -run 'TestCustomModel'
cd frontend && pnpm exec vitest run src/features/custom-model-config src/features/media-studio
```

新增能力、模板字段或媒体模式时，同时更新 DTO、管理端设置、用户模型列表、错误处理、计费和流式/异步测试。
