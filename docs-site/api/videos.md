# 视频生成

视频生成是异步的：先提交任务拿到任务号，再轮询状态，完成后下载成片。

Grok 和多模态分组可用。其他分组返回 `404 Videos API is not supported for this platform`，除非管理员为所请求的模型单独配置了视频通道。

## 提交任务

<Endpoint method="POST" path="/v1/videos/generations" />

```bash
curl -sS "https://api.example.com/v1/videos/generations" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "your-video-model",
    "prompt": "镜头缓缓靠近海边灯塔",
    "duration": 8,
    "resolution": "720p",
    "aspect_ratio": "16:9"
  }'
```

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 必填。视频模型 ID |
| `prompt` | string | 必填。画面描述 |
| `duration` | integer | 时长，单位秒。可选范围取决于模型 |
| `resolution` | string | 分辨率，例如 `480p`、`720p`、`1080p` |
| `aspect_ratio` | string | 画面比例，例如 `16:9` |
| `images` | array | 可选。图生视频的参考图 URL |

响应中的 `id` 是任务号，请原样保存。余额计费时，回执里的 `cost` 是冻结金额，任务完成后的 `cost` 是实际结算金额，单位见 `currency`。

::: warning 不要重复提交
网络中断后用已保存的任务号继续查询。重新 POST 会创建并计费另一个任务。
:::

## 查询任务

<Endpoint method="GET" path="/v1/videos/{request_id}" />

```bash
curl -sS "https://api.example.com/v1/videos/$TASK_ID" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

| `status` | 含义 |
| --- | --- |
| `queued` | 排队中 |
| `in_progress` | 生成中 |
| `completed` | 已完成，可以下载 |
| `failed` | 失败。上游取消或过期的任务也归为此状态 |

```json
{
  "id": "example-task",
  "status": "completed",
  "progress": 100,
  "seconds": 8,
  "url": "/v1/videos/example-task/content"
}
```

建议每 5 秒查询一次，并设置总超时。超时后保存任务号，之后可以继续查询。查询必须使用提交任务的同一把 Key。

## 下载成片

<Endpoint method="GET" path="/v1/videos/{request_id}/content" />

任务完成后调用。网关核对任务归属后返回视频内容，或以 302 跳转到带签名的下载地址。

```bash
curl -sS -L "https://api.example.com/v1/videos/$TASK_ID/content" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -o video.mp4
```

::: tip 避免把 Key 发给第三方
`curl -L` 在跳转到其他域名时默认不会转发 `Authorization` 请求头。自己实现下载时请保持同样的行为：先取跳转地址，再用不带 Key 的请求下载。
:::

任务未完成时返回 `409 video_not_ready`，此时继续轮询状态。

## 编辑与延长

<Endpoint method="POST" path="/v1/videos/edits" />
<Endpoint method="POST" path="/v1/videos/extensions" />

对已有视频做编辑或延长。两者同样返回任务号，查询和下载方式与生成任务相同。可用的字段取决于上游模型。

## 常见问题

| 现象 | 处理 |
| --- | --- |
| `401` 或 `403` | 检查 Key、分组的媒体生成权限和账户余额 |
| 查询返回 `404` | 核对任务号，并确认使用的是提交任务的那把 Key |
| `503` 提示模型或分辨率缺少价格 | 请管理员为该模型和分辨率档位配置价格 |

更完整的调用脚本见仓库的[生图与视频 API 调用指南](https://github.com/DR-lin-eng/sub2api-no2api/blob/main/docs/MEDIA_API.md)。
