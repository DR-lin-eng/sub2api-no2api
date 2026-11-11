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

官方合同参考：[Create image edit](https://developers.openai.com/api/reference/resources/images/methods/edit)。

## 回归

Go 1.26.11 Docker 专项通过：

- `TestChatGPTAppOrdinaryImagesIgnoreOptionalNativeControls`
- `TestChatGPTAppGIFStandardClientControlsStayOnAppPath`
- `TestChatGPTAppPNGToGIFCurlControlsDoNotSelectNativeEndpoint`
- `TestImagesProtocolAPIKeyNativeStreaming`
- `TestImagesProtocolOAuthHardNativeSVGKeepsOpenAIContract`

随后 service、repository、HTTP 全量 unit 回归及 golangci-lint 均通过。PNG 编辑 JSON/multipart、Azure PUT 上传和双网关帧计费回归继续由 [PNG/GIF 编辑验证](../png-gif-edits-20261111/VERIFICATION.md) 覆盖。
