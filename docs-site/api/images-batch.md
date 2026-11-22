# 批量图片

一次提交多条提示词，由 Gemini 的批处理通道异步生成，完成后逐张或打包下载。适合对时效不敏感的大批量出图。

::: warning 需要管理员启用
只支持 Gemini 分组，且管理员需要开启站点的批量图片功能，并在分组上允许批量生图。
:::

## 提交任务

<Endpoint method="POST" path="/v1/images/batches" />

```bash
curl -sS "https://api.example.com/v1/images/batches" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "provider": "gemini_api",
    "image_size": "1K",
    "response_mime_type": "image/png",
    "items": [
      {"custom_id": "cover_001", "prompt": "一张简洁的产品主视觉图", "output_count": 1}
    ]
  }'
```

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 必填。可用模型见 `GET /v1/images/batches/models` |
| `provider` | string | `gemini_api` 或 `vertex` |
| `items` | array | 必填。每项是一条生成请求 |
| `items[].custom_id` | string | 你自定义的条目标识，用于取回结果 |
| `items[].prompt` | string | 提示词 |
| `items[].output_count` | integer | 这条提示词重复生成的张数，默认 1，最多 4 |
| `items[].reference_images` | array | 可选。参考图，支持 PNG、JPEG、WebP |
| `image_size` | string | 输出尺寸档位，例如 `1K` |
| `response_mime_type` | string | 输出格式，例如 `image/png` |

单个任务最多产出 200 张图片，超过请拆成多个任务。

### 响应

```json
{
  "id": "imgbatch_0123456789abcdef0123456789abcdef",
  "object": "image.batch",
  "status": "queued",
  "model": "gemini-2.5-flash-image",
  "provider": "gemini_api",
  "item_count": 1,
  "success_count": 0,
  "fail_count": 0,
  "estimated_cost": 0.25,
  "actual_cost": null,
  "created_at": 1783123200,
  "submitted_at": 1783123201,
  "settled_at": null
}
```

## 任务状态

| `status` | 含义 |
| --- | --- |
| `queued` | 已受理，等待提交到上游 |
| `running` | 上游处理中 |
| `processing_results` | 正在整理结果 |
| `settling` | 正在结算 |
| `completed` | 已完成，可以下载 |
| `failed` | 失败 |
| `cancelled` | 已取消 |
| `output_deleted` | 输出已被手动删除或过期清理 |

只有成功生成的图片会计费，失败的条目不计费。

## 查询与下载

```bash
# 任务状态
curl -sS "https://api.example.com/v1/images/batches/$BATCH_ID" \
  -H "Authorization: Bearer $SUB2API_API_KEY"

# 条目列表
curl -sS "https://api.example.com/v1/images/batches/$BATCH_ID/items" \
  -H "Authorization: Bearer $SUB2API_API_KEY"

# 下载单张
curl -sS "https://api.example.com/v1/images/batches/$BATCH_ID/items/cover_001/content" \
  -H "Authorization: Bearer $SUB2API_API_KEY" -o cover_001.png

# 打包下载
curl -sS "https://api.example.com/v1/images/batches/$BATCH_ID/download" \
  -H "Authorization: Bearer $SUB2API_API_KEY" -o batch.zip
```

条目列表的响应：

```json
{
  "object": "list",
  "data": [
    {
      "custom_id": "cover_001",
      "status": "succeeded",
      "mime_type": "image/png",
      "file_extension": "png",
      "image_count": 1,
      "error": null
    }
  ],
  "has_more": false
}
```

输出在任务结束后默认保留 72 小时。清理后再下载返回 `410 Gone`，错误码为 `BATCH_IMAGE_OUTPUT_DELETED`。

## 全部路径

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/v1/images/batches` | 提交任务 |
| GET | `/v1/images/batches` | 列出自己的任务 |
| GET | `/v1/images/batches/models` | 列出可用的批量图片模型 |
| GET | `/v1/images/batches/{id}` | 查询任务 |
| GET | `/v1/images/batches/{id}/items` | 列出条目 |
| GET | `/v1/images/batches/{id}/items/{custom_id}/content` | 下载单张图片 |
| GET | `/v1/images/batches/{id}/download` | 打包下载 |
| POST | `/v1/images/batches/{id}/cancel` | 取消任务 |
| DELETE | `/v1/images/batches/{id}` | 删除任务记录 |
| DELETE | `/v1/images/batches/{id}/outputs` | 删除输出文件 |

参考图限制、保留时间和部署配置见仓库的[批量图片文档](https://github.com/DR-lin-eng/sub2api-no2api/blob/main/docs/BATCH_IMAGE_MVP.md)。
