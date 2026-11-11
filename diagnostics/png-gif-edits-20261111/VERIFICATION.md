# PNG 图生 GIF 修复（2026-11-11）

## 复现与原因

用户报告普通 OAuth `gpt-image-2.6-sunburst` 的标准 `/v1/images/edits` 返回 `GIF output is not enabled for this request.`。

使用本会话已授权的 OAuth 测试账号，通过两套隔离网关发送真实 PNG 编辑请求，修复前确认：

- 带默认 `complexity=high` 时，选择原生 Codex Images 编辑端点，真实上游返回相同的 400。
- 去掉复杂度后，选择 App GIF 编辑链路，但参考图 PUT 上传被共享传输层按 GET/HEAD 下载规则拒绝。
- 启用有界 PUT 后继续核对实际上传，确认 direct Azure 还需要默认 `x-ms-blob-type: BlockBlob`、`x-ms-version: 2020-04-08` 和图片 MIME 头；上游 `/files` 不一定显式返回这些值。

未访问用户生产实例的请求日志，因此不把测试账号的复现等同于读取了 b9 的原请求体。

## 最终行为

普通 OAuth 的 GIF 请求显式携带默认复杂度、质量、审核级别或自动尺寸时，保持 App GIF 通道；复杂度保留在动画指令。PNG 通过原上传/处理/引用链路附加到同一请求，再提交动画生成。

存储上传使用专用的有界 HTTPS PUT 标记，继续检查所有 DNS 结果、固定目标 IP、验证原域名 TLS、禁用重定向。存储请求只发送 MIME 与上游存储头，不携带账号认证、Cookie、工作区或设备证明。Azure 缺省头可被上游返回的同名存储头覆盖。普通下载仍只允许 GET/HEAD，账号代理和 IPv6 出口保留。

API Key/SIWC 保持原生标准 API 体与端点；非默认质量、复杂度、自定义尺寸等原生控制继续保留原生路径，不在 App 适配中丢弃。SVG 与显式旧版开关保持既有行为。GIF 已完成结果继续按真实帧数可靠结算，失败不重新生图。

## 文档与证据

已读取官方 [Images edit reference](https://developers.openai.com/api/reference/resources/images/methods/edit)：标准 JSON `images[].image_url`、multipart `image[]`、`output_format=gif` 与帧参数均适用；GIF 需要对应模型的预览资格，不支持流式、mask、部分图片或自定义压缩。

Azure 上传头对照用户提供的 20261108 逆向报告及 `uLW7` 上传器切片；报告仅作协议证据使用。

## Docker 验证

基线源码为 `b9e115633`；运行态复用 PR #203 的镜像 `sub2api:gif-contract-fixed-20261110`，本次涉及的生产文件与基线一致（版本文件除外）。修复镜像为 `sub2api:png-gif-edits-fixed-20261111`。

[runtime/check.py](runtime/check.py) 启动两套真实网关、独立数据库/Redis、模型上游 fixture 和 HTTPS 存储。存储位于无外部路由的内部 Docker 子网，使用临时测试 CA，IP 只属于隔离 fixture。校验实际 PUT 的 PNG 字节哈希、Azure 请求头、无账号凭据和双侧四帧结算。

| 阶段 | 不带默认参数 | 显式默认参数 | 覆盖 |
| --- | --- | --- | --- |
| 基线 | 上传失败 502 | 同文案 400 | 两款模型 × JSON/multipart |
| 修复 | 200 / GIF | 200 / GIF | 两款模型 × JSON/multipart |
| 回退 | 上传失败 502 | 同文案 400 | 两款模型 × JSON/multipart |
| 恢复 | 200 / GIF | 200 / GIF | 两款模型 × JSON/multipart |

32 项运行态场景通过，逐项结果见 [runtime/result.json](runtime/result.json)。各成功请求在每套网关仅结算一次、按 4 帧计数。临时容器、网络、数据库和测试私钥均在结束后清理。

Go 1.26.10 Docker 的 service、repository、HTTP 全量 unit 回归通过；最终上传头、JSON/multipart、默认参数、非默认控制与公开目的地边界专项回归通过。完整生产镜像构建通过。
冻结补丁的 golangci-lint 为 0 issues，`make check-docs` 与补丁格式检查通过。既有布局文件 `payment_fulfillment_test.go` 在基线已为 1201 行，本次未修改。

## 真实账号验收

使用用户在本会话提供并授权联网测试的普通 OAuth 账号，两套隔离网关通过标准 `/v1/images/edits` 发起 `gpt-image-2.6-sunburst` 请求。输入为本目录可复现的蓝色圆形 PNG，`output_format=gif`、4 帧、每帧 200 ms、`size=auto`，显式默认复杂度/质量/审核级别。

- JSON data URL：HTTP 200，59.39 秒，101,618 字节 GIF。
- multipart `image[]`：HTTP 200，77.23 秒，109,504 字节 GIF。
- 两份文件经 Go `image/gif.DecodeAll` 独立解码，均为 4 帧、每帧 200 ms、循环播放，LZW 图像数据有效。
- 两套网关均按 4 帧各可靠结算一次；0.4 为测试配置的网关价格，不代表 OpenAI 实际账单。

详见 [LIVE_VERIFICATION.json](LIVE_VERIFICATION.json)。真实产物只留在本地临时目录，未提交到 Git。

本机代理的 fake-IP DNS 会触发存储公开地址校验；联网验收使用隔离测试容器内的临时 HTTPS DNS 解析，未放宽代码校验或修改用户代理。测试采用标准 TLS，不含导出账号所引用的外部自定义模板。凭据仅从原文件读入并经 stdin 写入临时数据库；完成后清理容器、数据库、网络和 DNS fixture。
