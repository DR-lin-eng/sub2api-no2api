# ChatGPT App 生图与旧版回退

面向网关维护者和 API 使用方。依据用户提供的 20261107 静态报告及包内请求、上传、
增量解码和 topic 模块，对齐 macOS `26.1103.41825 / build 25679`、Linux
`26.1103.32239 / build 25471` 的独立 Images 页面。来源和 Docker 验证见
[验证记录](../diagnostics/chatgpt-images-20261107/README.md)。

## 选择版本

管理端「系统设置 → 网关 → 请求转发行为」提供「使用旧版 OpenAI 生图」开关。

| 开关 | 普通 ChatGPT OAuth 的专用 Images 请求 |
| --- | --- |
| 关闭，默认 | 新版 App `/backend-api/f/conversation`，使用 `picture_v2` |
| 开启 | 原有 Codex Responses 图片工具或 Codex 原生 Images 端点 |

设置字段为 `openai_images_legacy_enabled`。未保存过此字段的实例默认使用新版。
保存后本实例立即生效；其他实例最多 60 秒刷新。每个请求捕获一次选择，已开始的生成、
续接、历史查询和下载不会因中途修改设置而切换版本，也不会在失败后自动换协议重新生图。

主设置 API 支持只提交这个字段，其他设置保持已保存值：

```json
{"openai_images_legacy_enabled": true}
```

普通 ChatGPT OAuth 的 `/v1/images/generations`、`/v1/images/edits` 和已配置管理端生图测试
使用同一开关。API Key、SIWC、PAT、Agent Identity 保留各自已有协议。
带图片工具的混合 Responses 请求继续遵循其原有 Responses 合同。

## 新版请求与结果

新版将图片 API 的 prompt 组织为用户消息，生成独立 message、parent 和 image-send UUID，
使用 `picture_v2`、`supported_encodings:["v1"]` 及 App profile headers。
聊天驱动模型继续由 `SUB2API_IMAGES_MAIN_MODEL` 选择；报告没有确认 App 的内部图像模型 ID。
请求中的图像模型名作为网关路由及计价标识，不表示 App 内部一定执行了同名 GPT Image 模型。

编辑参考图先创建 `/files` 条目，向返回的上传地址 PUT 字节，再消费
`/files/process_upload_stream` 的 NDJSON 就绪事件。引用按 `file_` 前缀选择
`sediment://` 或 `file-service://`，不保存到 Library。用户提供的 HTTP 图片输入继续经过
现有公开目的地校验；所有请求保留所选账号的代理、IPv6 出口和 TLS profile。

流解码支持 v1 的通道、字段继承、patch、数组插入、append、truncate 和普通消息快照。
输入引用、进行中的图像条目及尚未完成的像素高度不作为最终出图。
结果可来自裸 base64、data URL、`image_generation_call` 或消息里的图像文件指针。
指针通过 `/files/download/{file_id}` 解析，inline retry 使用有限退避。
外部存储地址执行公开目的地校验和 DNS 固定，不接收账号 bearer 或 workspace 头。

SSE 的初始/空闲等待分别为 60 秒。topic handoff 使用 `/celsius/ws/user` 返回的地址，
以 `offset:"0"` 请求历史，消费 recovered catchups，校验非空父事件 ID 并去重；
nullable stream ID、heartbeat 和 encoded SSE item 分别处理。
WebSocket 初始等待 5 秒、后续空闲 30 秒。恢复只续接原 conversation；
HTTP resume 最多 3 次，随后按元数据进行有限历史查询。
网关对包含上传与恢复的整次请求另设 15 分钟上限，这与 App 恢复 freshness 窗口是不同约束。

## 参数与计价

新版支持 prompt、参考图、`n`（最多 10）、流式/非流式及 `b64_json`/`url` 结果表示。
多张请求按需串行创建独立生成；已取得足够图片时停止创建新会话。
App 不提供本项目可以确认的原生尺寸、质量、复杂度、动画、SVG、mask 或 markup 参数合同。
显式要求这些保证时，网关在上传/生成前返回 400，提示使用旧版；不会静默丢弃参数。

媒体工坊通过配置中的 `openai_images_protocol` 选择控件和请求字段。
可能调度普通 OAuth 的分组使用新版共同参数，并将模型选择标为计费模型；
仅包含原生 API Key/SIWC/PAT 等账号的普通 OpenAI 分组保留原生控件。
旧服务未返回该能力字段时，前端继续使用旧请求合同。

App 不保证返回图像 token 用量。新版默认沿用已有图片价格，按实际完成张数和实际图片尺寸结算。
若管理员显式配置 token 价格且分组没有启用既有按张计费，生成前会拒绝请求；
可配置图片/按次价格、启用分组按张计费，或使用旧版 token 通道。
已返回的用量保留为实际观测数据，不从图片大小、进度或文本长度猜测 token。

生成已完成、但结果下载失败时，返回错误并保留完成张数、已知尺寸和 conversation 标识，
交给现有部分成功与可靠结算流程处理，不重新生成。上游请求 ID 保留模型请求的值，
不会被后续文件下载的请求 ID 覆盖。

## 验证准备与边界

请求先调用 `/sentinel/chat-requirements/prepare`，将服务端实际返回的 token 仅用于
同一 principal、同一 backend 的生成/续接请求。Go 网关没有 App 浏览器上下文，
prepare 的 `p` 使用包内实现提供的错误信封分支，明确报告浏览器上下文不可用。
该信封不声称浏览器遥测、设备证明或 challenge 已成功。
需要浏览器 VM/Turnstile 或 proof-of-work 的响应返回明确的 `chatgpt_verification_required`，
由用户在 App 完成验证或由管理员切回旧版。不会执行上游下发的浏览器程序。

Library 笔记的 `/wham/images/*` 是另一套单图、最小 JSON、`retry:never` 合同，
没有替换本次选择的独立 Images 页面通道。App 生图可能创建所选上游账号的聊天历史。
本地协议测试不能证明真实账号的图片权限、验证通过率、内部模型名称或上游性能。

## 升级与回退

没有 Ent schema 或数据库迁移。滚动升级时可先在尚未加入流量的首个新节点开启旧版并保存，
待所有节点升级后再关闭开关；这样旧客户端和混合节点阶段继续使用旧协议。
回退前先打开旧版开关，等待正在进行的生图结束。代码降级后旧节点继续使用原有协议，
额外的设置键不会修改已有账号、价格或生成记录。

代码入口：

- [协议解码模块](../backend/internal/modules/chatgptimages/README.md)
- [新版网关适配](../backend/internal/application/service/openai_images_chatgpt.go)
- [版本设置缓存](../backend/internal/application/service/setting_openai_images_mode.go)
- [媒体工坊](../frontend/src/features/media-studio/README.md)
