# Responses

<Endpoint method="POST" path="/v1/responses" />

OpenAI Responses 兼容接口，也是 Codex CLI 使用的接口。OpenAI 类分组直接转发，其他分组由网关转换协议。

多模态分组不支持此端点，返回 404。

## 请求

::: code-group

```bash [curl]
curl -sS "https://api.example.com/v1/responses" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "your-model",
    "input": "用三个要点解释什么是 API 网关"
  }'
```

```python [Python]
import os
from openai import OpenAI

client = OpenAI(
    base_url="https://api.example.com/v1",
    api_key=os.environ["SUB2API_API_KEY"],
)

response = client.responses.create(
    model="your-model",
    input="用三个要点解释什么是 API 网关",
)
print(response.output_text)
```

```javascript [Node.js]
import OpenAI from 'openai'

const client = new OpenAI({
  baseURL: 'https://api.example.com/v1',
  apiKey: process.env.SUB2API_API_KEY,
})

const response = await client.responses.create({
  model: 'your-model',
  input: '用三个要点解释什么是 API 网关',
})
console.log(response.output_text)
```

:::

## 常用参数

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 必填。[模型列表](/api/models)返回的模型 ID |
| `input` | string 或 array | 必填。文本，或由消息和多模态内容组成的数组 |
| `instructions` | string | 系统指令 |
| `stream` | boolean | 为 `true` 时以 SSE 返回，见[流式响应](/guide/streaming) |
| `tools` | array | 工具定义 |
| `reasoning` | object | 推理设置，例如 `{"effort": "medium"}` |

## 响应

```json
{
  "id": "resp_abc123",
  "object": "response",
  "status": "completed",
  "model": "your-model",
  "output": [
    {
      "type": "message",
      "role": "assistant",
      "content": [{"type": "output_text", "text": "1. 统一入口……"}]
    }
  ],
  "usage": {"input_tokens": 18, "output_tokens": 64, "total_tokens": 82}
}
```

## 子路径与其他传输方式

- `POST /v1/responses/{subpath}` 只转发网关支持的子路径，其余返回 `404 Unsupported responses subpath`。
- `GET /v1/responses` 是 WebSocket 模式，供 Codex CLI 等客户端使用。
- `/responses` 和 `/backend-api/codex/responses` 是同一处理逻辑的别名，供只能填写根地址的客户端使用。
