# 快速开始

本页用三步发出第一个请求。

## 1. 创建 API Key

登录你的 Sub2API 站点，进入「API 密钥」页面创建一把 Key，并为它选择分组。完整的 Key 请妥善保存，不要写进前端代码或提交到 Git。

## 2. 设置环境变量

::: code-group

```bash [macOS / Linux]
export SUB2API_BASE_URL="https://api.example.com"
export SUB2API_API_KEY="sk-your-api-key"
```

```powershell [Windows PowerShell]
$env:SUB2API_BASE_URL="https://api.example.com"
$env:SUB2API_API_KEY="sk-your-api-key"
```

:::

## 3. 发送请求

先列出这把 Key 可用的模型，再选一个模型名填入请求。

```bash
curl -sS "$SUB2API_BASE_URL/v1/models" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

下面的示例把 `your-model` 换成上一步返回的模型 ID。

::: code-group

```bash [curl]
curl -sS "$SUB2API_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "your-model",
    "messages": [{"role": "user", "content": "用一句话介绍你自己"}]
  }'
```

```python [Python]
import os
from openai import OpenAI

client = OpenAI(
    base_url=os.environ["SUB2API_BASE_URL"] + "/v1",
    api_key=os.environ["SUB2API_API_KEY"],
)

completion = client.chat.completions.create(
    model="your-model",
    messages=[{"role": "user", "content": "用一句话介绍你自己"}],
)
print(completion.choices[0].message.content)
```

```javascript [Node.js]
import OpenAI from 'openai'

const client = new OpenAI({
  baseURL: `${process.env.SUB2API_BASE_URL}/v1`,
  apiKey: process.env.SUB2API_API_KEY,
})

const completion = await client.chat.completions.create({
  model: 'your-model',
  messages: [{ role: 'user', content: '用一句话介绍你自己' }],
})
console.log(completion.choices[0].message.content)
```

:::

收到包含 `choices` 的 JSON 即表示调用成功。若返回错误，请对照[错误处理](/guide/errors)。

## 接下来

- 想用 Claude Code、Codex CLI 或 Gemini CLI：见[客户端接入](/clients/claude-code)。
- 想用 Anthropic 或 Gemini 的官方 SDK：见[官方 SDK](/clients/sdks)。
- 想生成图片或视频：见[图片生成与编辑](/api/images)和[视频生成](/api/videos)。
