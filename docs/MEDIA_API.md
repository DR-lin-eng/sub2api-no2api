# 生图与视频 API 调用指南

本文使用 Sub2API 的多模态分组和火山方舟上游。客户端只需网关地址、网关 API Key 和分组公开的模型名。图片结果可以返回 URL 或 Base64，视频生成先返回任务号，再按任务号查询和下载。

**准备环境。** 示例使用 Bash、curl 7.82+ 和 jq。将下列值替换为你的站点、Key 和公开模型名。网关地址写到站点根路径，后面的命令已包含 `/v1`。管理员需为分组开通媒体生成权限、绑定账号并配置对应模型的价格。[账号与价格配置说明](CUSTOM_MODELS_AND_MEDIA.md#多模态账号与火山方舟)

```bash
export SUB2API_BASE_URL='https://gateway.example.com'
export SUB2API_API_KEY='your-gateway-api-key'
export MEDIA_IMAGE_MODEL='my-image'
export MEDIA_VIDEO_MODEL='my-video'

curl -fsS -H "Authorization: Bearer $SUB2API_API_KEY" \
  "$SUB2API_BASE_URL/v1/models" | jq -er '.data[].id'
```

`my-image` 和 `my-video` 是示例别名。从返回的模型中选择分组已启用的图片模型和视频模型。下文所有请求都使用同一把 Key。

**生成图片。** 调用 `POST /v1/images/generations`，从 `data[0].url` 读取图片地址。方舟通道单次请求支持 `n=1`，`size` 需符合目标模型规格。示例使用 2048 × 2048。

```bash
jq -n --arg model "$MEDIA_IMAGE_MODEL" \
  '{model:$model,prompt:"清晨的海边灯塔，水彩风格",size:"2048x2048",n:1,response_format:"url"}' |
  curl --fail-with-body -sS -H "Authorization: Bearer $SUB2API_API_KEY" \
    --json @- "$SUB2API_BASE_URL/v1/images/generations" |
  jq -er 'if .error then error(.error.message) else .data[0].url end'
```

**用参考图编辑。** 将本地 PNG 保存为 `reference.png`，通过 `POST /v1/images/edits` 发送 multipart 表单。让 curl 自动生成表单边界。也可以发 JSON，把参考图 URL 或 `data:image/...;base64,...` 放在 `image` 字段。方舟编辑至少需要一张参考图，遮罩编辑需要支持 `mask` 的其他通道。

```bash
curl --fail-with-body -sS -H "Authorization: Bearer $SUB2API_API_KEY" \
  --form-string "model=$MEDIA_IMAGE_MODEL" \
  --form-string 'prompt=保留灯塔构图，把天空改为日落' \
  --form-string 'size=2048x2048' --form-string 'response_format=b64_json' \
  -F 'image=@reference.png;type=image/png' "$SUB2API_BASE_URL/v1/images/edits" \
  -o edited.json &&
python3 - <<'PY'
import base64, json
from pathlib import Path

result = json.loads(Path("edited.json").read_bytes())
if result.get("error"):
    raise SystemExit(result["error"]["message"])
image = base64.b64decode(result["data"][0]["b64_json"], validate=True)
if not image:
    raise SystemExit("图片内容为空")
Path("edited-image").write_bytes(image)
PY
```

该示例用 Python 3 将 Base64 还原为文件 `edited-image`。输出图片的实际格式由模型决定。

**接收流式响应。** 在生成请求中加 `"stream":true`，用 `curl -N` 关闭客户端输出缓冲。下例显示方舟图片的 SSE 数据。

```bash
jq -n --arg model "$MEDIA_IMAGE_MODEL" \
  '{model:$model,prompt:"海边灯塔",size:"2048x2048",response_format:"url",stream:true}' |
  curl --fail-with-body -sS -N -H "Authorization: Bearer $SUB2API_API_KEY" \
    --json @- "$SUB2API_BASE_URL/v1/images/generations"
```

```text
data: {}

data: {"created":1700000000,"data":[{"url":"https://cdn.example.com/image.png"}]}

data: [DONE]

```

SSE 客户端以空行分隔事件，忽略 `data: {}` 心跳，从结果事件取图，遇到 `[DONE]` 结束。兼容中转也可能逐帧返回上游图片事件。方舟在完整结果及用量到齐后发送图片。非流式默认每 15 秒刷新一个换行，读取完整响应后按一个 JSON 对象解析。流式默认立即发送首拍，随后每 10 秒保活。**首拍会提交 HTTP 200**，所以还要检查 JSON 的 `error`，或 SSE 的 `upstream_error`、`proxy_error`、`error` 事件及其数据。连接在收到结果前中断时，结果仍未确认。

**提交视频。** 使用 `POST /v1/videos/generations`。下例生成 8 秒、720p 的视频，并保存网关返回的任务号。图生视频可在同一个 JSON 对象中加入 `"images":["https://你的素材域名/first-frame.jpg"]`，方舟通道支持一张首帧图。若设置 `stream:true`，从 SSE 数据事件读取同一份任务回执；此处的 `[DONE]` 表示提交响应结束，成片状态仍需查询。

```bash
MEDIA_TASK_ID=$(
  jq -n --arg model "$MEDIA_VIDEO_MODEL" \
    '{model:$model,prompt:"镜头缓缓靠近海边灯塔",duration:8,resolution:"720p",aspect_ratio:"16:9"}' |
    curl --fail-with-body -sS -H "Authorization: Bearer $SUB2API_API_KEY" \
      --json @- "$SUB2API_BASE_URL/v1/videos/generations" |
    jq -er 'if .error then error(.error.message) else .id end'
)
```

将 `MEDIA_TASK_ID` 原样保存。方舟通道接受 1–15 秒的整数时长和 `480p`、`720p`、`1080p` 分辨率，可用组合取决于模型。按次计费的分组需要显式填写已定价的分辨率。余额模式下，任务回执中的 `cost` 表示冻结金额，完成后的 `cost` 表示实际结算金额，单位见 `currency`。

**查询任务。** 按下面的命令查询一次。状态为 `queued` 或 `in_progress` 时，稍后用相同的任务号和 Key 再查。收到 `completed` 后即可下载；`failed` 是终态，可从返回体读取错误详情。方舟取消或过期的任务也映射为 `failed`。网络中断后可用已保存的任务号续查，重新 POST 会创建另一份任务。

```bash
curl --fail-with-body -sS -H "Authorization: Bearer $SUB2API_API_KEY" \
  "$SUB2API_BASE_URL/v1/videos/$MEDIA_TASK_ID" |
  jq '{id,status,progress,seconds,url,cost,currency,error}'
```

完成后的典型字段如下，`url` 是网关内的相对路径。程序可以给轮询设置 5 秒间隔和总超时，超时后保存任务号供下次恢复。

```json
{"id":"example-task","status":"completed","progress":100,"seconds":8,"url":"/v1/videos/example-task/content"}
```

**下载成片。** 方舟的 `/content` 在核对任务归属、完成结算后返回 302。先向网关取签名地址，再用独立请求下载。这样站点 Key 只发给网关，签名地址本身足以读取文件。

```bash
MEDIA_DOWNLOAD_URL=$(curl -fsS -o /dev/null -w '%{redirect_url}' \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  "$SUB2API_BASE_URL/v1/videos/$MEDIA_TASK_ID/content") &&
  test -n "$MEDIA_DOWNLOAD_URL" &&
  curl -fsS --proto '=https' "$MEDIA_DOWNLOAD_URL" -o video.mp4
```

| 调用时遇到的情况 | 客户端处理 |
| --- | --- |
| `401` 或 `403` | 检查网关 Key、分组、媒体生成权限及当前计费资格。 |
| 字段错误，如 `mask`、`n` 或 `duration` | 按 `error.message` 修正请求。错误发生在心跳后时也要读取响应体。 |
| 查询得到 `404` 或取片得到 `409 video_not_ready` | 核对原任务号及原 Key。409 时先查状态，仅对排队中或生成中的任务继续轮询。 |
| `503` 提示模型或分辨率缺价 | 请管理员为公开模型与所选档位配置价格后重试。 |
| 需要先返回任务号、之后再取图片 | 独立的异步图片端点有分组和对象存储要求，支持范围见[异步图片任务](ASYNC_IMAGE_TASKS.md)。 |
