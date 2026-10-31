# 自定义模型与媒体工坊

本文说明管理员如何声明模型能力，以及用户媒体工坊如何消费这些能力。两者共享模型名称和能力概念，但自定义配置不是账号凭据、模型价格或网关可用性的替代物。

## 自定义模型配置

管理员页面为 `/admin/custom-model-config`，后端路由为 `/api/v1/admin/custom-model-configs`。配置可以使用完整模型名精确匹配，或使用前缀匹配；运行时先使用精确匹配，再使用最长匹配前缀。能力值包括已实现的图片、视频和音频等标识，视频还可指定对应 API 类型。

请求模板单独管理 `name`、描述、`request_adapter` 和 `attributes`，模型配置通过 `template_id` 关联。模板适配器只在支持的网关/媒体调用位置消费；保存模板不会自动让任意上游支持该协议。`request_adapter.match.mode` 可选 `exact`（默认）或 `prefix`：前缀模式下请求路径命中前缀后，剩余部分会自动拼接到渲染后的 `upstream.path` 之后。

`attributes` 按模态（`image` / `video` / `audio`）声明字段，每个模态块为 `{"mode": "overlay|replace", "fields": [...]}`。`mode` 只影响媒体工坊参数面板的渲染方式，不参与请求适配和计费：

| 情况 | 媒体工坊参数面板 |
| --- | --- |
| 未配置模板，或该模态 `fields` 为空 | 渲染内置参数，与未引入属性配置时一致 |
| `replace` | 只渲染模板声明的字段，内置参数控件全部隐藏 |
| `overlay`（默认） | 内置参数照常渲染，模板中同名字段替换对应控件 |

`overlay` 的同名映射为 `size`、`quality`、`n`（图片）与 `resolution`、`aspect_ratio`、`duration`（视频）；被替换的内置参数不再随请求发送。字段级 `ui: hidden` 由前端隐藏，后端仍按声明校验提交值。归一化、校验和错误码（`MODEL_ATTRIBUTES_INVALID`、`MODEL_ATTRIBUTE_REQUIRED`、`MODEL_ATTRIBUTE_INVALID`）由 `backend/internal/application/service/model_attributes.go` 负责。

`custom_model_config_enabled` 是运行时总开关。关闭时保留管理端配置，但网关运行时不使用它。管理端的 `runtime=1` 查询只返回当前启用的运行时配置，供媒体能力缓存按需加载。后端运行时缓存约 5 秒刷新；前端能力判定缓存 60 秒并合并并发刷新请求，保存或删除后应主动失效。

## 媒体工坊

用户页面为 `/media-studio`，由 `media_studio_enabled` 控制。用户端先读取媒体配置和模型列表，再创建会话；图片可以同步返回或转入异步任务，视频提交任务后轮询状态并通过 API Key 读取受保护内容。批量图片是独立页面 `/batch-image`，当前未内嵌在媒体工坊中，详见 [批量图片](BATCH_IMAGE_MVP.md)。

管理员可以在分组编辑器中启用“可用模型”并选择模型。设置保存在 `groups.model_allowlist`，同时过滤模型列表、Codex manifest、Gemini/Antigravity 列表和批量生图列表，并在网关请求进入合成路由与调度前校验客户端模型名；未启用时保持账号能力发现和旧 `models_list_config` 展示配置兼容。

媒体工坊支持的模型来自后端分组、账号能力和自定义能力解析。配置模型能力只影响能力筛选和请求适配，不创建账号、不绕过 API Key/分组权限，也不改变计费。托管 API Key 只保存在页面内存；本地存储保存分组、模型、生成参数和会话记录，其中会话记录包含提示词文本，图片、视频和参考素材本体存放在按用户隔离的 IndexedDB。服务器不提供会话或作品库归档接口，也不为历史作品新增对象存储写入。异步图片生成不属于本次同步范围，继续沿用主线实现。

管理员还可以在 `/api/v1/admin/media-studio/group-routes` 配置分组的媒体路由。该设置决定媒体请求使用哪些分组；它与普通模型网关路由和自定义模型配置分开维护。每个分组必须指定唯一模态（`image`、`video` 或 `audio`），一个分组只能绑定一种模态；用户端按当前模式只展示对应类型的分组，后端也会拒绝模态不匹配的模型列表和会话请求。

## 失败与安全边界

异步图片/视频的“已受理”不等于生成完成，页面必须轮询终态并处理失败、取消和超时。受保护媒体读取必须走认证请求或受控签名 URL，不能把 access token 拼进资源地址。模型列表中的未知能力应回退到本地化未知状态，不能拼接未经验证的翻译 key。

能力缓存不可在应用启动时为匿名用户强制请求管理员接口；它应在实际媒体或管理场景按需加载。请求模板和媒体配置的 JSON 大小、字段和 URL 校验由后端 handler/service 执行，前端校验只用于交互反馈。

Responses 编辑图片的 `input_image.image_url` 和 `input_image_mask.image_url` 只下载公开 HTTP(S) 地址；是否接受 HTTP 继续遵循 `security.url_allowlist.allow_insecure_http`。私网、回环、链路本地、元数据及共享地址始终拒绝，管理员配置的 `allow_private_hosts` 不适用于这些用户输入。下载器校验全部 DNS 结果、固定连接 IP，并对重定向逐跳校验；保留本地 `/generated/`、base64 data URL 和账号 `file_id` 输入。每次远程下载总超时为 30 秒，原有大小上限继续生效。HTTP/HTTPS 代理需要允许对目标图片端口建立 CONNECT 隧道；SOCKS 代理使用固定 IP。验证及影响说明见 [安全修复记录](SECURITY_REVIEW_RESPONSES_IMAGE_SSRF_20261006.md)。

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
