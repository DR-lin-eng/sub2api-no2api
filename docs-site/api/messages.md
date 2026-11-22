# Messages（Anthropic）

<Endpoint method="POST" path="/v1/messages" />

Anthropic Messages 兼容接口，也是 Claude Code 使用的接口。Anthropic 类分组直接转发，其他分组由网关转换协议。

多模态分组不支持此端点，返回 404。

## 请求

Anthropic SDK 会自己追加 `/v1/messages`，所以 `base_url` 填站点根地址，不带 `/v1`。

::: code-group

```bash [curl]
curl -sS "https://api.example.com/v1/messages" \
  -H "x-api-key: $SUB2API_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "your-model",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

```python [Python]
import os
from anthropic import Anthropic

client = Anthropic(
    base_url="https://api.example.com",
    api_key=os.environ["SUB2API_API_KEY"],
)

message = client.messages.create(
    model="your-model",
    max_tokens=1024,
    messages=[{"role": "user", "content": "你好"}],
)
print(message.content[0].text)
```

```javascript [Node.js]
import Anthropic from '@anthropic-ai/sdk'

const client = new Anthropic({
  baseURL: 'https://api.example.com',
  apiKey: process.env.SUB2API_API_KEY,
})

const message = await client.messages.create({
  model: 'your-model',
  max_tokens: 1024,
  messages: [{ role: 'user', content: '你好' }],
})
console.log(message.content[0].text)
```

:::

## 常用参数

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 必填。[模型列表](/api/models)返回的模型 ID |
| `max_tokens` | integer | 必填。生成内容的 token 上限 |
| `messages` | array | 必填。对话消息 |
| `system` | string 或 array | 系统提示词 |
| `stream` | boolean | 为 `true` 时以 SSE 返回，见[流式响应](/guide/streaming) |
| `tools` | array | 工具定义 |
| `thinking` | object | 扩展思考设置 |

## 响应

```json
{
  "id": "msg_abc123",
  "type": "message",
  "role": "assistant",
  "model": "your-model",
  "content": [{"type": "text", "text": "你好！有什么可以帮你？"}],
  "stop_reason": "end_turn",
  "usage": {"input_tokens": 8, "output_tokens": 12}
}
```

## 计算 token 数

<Endpoint method="POST" path="/v1/messages/count_tokens" />

请求体与 `/v1/messages` 相同，不需要 `max_tokens`，返回输入的 token 数，不生成内容。

```bash
curl -sS "https://api.example.com/v1/messages/count_tokens" \
  -H "x-api-key: $SUB2API_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "your-model",
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

```json
{"input_tokens": 8}
```

## Antigravity 专用路径

`/antigravity/v1/messages` 只调度 Antigravity 账号，不与其他平台混合调度。把 `base_url` 设为 `https://api.example.com/antigravity` 即可。
