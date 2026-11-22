# 错误处理

网关返回标准 HTTP 状态码。响应体的结构跟随端点所属的协议，共有四种。

## 错误响应结构

::: code-group

```json [OpenAI 兼容端点]
{
  "error": {
    "type": "invalid_request_error",
    "message": "Model \"foo\" does not exist or is not available for this group",
    "code": "model_not_found",
    "param": "model"
  }
}
```

```json [Anthropic 兼容端点]
{
  "type": "error",
  "error": {
    "type": "authentication_error",
    "message": "Invalid API key"
  }
}
```

```json [Gemini 原生接口]
{
  "error": {
    "code": 401,
    "message": "Invalid API key",
    "status": "UNAUTHENTICATED"
  }
}
```

```json [鉴权层]
{
  "code": "INVALID_API_KEY",
  "message": "Invalid API key"
}
```

:::

`code` 和 `param` 只在部分错误中出现。鉴权层的错误码见[认证](/guide/authentication#鉴权与计费检查)。

通用的解析方式：先看 HTTP 状态码，再依次尝试读取 `error.message` 和顶层的 `message`。

## 状态码

| 状态码 | 含义 | 建议处理 |
| --- | --- | --- |
| 400 | 请求参数或请求体不合法 | 按错误信息修正请求，不要原样重试 |
| 401 | 缺少 Key，或 Key 无效、已停用 | 检查 Key 和请求头 |
| 403 | Key 过期、余额不足、无订阅、IP 受限或分组不可用 | 检查账户状态和 Key 设置 |
| 404 | 路径不存在，或当前分组不支持该端点或模型 | 对照[分组与端点的对应关系](/guide/overview#分组决定可用端点) |
| 413 | 请求体超过站点设置的上限 | 减小请求体，例如压缩图片 |
| 429 | 触发限速、并发上限，或额度用完 | 按 `Retry-After` 等待后重试；额度用完时不要重试 |
| 500 | 网关内部错误 | 稍后重试，持续出现请联系站点管理员 |
| 502 | 上游返回了错误或无效响应 | 退避后重试 |
| 503 | 暂无可用的上游账号，或服务过载 | 退避后重试 |
| 504 | 上游响应超时 | 退避后重试，或改用流式、异步端点 |

## 重试建议

- 只重试 429（限速类）、500、502、503 和 504。
- 使用指数退避并加入随机抖动，例如 1 秒、2 秒、4 秒，最多 3 到 5 次。
- 响应带有 `Retry-After` 时，以它为准。
- 图片和视频的提交请求不要盲目重试，重复提交会生成并计费多次。先用任务号查询状态。

## 请求追踪

网关的每个响应都带有 `X-Client-Request-ID` 响应头。向站点管理员反馈问题时，请提供这个值、请求时间、模型名和完整的错误响应体。
