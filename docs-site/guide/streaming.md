# 流式响应

文本端点都支持 Server-Sent Events（SSE）流式输出。事件格式与对应的上游协议一致，现有 SDK 的流式接口可以直接使用。

## 开启方式

| 端点 | 开启方式 | 结束标志 |
| --- | --- | --- |
| `POST /v1/chat/completions` | 请求体 `"stream": true` | `data: [DONE]` |
| `POST /v1/responses` | 请求体 `"stream": true` | `response.completed` 事件 |
| `POST /v1/messages` | 请求体 `"stream": true` | `message_stop` 事件 |
| `POST /v1beta/models/{model}:streamGenerateContent` | 使用该动作，并加 `?alt=sse` | 连接关闭 |

## 示例

::: code-group

```bash [curl]
curl -sS -N "https://api.example.com/v1/chat/completions" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "your-model",
    "stream": true,
    "messages": [{"role": "user", "content": "写一首关于大海的短诗"}]
  }'
```

```python [Python]
import os
from openai import OpenAI

client = OpenAI(
    base_url="https://api.example.com/v1",
    api_key=os.environ["SUB2API_API_KEY"],
)

stream = client.chat.completions.create(
    model="your-model",
    stream=True,
    messages=[{"role": "user", "content": "写一首关于大海的短诗"}],
)
for chunk in stream:
    if chunk.choices and chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)
```

:::

`curl` 需要 `-N` 关闭输出缓冲，才能逐块看到内容。

## 客户端注意事项

- **HTTP 200 不等于成功。** 流一旦开始，响应头已经以 200 发出，之后的失败会作为流内的错误事件到达。请同时处理 `error` 类事件。
- **反向代理要关闭缓冲。** 在网关前自建 Nginx 等代理时，需要关闭响应缓冲并调大读超时，否则内容会攒到最后一次性返回。
- **连接中断后重新发起请求。** 文本流不支持断点续传。
- **图片和视频端点的流式行为不同。** 详见[图片生成与编辑](/api/images#流式响应)和[视频生成](/api/videos)。

## WebSocket

`GET /v1/responses` 提供 Responses API 的 WebSocket 模式，供支持该传输方式的客户端（例如 Codex CLI）使用。普通集成使用 SSE 即可。
