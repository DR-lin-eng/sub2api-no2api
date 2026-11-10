# GIF 标准 API 与 OAuth 测试转接核对（2026-11-10）

## 官方合同

已实际读取 [Images generation reference](https://developers.openai.com/api/reference/resources/images/methods/generate) 与 [图像生成指南](https://developers.openai.com/api/docs/guides/image-generation)。2.6 GIF 使用标准生成/编辑端点与 `output_format=gif`；`frame_count` 为 2–16，省略为 9；帧时长默认 200 ms，reference 要求舍入到 10 ms，并限制在 GIF 字段范围。GIF 不支持流式、部分图片、mask 或自定义输出压缩，尺寸可用 `auto`。

标准生成请求示例：

```json
{"model":"gpt-image-2.6-flare","prompt":"A rotating blue circle, seamless loop.","output_format":"gif","size":"auto","frame_count":4,"frame_duration_ms":200,"n":1,"stream":false}
```

## 确认的实现差异与修复

修复前普通 OAuth 的 GIF 账号测试使用 App conversation 动画意图；同一账号经公共 Images API 收到 GIF 请求则被 `openAIImagesRequiresNativeProtocol` 强制转到 Codex Images。按钮测试成功不代表另一条路径可用。

基础 GIF 请求现与 OAuth 账号测试共用动画指令和实际文件校验，使用同一账号会话、验证与出口，不向 App JSON 添加未经确认的 `output_format`、`frame_count`、`animation` 或 `requestedMimeType` 原生字段。

帧数和时长从标准请求转入动画指令，返回文件需确为 GIF 并满足请求控制；PNG、帧数/时长不符不能作为成功返回。失败不重新生图，已完成作品的用量和实际帧数继续结算。开启实际张数计费时，两份 9 帧 GIF 为 18 个计费单位。

API Key、SIWC OAuth 保留标准 Images 请求体、生成/编辑端点和响应。普通 OAuth 显式尺寸、质量、复杂度等 App 不保证的参数仍在首次出站前选原生路径；显式旧版开关保持原生行为。SVG 路径不受本次 GIF 适配影响。

用户原报错的完整旧字符串已不在当前 main 源码中。本次受控复现使用只开放 App GIF、拒绝 Codex native GIF 的模拟上游，验证两条转接的差异；该 fixture 的 400 不是捕获的真实 OpenAI 错误。

## Docker 验证

基线 `4c1174633`。真实运行两套同版本网关，每套具有独立 PostgreSQL 数据库和 Redis，通过 API Key 对接号池；模拟上游只代替 OpenAI 生成端。复现入口 [runtime/check.py](runtime/check.py)，结果 [runtime/result.json](runtime/result.json)。

四阶段均检查 OAuth 按钮测试、API Key 串接按钮测试，以及通过两层标准 API 请求与计费：

| 阶段 | OAuth GIF 按钮 | API Key 串接 GIF 按钮 | 两层 API GIF | 两侧按帧结算 |
| --- | --- | --- | --- | --- |
| 基线 | 成功 | 原生路径失败 | 400 | 无成功输出 |
| 修复 | 成功 | 成功 | 200 | 9/4 帧按 9/4 张，一次结算 |
| 回退 | 成功 | 原生路径失败 | 400 | 无成功输出 |
| 恢复 | 成功 | 成功 | 200 | 9/4 帧按 9/4 张，一次结算 |

两款 2.6 模型均覆盖，共 24 项检查通过。容器、网络与数据库卷结束后删除。

Go 1.26.10 Docker 的 service 全量单元测试通过。HTTP 全量首次在并行构建负载下有三个 WS 5 秒读取超时，低负载串行重跑全部通过。lint 首次并行运行被内存终止；限制为单线程及 1500 MiB Go 内存后为 0 issues。完整生产镜像、专项 API Key/SIWC 生成和编辑边界、旧版开关、GIF 文件/帧参数/计费测试均通过。

既有布局检查仍被 `payment_fulfillment_test.go` 1201 行阻挡，该文件未修改。

## 用户授权账号联网验证

使用用户提供的普通 OAuth 账号，在两套隔离 Docker 网关中调用真实 OpenAI 上游；结果见 [LIVE_VERIFICATION.json](LIVE_VERIFICATION.json)。

- API Key 账号的串接测试按钮成功，37.98 秒，返回 75,533 字节 GIF。
- 标准 `/v1/images/generations` 请求经两套网关返回 HTTP 200，30.26 秒，`data[0].b64_json` 为 106,757 字节 GIF，`output_format=gif`。
- 两个文件均经 Go `image/gif.DecodeAll` 独立校验，4 帧，每帧 200 ms，循环播放，LZW 图像数据有效。
- 两套网关的用量均按 4 帧各结算一次。记录中的 0.4 为隔离 fixture 配置的网关价格，不是 OpenAI 真实账单。

最初的工作区发现发生连接 EOF，在模型生成前失败；确认官方工作区接口可达后重试成功。联网验收使用隔离环境的标准 TLS 与 HTTP/2，账号导出的自定义 TLS 模板定义不在文件内，因此不将该结果扩展为原部署的自定义 TLS 验收。
凭据只经 stdin 写入临时数据库，没有进入源码、命令参数或验证记录；结束后删除容器、网络及数据库卷。真实生成的文件保存在临时目录供用户核验，不提交到 Git。
