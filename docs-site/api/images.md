# 图片生成与编辑

OpenAI Images 兼容接口。OpenAI、Grok 和多模态分组可用，其他分组返回 `404 Images API is not supported for this platform`。

可用的图片模型、尺寸和单次张数由分组和上游模型决定。先用[模型列表](/api/models)确认模型 ID。

## 生成图片

<Endpoint method="POST" path="/v1/images/generations" />

::: code-group

```bash [curl]
curl -sS "https://api.example.com/v1/images/generations" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "your-image-model",
    "prompt": "清晨的海边灯塔，水彩风格",
    "size": "1024x1024",
    "n": 1,
    "response_format": "url"
  }'
```

```python [Python]
import os
from openai import OpenAI

client = OpenAI(
    base_url="https://api.example.com/v1",
    api_key=os.environ["SUB2API_API_KEY"],
)

result = client.images.generate(
    model="your-image-model",
    prompt="清晨的海边灯塔，水彩风格",
    size="1024x1024",
)
print(result.data[0].url or result.data[0].b64_json[:32])
```

:::

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | 必填。图片模型 ID |
| `prompt` | string | 必填。画面描述 |
| `size` | string | 尺寸，例如 `1024x1024`。可选值取决于模型 |
| `n` | integer | 生成张数。部分上游只支持 1 |
| `response_format` | string | `url` 或 `b64_json` |
| `stream` | boolean | 为 `true` 时以 SSE 返回 |

### 响应

```json
{
  "created": 1784092800,
  "data": [{"url": "https://cdn.example.com/image.png"}]
}
```

`response_format` 为 `b64_json` 时，`data[].b64_json` 是 Base64 编码的图片内容。返回的图片 URL 有时效，请及时下载保存。

## 编辑图片

<Endpoint method="POST" path="/v1/images/edits" />

用参考图生成新图。可以用 multipart 表单上传文件，也可以发 JSON 并在 `image` 字段放图片 URL 或 `data:image/...;base64,...`。

```bash
curl -sS "https://api.example.com/v1/images/edits" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  --form-string "model=your-image-model" \
  --form-string "prompt=保留灯塔构图，把天空改为日落" \
  --form-string "size=1024x1024" \
  -F "image=@reference.png;type=image/png"
```

响应结构与生成接口相同。是否支持遮罩（`mask`）和多张参考图取决于上游通道。

## 流式响应

在请求体中加 `"stream": true`，并用 `curl -N` 读取：

```text
data: {}

data: {"created":1784092800,"data":[{"url":"https://cdn.example.com/image.png"}]}

data: [DONE]
```

- `data: {}` 是保活心跳，忽略即可。
- 从带有 `data` 数组的事件取图，收到 `[DONE]` 结束。
- 心跳发出时 HTTP 状态已是 200，之后的失败会以 `error`、`upstream_error` 或 `proxy_error` 事件到达，需要一并处理。

## 耗时较长时

生图可能持续数十秒到数分钟。部分通道在非流式请求期间会定期发送空白字符保活，客户端读完整个响应体后再按 JSON 解析即可。

如果你的网络路径上有 CDN 或代理会在长时间无数据时断开连接，请改用[异步图片任务](/api/images-async)。

更完整的调用脚本见仓库的[生图与视频 API 调用指南](https://github.com/DR-lin-eng/sub2api-no2api/blob/main/docs/MEDIA_API.md)。
