# 概览

Sub2API 是一个 AI API 网关。你在站点控制台创建 API Key 并选择分组，之后用这把 Key 按上游厂商的原生协议发请求，网关负责鉴权、选择上游账号、转发和计费。

## 网关地址

本文档的示例统一使用占位地址 `https://api.example.com`。在左侧栏的「示例中的网关地址」填入你的站点地址后，全站示例会自动替换。站点运营者也可以在分享链接后追加 `?base_url=https://你的域名`，读者打开时即已替换。

不同协议使用的前缀如下：

| 协议 | 前缀 | 典型客户端 |
| --- | --- | --- |
| OpenAI 兼容 | `https://api.example.com/v1` | OpenAI SDK、Codex CLI |
| Anthropic 兼容 | `https://api.example.com`（SDK 自动追加 `/v1/messages`） | Anthropic SDK、Claude Code |
| Gemini 原生 | `https://api.example.com/v1beta` | Google GenAI SDK、Gemini CLI |
| Antigravity 专用 | `https://api.example.com/antigravity` | 只调度 Antigravity 账号的 Claude / Gemini 客户端 |

`/v1` 下的多数端点还提供不带前缀的别名，例如 `/chat/completions`、`/responses`、`/models`，方便只允许填写根地址的客户端。

## 分组决定可用端点

每把 Key 绑定一个分组，分组的平台类型决定这把 Key 能调用哪些端点和模型。

| 分组平台 | 文本端点 | 媒体端点 |
| --- | --- | --- |
| Anthropic、Antigravity | Messages、Chat Completions、Responses | 无 |
| OpenAI | Messages、Chat Completions、Responses、Embeddings | 图片、异步图片（需管理员启用） |
| Grok | Messages、Chat Completions、Responses | 图片、异步图片（需管理员启用）、视频 |
| Kimi、智谱、DeepSeek、MiniMax、OpenCode Go | Messages、Chat Completions、Responses | 无 |
| Gemini | Gemini 原生接口、Messages、Chat Completions、Responses | 批量图片（需管理员启用） |
| 多模态 | 无（返回 404） | 图片、视频 |
| 复合分组 | 按请求的模型名路由到上面的具体平台 | 同目标平台 |

不确定自己的 Key 属于哪种分组时，调用 [`GET /v1/models`](/api/models) 查看可用模型，或调用 [`GET /v1/api-key-groups`](/api/usage#查询可用分组) 查看分组的平台类型。

## 下一步

- [快速开始](/guide/quickstart)：发出第一个请求。
- [认证](/guide/authentication)：三种传 Key 的方式和常见鉴权错误。
- [端点总览](/api/)：所有可用路径。
