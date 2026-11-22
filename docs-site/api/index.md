# 端点总览

所有端点都需要[认证](/guide/authentication)。一把 Key 能调用哪些端点取决于它绑定的[分组](/guide/overview#分组决定可用端点)。

## 文本

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/v1/chat/completions` | [Chat Completions](/api/chat-completions) |
| POST | `/v1/responses` | [Responses](/api/responses) |
| POST | `/v1/messages` | [Messages（Anthropic）](/api/messages) |
| POST | `/v1/messages/count_tokens` | [计算 Messages 请求的 token 数](/api/messages#计算-token-数) |
| POST | `/v1beta/models/{model}:generateContent` | [Gemini 原生接口](/api/gemini) |
| POST | `/v1beta/models/{model}:streamGenerateContent` | [Gemini 原生流式接口](/api/gemini#流式生成) |
| POST | `/v1beta/models/{model}:countTokens` | [Gemini 原生 token 计数](/api/gemini#计算-token-数) |
| POST | `/v1/embeddings` | [Embeddings](/api/embeddings) |

## 模型

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/v1/models` | [列出可用模型](/api/models) |
| GET | `/v1/models/{model}` | [查询单个模型](/api/models#查询单个模型) |
| GET | `/v1beta/models` | [Gemini 格式的模型列表](/api/models#gemini-格式) |
| GET | `/v1beta/models/{model}` | [Gemini 格式的单个模型](/api/models#gemini-格式) |

## 图片

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/v1/images/generations` | [生成图片](/api/images#生成图片) |
| POST | `/v1/images/edits` | [编辑图片](/api/images#编辑图片) |
| POST | `/v1/images/generations/async` | [提交异步生成任务](/api/images-async) |
| POST | `/v1/images/edits/async` | [提交异步编辑任务](/api/images-async) |
| GET | `/v1/images/tasks/{task_id}` | [查询异步任务](/api/images-async#查询任务) |
| POST | `/v1/images/batches` | [提交批量图片任务](/api/images-batch) |
| GET | `/v1/images/batches/{id}` | [查询批量任务](/api/images-batch#查询与下载) |

批量图片的其余路径见[批量图片](/api/images-batch#全部路径)。

## 视频

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/v1/videos/generations` | [提交视频生成任务](/api/videos#提交任务) |
| POST | `/v1/videos/edits` | [提交视频编辑任务](/api/videos#编辑与延长) |
| POST | `/v1/videos/extensions` | [提交视频延长任务](/api/videos#编辑与延长) |
| GET | `/v1/videos/{request_id}` | [查询视频任务](/api/videos#查询任务) |
| GET | `/v1/videos/{request_id}/content` | [下载成片](/api/videos#下载成片) |

## Key 与用量

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/v1/usage` | [查询 Key 的额度和用量](/api/usage) |
| GET | `/v1/sub2api/billing` | [查询当前计费倍率](/api/usage#查询当前计费倍率) |
| GET | `/v1/api-key-groups` | [查询可用分组](/api/usage#查询可用分组) |

## 路径别名

下列端点同时提供不带 `/v1` 的别名，行为完全相同：

`/chat/completions`、`/responses`、`/messages/count_tokens`、`/embeddings`、`/models`、`/images/generations`、`/images/edits`、`/images/generations/async`、`/images/edits/async`、`/images/tasks/{task_id}`、`/videos/...`

Antigravity 分组另有只调度 Antigravity 账号的专用前缀：`/antigravity/v1/messages`、`/antigravity/v1/models` 和 `/antigravity/v1beta/models/...`。

::: tip 路由以源码为准
完整路由定义在仓库的 [`gateway.go`](https://github.com/DR-lin-eng/sub2api-no2api/blob/main/backend/internal/transport/http/server/routes/gateway.go)。
:::
