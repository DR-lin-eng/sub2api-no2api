# Gemini 原生接口

<Endpoint method="POST" path="/v1beta/models/{model}:generateContent" />

Gemini API 原生格式的接口，供 Google GenAI SDK 和 Gemini CLI 直连。支持 `generateContent`、`streamGenerateContent` 和 `countTokens` 三个动作。

Key 推荐放在 `x-goog-api-key` 请求头。这组路径也接受 `?key=` 查询参数。

## 生成内容

::: code-group

```bash [curl]
curl -sS "https://api.example.com/v1beta/models/your-model:generateContent" \
  -H "x-goog-api-key: $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [
      {"role": "user", "parts": [{"text": "用一句话解释量子纠缠"}]}
    ]
  }'
```

```python [Python]
import os
from google import genai
from google.genai import types

client = genai.Client(
    api_key=os.environ["SUB2API_API_KEY"],
    http_options=types.HttpOptions(base_url="https://api.example.com"),
)

response = client.models.generate_content(
    model="your-model",
    contents="用一句话解释量子纠缠",
)
print(response.text)
```

```javascript [Node.js]
import { GoogleGenAI } from '@google/genai'

const ai = new GoogleGenAI({
  apiKey: process.env.SUB2API_API_KEY,
  httpOptions: { baseUrl: 'https://api.example.com' },
})

const response = await ai.models.generateContent({
  model: 'your-model',
  contents: '用一句话解释量子纠缠',
})
console.log(response.text)
```

:::

Google GenAI SDK 会自己追加 `/v1beta`，所以 `base_url` 填站点根地址。

### 响应

```json
{
  "candidates": [
    {
      "content": {"role": "model", "parts": [{"text": "量子纠缠是……"}]},
      "finishReason": "STOP"
    }
  ],
  "usageMetadata": {"promptTokenCount": 9, "candidatesTokenCount": 28, "totalTokenCount": 37}
}
```

## 流式生成

<Endpoint method="POST" path="/v1beta/models/{model}:streamGenerateContent?alt=sse" />

请求体与 `generateContent` 相同。加上 `alt=sse` 后，每个 SSE 事件是一段 `GenerateContentResponse`。

```bash
curl -sS -N "https://api.example.com/v1beta/models/your-model:streamGenerateContent?alt=sse" \
  -H "x-goog-api-key: $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"contents": [{"role": "user", "parts": [{"text": "写一首关于秋天的短诗"}]}]}'
```

## 计算 token 数

<Endpoint method="POST" path="/v1beta/models/{model}:countTokens" />

```bash
curl -sS "https://api.example.com/v1beta/models/your-model:countTokens" \
  -H "x-goog-api-key: $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"contents": [{"role": "user", "parts": [{"text": "你好"}]}]}'
```

```json
{"totalTokens": 2}
```

## 错误格式

这组接口的错误使用 Google 的结构：

```json
{"error": {"code": 401, "message": "Invalid API key", "status": "UNAUTHENTICATED"}}
```

## Antigravity 专用路径

`/antigravity/v1beta/models/...` 只调度 Antigravity 账号。把 SDK 的 `base_url` 设为 `https://api.example.com/antigravity` 即可。
