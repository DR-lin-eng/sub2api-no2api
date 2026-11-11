# 普通生图参数忽略与 GIF 新路径（2026-11-11）

## 触发请求

本次复现采用用户提供的标准 Images 编辑参数：

```text
model=gpt-image-2.6-sunburst
output_format=gif
size=auto
quality=low
complexity=high
moderation=low
frame_count=9
frame_duration_ms=200
```

旧选路把非默认 `quality`、`complexity`、`moderation` 视为原生能力，普通 OAuth 因此进入 Codex Images 编辑端点并返回 `GIF output is not enabled for this request.`。

## 当前选路

普通 OAuth 的可选图像控制现在按以下规则处理：

- GIF 的质量、复杂度、审核、参考图保真度和风格字段转为 App 动画指令，原字段不进入 App 请求体。
- 普通图片生成的尺寸、质量、复杂度、背景、格式、压缩、审核、安全标识等可选字段直接忽略，继续走 App 新路径；App 返回的真实 MIME/尺寸作为结果事实。
- `output_format=gif` 仍使用动画指令和帧校验；`output_format=svg`、mask、markup 等改变输出语义的字段保留原生路径。
- API Key、SIWC OAuth 不进入 App，继续使用标准 Images API。

| 参数 | 普通 OAuth 静态生图/编辑 | 普通 OAuth GIF |
| --- | --- | --- |
| `size`, `background` | 忽略，使用实际返回尺寸/背景 | 忽略，使用实际返回尺寸/背景 |
| `quality`, `complexity`, `moderation`, `input_fidelity`, `style` | 忽略 | 转为动画指令，原字段不出站 |
| `output_format=png/jpeg/webp` | 忽略，输出以实际 MIME 为准 | 不适用 |
| `output_compression`, `partial_images` | 忽略 | 原有 GIF 非法组合校验仍会返回 400，不切换协议 |
| `safety_identifier`, `user` | 忽略 | 忽略 |
| `output_format=gif`, `frame_count`, `frame_duration_ms` | 按 GIF 请求处理 | 转动画指令并验证实际帧数/时长 |
| `output_format=svg`, `svg`, `mask`, `markup` | 原生链路，保留既有 SVG/编辑行为 | 仍遵循原有组合校验 |
| `prompt`, 参考图, `n`, `stream`, `response_format` | 既有 App 适配保持有效 | `stream` 仍遵循 GIF 限制 |

以上行为适用于新版模式。显式开启旧版生图时仍使用旧版链路。字段类型、模型有效值、大小限制和 GIF/SVG 非法组合校验保留；忽略可选控制不等于接受非法请求。

官方合同参考：[Create image edit](https://developers.openai.com/api/reference/resources/images/methods/edit)。

## 回归

Go 1.26.11 Docker 专项通过：

- `TestChatGPTAppOrdinaryImagesIgnoreOptionalNativeControls`
- `TestChatGPTAppGIFStandardClientControlsStayOnAppPath`
- `TestChatGPTAppPNGToGIFCurlControlsDoNotSelectNativeEndpoint`
- `TestImagesProtocolAPIKeyNativeStreaming`
- `TestImagesProtocolOAuthHardNativeSVGKeepsOpenAIContract`

随后 service、repository、HTTP 全量 unit 回归及 golangci-lint 均通过。PNG 编辑 JSON/multipart、Azure PUT 上传和双网关帧计费回归继续由 [PNG/GIF 编辑验证](../png-gif-edits-20261111/VERIFICATION.md) 覆盖。

## 实际双网关对照

[runtime/check.py](runtime/check.py) 使用两套真实 Docker 网关、独立数据库/Redis、本地 App 上游与 HTTPS 参考图存储。基线固定为 main `330d81ba7` 对应的发布摘要，修复镜像来自同一生产代码补丁。基线、修复、回退、恢复共 56 项检查通过，结果见 [runtime/result.json](runtime/result.json)。

两款 2.6 模型各覆盖普通生成、普通流式、普通 JSON/multipart 编辑，以及 GIF 生成、GIF JSON/multipart 编辑。请求同时包含尺寸、背景、质量、复杂度、风格、审核、保真度与安全字段；静态请求额外带 JPEG、压缩和部分图片参数。

修复后所有这些请求均进入 App conversation，可选原生字段没有进入 App JSON。普通请求要求 JPEG 和非实际尺寸时，上游返回的 PNG 和实际尺寸仍被正确交付；GIF 文件返回 9 帧，两端只结算一次且均计为 9 个单位。参考 PNG 的实际 PUT 字节、HTTPS 和无账号凭据均验证。基线普通流式请求可能先发 HTTP 200 心跳再发错误，检查同时确认没有图片完成事件，不能据 HTTP 200 算生成成功。

此对照未再次使用真实收费账号。所有测试凭据、证书私钥、容器、网络和数据库均为临时 fixture，验收后清理。
