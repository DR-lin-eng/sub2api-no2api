# GPT Image 2.6 模型适配

面向网关、计费和媒体工坊维护者。2026-11-07 核对官方
[Sunburst](https://developers.openai.com/api/docs/models/gpt-image-2.6-sunburst)、
[Flare](https://developers.openai.com/api/docs/models/gpt-image-2.6-flare)、
[图像生成指南](https://developers.openai.com/api/docs/guides/image-generation) 和
[Images API](https://developers.openai.com/api/reference/resources/images/methods/generate)。

## 模型目录与参数

正式模型 ID 为 `gpt-image-2.6-flare` 和 `gpt-image-2.6-sunburst`。OpenAI、多模态和组合分组的默认目录，
以及管理端对应账号的模型选择器均包含两款模型。账号显式映射、已同步能力快照、分组白名单和自定义展示列表
继续决定最终可见范围；默认目录不会替代管理员限制或上游能力快照。

API Key 账号保留原生 Images 参数并应用既有模型映射、请求模板。OAuth 账号使用 Codex 原生 Images 端点。
`complexity` 支持 `low`、`medium`、`high`，省略时使用上游默认值 `high`，与渲染 `quality` 独立。
2.6 的质量选项包括 `auto`、`low`、`medium`、`high`、`xhigh`、`max`。
JSON 和 multipart 编辑请求均保留复杂度与标注图片 `markup`。GIF 参数可用于生成或编辑，SVG 作者配置仅用于生成。
OAuth 原生请求还保留 `safety_identifier` 和兼容字段 `user`；非空安全标识的优先级由上游执行。

GIF 输出使用 `output_format=gif`，可设置 `frame_count`（2–16）及 `frame_duration_ms`，
不接受流式、部分图片、遮罩或输出压缩参数。按官方指南，SVG 仅用于生成端点，要求 `n=1`，
不接受流式、部分图片和输出压缩；可通过 `svg.model` 和 `svg.reasoning` 选择作者模型及推理设置。
非法组合在发送上游前返回参数错误。

媒体工坊为 2.6 显示复杂度和扩展质量选项，支持保存参数、重试和图片编辑。模型切回旧版本时，
扩展质量回到 `auto`，不自动提交 2.6 复杂度。GIF/SVG 等结果按实际格式预览、缓存和下载，
SVG 使用图片元素展示。高级格式、动画参数及作者配置可由现有请求模板或 API 客户端提交。

## 用量与计费

[官方 Standard 价格](https://developers.openai.com/api/docs/pricing#image-generation)，每百万 token：

| 类型 | 输入 | 缓存输入 | 输出 |
| --- | --- | --- | --- |
| 文本 | $5 | $1.25 | — |
| 图片 | $8 | $2 | $30 |

API Key 和 OAuth 的 Images 响应使用同一份图片用量解析：未报告输出分类时，`output_tokens` 作为图片输出，
明确分类优先。图像缓存只采用明确明细，并受输入和缓存总量约束；只有图文缓存明细时，合计作为缓存读取总量。

SVG 的 `svg_usage` 独立于参考图片 `usage`。Token 计费分别按图片模型和实际作者模型计价：
作者使用普通 token 倍率，参考图片使用既有独立图片倍率，最终合并到一次可靠结算。
显式按张或按次价格覆盖完整作品，不追加作者 token 费用。用量行的输入、输出和推理计数包含作者用量，
图片 token 只记录参考图片部分。沿用现有计费命令、持久队列与幂等处理。

## 代码与验证入口

Go 文件均位于 `backend/internal/`：模型目录在 `shared/openai/constants.go` 和
`application/service/multimodal_media.go`，参数和协议在 `application/service/openai_images26_options.go`
与 `openai_images_direct.go`，作者结算在 `openai_images_svg_billing.go` 和 `openai_gateway_usage.go`。
前端见 [媒体工坊](../frontend/src/features/media-studio/README.md)。

使用固定 `golang:1.26.10-alpine` Docker 容器，在 `backend/` 执行：

```sh
go test -tags=unit ./internal/application/service ./internal/shared/openai ./internal/transport/http/handler \
  -run 'GPTImage26|NativeImage|OpenAIImages|ImageBilling|ImageCount|RecordUsage|ModelListing|GatewayModels|DefaultModels' -count=1
```

前端在带 pnpm 依赖的 Docker 容器运行媒体工坊、`ModelWhitelistSelector` 和多语言门禁测试，
然后执行 `pnpm run typecheck`、`pnpm run lint:check`、`pnpm run build`。
受控上游响应验证协议和本地结算，不证明真实账号拥有 GIF/SVG 预览权限。

2026-11-07 验证结果：Docker 中后端 application/service、HTTP 全子树和 shared/openai 的完整 unit 回归通过，
最终专项回归通过，后端全量 lint 为 0 问题；前端 60 项测试、typecheck、lint 和生产构建通过。生产构建使用 4 GiB Node 堆。
Playwright 在 Docker 提供的真实 Vue 组件上验证 1440px/390px 视口、两款模型选项、参数提交、SVG 预览及刷新恢复。
宿主文档链接和 diff 空白检查通过。源码布局门禁仍被基线已有的 `payment_fulfillment_test.go` 阻挡：
HEAD 中该文件已是 1201 行，超过 1200 行门槛，本次未修改该文件。
