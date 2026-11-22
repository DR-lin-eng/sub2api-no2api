# Embeddings

<Endpoint method="POST" path="/v1/embeddings" />

OpenAI Embeddings 兼容接口，把文本转换为向量。

::: warning 仅 OpenAI 分组可用
其他平台的分组调用此端点会返回 `404 Embeddings API is not supported for this platform`。
:::

## 请求

::: code-group

```bash [curl]
curl -sS "https://api.example.com/v1/embeddings" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "your-embedding-model",
    "input": "Sub2API 是一个 AI API 网关"
  }'
```

```python [Python]
import os
from openai import OpenAI

client = OpenAI(
    base_url="https://api.example.com/v1",
    api_key=os.environ["SUB2API_API_KEY"],
)

result = client.embeddings.create(
    model="your-embedding-model",
    input="Sub2API 是一个 AI API 网关",
)
print(len(result.data[0].embedding))
```

:::

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 必填。分组中可用的向量模型 ID |
| `input` | string 或 array | 必填。一段文本或文本数组 |

## 响应

```json
{
  "object": "list",
  "data": [{"object": "embedding", "index": 0, "embedding": [0.0123, -0.0456]}],
  "model": "your-embedding-model",
  "usage": {"prompt_tokens": 9, "total_tokens": 9}
}
```
