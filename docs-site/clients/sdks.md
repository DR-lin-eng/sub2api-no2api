# 官方 SDK

各厂商的官方 SDK 都可以直接使用，只需要改 API 地址和 Key。不同 SDK 对地址的拼接方式不同，请注意是否带版本前缀。

| SDK | 地址参数 | 填写的值 |
| --- | --- | --- |
| OpenAI（Python、Node.js） | `base_url` / `baseURL` | `https://api.example.com/v1` |
| Anthropic（Python、Node.js） | `base_url` / `baseURL` | `https://api.example.com` |
| Google GenAI（Python、Node.js） | `http_options.base_url` / `httpOptions.baseUrl` | `https://api.example.com` |

## OpenAI SDK

::: code-group

```python [Python]
import os
from openai import OpenAI

client = OpenAI(
    base_url="https://api.example.com/v1",
    api_key=os.environ["SUB2API_API_KEY"],
)
```

```javascript [Node.js]
import OpenAI from 'openai'

const client = new OpenAI({
  baseURL: 'https://api.example.com/v1',
  apiKey: process.env.SUB2API_API_KEY,
})
```

```bash [环境变量]
export OPENAI_BASE_URL="https://api.example.com/v1"
export OPENAI_API_KEY="sk-your-api-key"
```

:::

可用接口：[Chat Completions](/api/chat-completions)、[Responses](/api/responses)、[Embeddings](/api/embeddings)、[图片](/api/images)。

## Anthropic SDK

::: code-group

```python [Python]
import os
from anthropic import Anthropic

client = Anthropic(
    base_url="https://api.example.com",
    api_key=os.environ["SUB2API_API_KEY"],
)
```

```javascript [Node.js]
import Anthropic from '@anthropic-ai/sdk'

const client = new Anthropic({
  baseURL: 'https://api.example.com',
  apiKey: process.env.SUB2API_API_KEY,
})
```

```bash [环境变量]
export ANTHROPIC_BASE_URL="https://api.example.com"
export ANTHROPIC_API_KEY="sk-your-api-key"
```

:::

可用接口：[Messages](/api/messages)。

## Google GenAI SDK

::: code-group

```python [Python]
import os
from google import genai
from google.genai import types

client = genai.Client(
    api_key=os.environ["SUB2API_API_KEY"],
    http_options=types.HttpOptions(base_url="https://api.example.com"),
)
```

```javascript [Node.js]
import { GoogleGenAI } from '@google/genai'

const ai = new GoogleGenAI({
  apiKey: process.env.SUB2API_API_KEY,
  httpOptions: { baseUrl: 'https://api.example.com' },
})
```

:::

可用接口：[Gemini 原生接口](/api/gemini)。

## 其他 OpenAI 兼容客户端

大多数支持「自定义 OpenAI 地址」的应用和框架都可以接入：API 地址填 `https://api.example.com/v1`，Key 填你的 API Key，模型名填[模型列表](/api/models)返回的 ID。
