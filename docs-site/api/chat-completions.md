# Chat Completions

<Endpoint method="POST" path="/v1/chat/completions" />

OpenAI Chat Completions 兼容接口。请求体和响应体遵循 OpenAI 的格式，网关按 Key 所属分组把请求转给对应的上游，并在需要时转换协议。因此 Anthropic、Gemini 等分组的模型也可以用这个端点调用。

多模态分组不支持此端点，返回 404。

## 请求

::: code-group

```bash [curl]
curl -sS "https://api.example.com/v1/chat/completions" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "your-model",
    "messages": [
      {"role": "system", "content": "你是一个简洁的助手。"},
      {"role": "user", "content": "HTTP 502 是什么意思？"}
    ]
  }'
```

```python [Python]
import os
from openai import OpenAI

client = OpenAI(
    base_url="https://api.example.com/v1",
    api_key=os.environ["SUB2API_API_KEY"],
)

completion = client.chat.completions.create(
    model="your-model",
    messages=[
        {"role": "system", "content": "你是一个简洁的助手。"},
        {"role": "user", "content": "HTTP 502 是什么意思？"},
    ],
)
print(completion.choices[0].message.content)
```

```javascript [Node.js]
import OpenAI from 'openai'

const client = new OpenAI({
  baseURL: 'https://api.example.com/v1',
  apiKey: process.env.SUB2API_API_KEY,
})

const completion = await client.chat.completions.create({
  model: 'your-model',
  messages: [
    { role: 'system', content: '你是一个简洁的助手。' },
    { role: 'user', content: 'HTTP 502 是什么意思？' },
  ],
})
console.log(completion.choices[0].message.content)
```

:::

## 常用参数

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 必填。[模型列表](/api/models)返回的模型 ID |
| `messages` | array | 必填。对话消息，每项包含 `role` 和 `content` |
| `stream` | boolean | 为 `true` 时以 SSE 返回，见[流式响应](/guide/streaming) |
| `max_tokens` | integer | 生成内容的 token 上限 |
| `temperature` | number | 采样温度 |
| `tools` | array | 函数调用的工具定义 |

其余字段按 OpenAI 的定义透传或转换。某个字段是否生效取决于上游模型。

## 响应

```json
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1784092800,
  "model": "your-model",
  "choices": [
    {
      "index": 0,
      "message": {"role": "assistant", "content": "502 表示网关从上游收到了无效响应。"},
      "finish_reason": "stop"
    }
  ],
  "usage": {"prompt_tokens": 24, "completion_tokens": 16, "total_tokens": 40}
}
```

计费以响应中的 `usage` 为依据，并乘以分组的[计费倍率](/api/usage#查询当前计费倍率)。
