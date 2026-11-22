# 异步图片任务

提交后立即拿到任务号，之后轮询结果。适合生成时间长、容易被 CDN 或代理超时断开的场景。

::: warning 需要管理员启用
此功能默认关闭，依赖站点配置的对象存储，且只支持 OpenAI 和 Grok 分组。未启用时返回 `404 async image tasks are not enabled`。
:::

## 提交任务

<Endpoint method="POST" path="/v1/images/generations/async" />
<Endpoint method="POST" path="/v1/images/edits/async" />

请求体与同步的[生成](/api/images#生成图片)和[编辑](/api/images#编辑图片)接口相同，但不接受 `stream`。

```bash
curl -i "https://api.example.com/v1/images/generations/async" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "your-image-model",
    "prompt": "暴风雪中的灯塔",
    "size": "1536x1024"
  }'
```

受理后返回 `202 Accepted`：

```json
{
  "id": "imgtask_0123456789abcdef",
  "task_id": "imgtask_0123456789abcdef",
  "object": "image.generation.task",
  "status": "processing",
  "created_at": 1784092800,
  "expires_at": 1784179200,
  "poll_url": "/v1/images/tasks/imgtask_0123456789abcdef"
}
```

响应头 `Location` 是轮询路径，`Retry-After: 3` 是建议的轮询间隔。

站点同时处理的异步任务数有上限。超过时返回 `429` 和 `Retry-After`，此时任务没有创建，退避后重新提交即可。

## 查询任务

<Endpoint method="GET" path="/v1/images/tasks/{task_id}" />

必须使用提交任务时的同一把 Key。

```bash
curl -sS "https://api.example.com/v1/images/tasks/imgtask_0123456789abcdef" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

| `status` | 含义 |
| --- | --- |
| `processing` | 仍在生成，稍后再查 |
| `completed` | 已完成，结果在 `result` 中 |
| `failed` | 失败，原因在 `error` 中 |

::: code-group

```json [completed]
{
  "id": "imgtask_0123456789abcdef",
  "task_id": "imgtask_0123456789abcdef",
  "object": "image.generation.task",
  "status": "completed",
  "http_status": 200,
  "image_url": "https://...",
  "result": {
    "created": 1784092923,
    "data": [{"url": "https://..."}]
  },
  "created_at": 1784092800,
  "completed_at": 1784092923,
  "expires_at": 1784179323
}
```

```json [failed]
{
  "id": "imgtask_0123456789abcdef",
  "task_id": "imgtask_0123456789abcdef",
  "object": "image.generation.task",
  "status": "failed",
  "http_status": 502,
  "error": {"type": "api_error", "message": "Upstream request failed"},
  "created_at": 1784092800,
  "completed_at": 1784092923,
  "expires_at": 1784179323
}
```

:::

`result` 与同步接口的响应体一致，但图片已转存到对象存储：`data[].url` 指向转存后的地址，不再返回 `b64_json`。`image_url` 是第一张图的地址。

## 行为说明

- 任务和结果在最后一次状态更新后保留 24 小时。
- 单个任务最多执行 30 分钟。
- 用其他 Key 查询，或任务号不存在，都返回 404。
- 生成用完了余额后，仍然可以查询已提交的任务。

部署和对象存储配置见仓库的[异步图片任务文档](https://github.com/DR-lin-eng/sub2api-no2api/blob/main/docs/ASYNC_IMAGE_TASKS.md)。
