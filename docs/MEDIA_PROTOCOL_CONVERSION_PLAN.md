# 生图与视频接口互转计划

目标是让客户端沿用熟悉的接口。用户换模型，或管理员调整模型映射后，网关把请求转给另一家上游，再按客户端原来的格式返回图片、视频任务或错误。每个转换方向只开放两端都能表达的能力。

2026-11-20 对比了 Sub2API `e921bbb7d` 和 New API `3a407cc9` 的源码。下表记录改造前的基线，真实厂商调用列入后续验收。

| 能力 | Sub2API 现状 | New API 可借鉴之处 |
| --- | --- | --- |
| 生图 | `/v1/images/generations`、`/v1/images/edits` 按 OpenAI、Grok、多模态分组分派。已有 Gemini 原生入口，但专用 Images 入口没有 Gemini 分支。异步图片入口目前只接 OpenAI、Grok。 | 同一 Images 入口能按模型调用火山 Seedream、阿里百炼等。插件同时转换输入和结果，连异步上游也能在有界等待后返回标准图片响应。 |
| 生视频 | 已有 `/v1/videos/generations` 提交、查询和取片，支持 Grok、Agnes 和多模态路径。多模态模板能改提交请求，查询和取片仍使用固定上游路径。 | `POST /v1/videos` 接受统一请求，Seedance、可灵、Veo 各自处理提交、状态和产物。插件还可声明火山任务接口等厂商原生入口。 |

本项目实现见[网关分派](../backend/internal/transport/http/server/routes/gateway.go)、[多模态转发](../backend/internal/application/service/multimodal_media.go)和[异步图片入口](../backend/internal/transport/http/handler/image_task_handler.go)。New API 对照见[公共协议](https://github.com/QuantumNous/new-api/blob/3a407cc9b064fe79a4ffac4146b2690c465dc929/pkg/jsplugin/routing.go#L89-L105)、[火山](https://github.com/QuantumNous/new-api/blob/3a407cc9b064fe79a4ffac4146b2690c465dc929/plugins/tasks/doubao/plugin.js#L343-L354)、[百炼](https://github.com/QuantumNous/new-api/blob/3a407cc9b064fe79a4ffac4146b2690c465dc929/plugins/tasks/alibaba/plugin.js#L370-L374)、[可灵](https://github.com/QuantumNous/new-api/blob/3a407cc9b064fe79a4ffac4146b2690c465dc929/plugins/tasks/kling/plugin.js#L30-L39)和[Veo](https://github.com/QuantumNous/new-api/blob/3a407cc9b064fe79a4ffac4146b2690c465dc929/plugins/tasks/google/plugin.js#L36-L46)。

1. **先定转换规则。** 为每个模型列出支持的操作和参数，例如参考图、遮罩、视频时长、画幅、首尾帧。客户端入口负责解析请求、按原协议写回结果，每家上游只需实现自己的调用方式。沿用现有分组权限、模型映射、选号和计费。新转换遇到无法表达的参数时，在提交前指出具体字段。
2. **先打通 OpenAI 与火山的生图。** 火山同时提供图片和视频，适合两期复用。先让 OpenAI Images 请求直连 Seedream，再让火山原生生图请求通过模型映射调用 OpenAI。文生图和带参考图编辑各测两个方向，核对 JSON、文件上传及调用方要求的 `url` 或 `b64_json`。流式和非流式分别验证，仅对通过的模型开放。
3. **再打通 Seedance 与可灵的视频。** 新增 OpenAI 兼容的 `POST /v1/videos`，与已有 `/v1/videos/generations` 共用任务处理。接入火山和可灵的原生格式，让火山客户端能调可灵、可灵客户端也能调 Seedance。公开任务 ID 关联提交协议、用户、分组、API Key、原账号和上游任务 ID。已受理的任务断线后按原账号续查。沿用多模态余额冻结和结算，重复轮询只结算一次，确认失败后解冻。
4. **按同一套用例扩厂商。** 下一批图片建议接 Google、阿里百炼，并复用已有 Gemini 原生生图与 Responses 图片工具入口。视频再接 Veo、通义万相。用百炼异步生图验证同步入口的等待上限和异步入口的任务恢复。媒体工坊读取同一份模型能力配置。每家先跑协议样例，再用真实账号生成、核对用量，按分组开放。

首期验收时，每种入口用一段固定的客户端代码，只换模型。图片必须能按原格式读出成品。视频要分别跑通文生视频、图生视频、查状态和下载，再验证重启后能查旧任务、其他用户无法取片、同一成功任务只出一笔账单。

当前已接入从现有图片及视频入口到方舟的出站转换。Docker 验证使用实际网关、PostgreSQL、Redis 和模拟方舟，覆盖模型映射、参考图、原任务归属恢复、取片与账单。方舟客户端反向调用其他上游、可灵和标准 `POST /v1/videos` 入口仍列入后续阶段。[启用方式与支持参数](CUSTOM_MODELS_AND_MEDIA.md#多模态账号与火山方舟)
