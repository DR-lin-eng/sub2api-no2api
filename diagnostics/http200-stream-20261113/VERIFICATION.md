# HTTP 200 响应中的失败处理

日期：2026-11-13。基线为 `1d3e1ea2b` 加本次聊天中已经实现的 [zstd 兼容恢复](../issue-8636-fix-20261113/README.md)。本次继续保留此前的修改。

## 触发路径

HTTP 200 的 Responses 请求可能继续返回模型失败。原实现已经识别嵌套错误和 `response.failed`，但以下情况处理不完整：

- 原生扁平事件 `{"type":"error","code":"server_error","message":"Upstream request failed"}` 未被通用失败分类完整识别，WS 错误字段解析也遗漏事件根部字段。
- `event: error` 或没有 JSON `type` 的 `data: {"error":...}` 被当作未知事件，可能提前成为不可重放的输出。
- 上游以 `application/json` 返回 200 错误时，流式扫描器没有将其送入 SSE 错误分类。
- 可安全重放的明确通用失败缺少一次独立的同账号恢复，单账号部署容易直接失败。
- 已输出内容后的 EOF/网络读取失败可能只留下一段不完整的流，或追加通用 `type:error`，导致客户端报告未完成；缺少空行的最后一个 SSE data 行也需要先完成帧边界再发送终止事件。

这些是源码和受控 Docker 回放确认的路径，不能据此认定用户此前每次报错都来自同一原因。

## 实现

[错误事件规范化](../../backend/internal/application/service/openai_responses_error_event.go)对已识别的通用错误采用 `response.failed` 信封；具体参数、上下文和安全错误继续走既有规则。[通用错误分类](../../backend/internal/application/service/openai_upstream_client_error.go)及 WS 字段解析补充原生顶层 code/message，避免把普通响应正文中的同样文字误认为错误。

[HTTP 200 JSON 适配](../../backend/internal/application/service/openai_http200_json_error.go)只把完整、有界、没有语义输出的通用 JSON 错误转为 SSE 数据事件。读取在原扫描器内部惰性执行，继续受既有超时和心跳约束；较大或非目标正文回放已读前缀并保留原 reader，不截断、不吞掉读取错误。

[同账号恢复](../../backend/internal/application/service/openai_http200_stream_recovery.go)只对明确通用失败、没有语义输出且没有已报告用量的请求执行一次 250 ms 延迟后的重试。原始请求和最终请求均验证安全重放；存储/后台响应、上游工具、增量 continuation、取消、已提交输出、被隔离账号及蒸馏组不进入此恢复。请求内标记跨换号共享，自动透传继续消耗已有发送预算。第二次仍失败时进入原有换号/耗尽处理。

已输出内容后不进入恢复；已发送或仍在缓冲中的正文先完整保留，然后追加一个符合 Responses schema 的失败终止事件。正常 EOF 缺少终态使用 `upstream_stream_truncated`，读取错误使用 `upstream_stream_read_error`，HTTP/2 reset 使用既有 `upstream_http2_stream_error` 分类。不会将截断伪装为 `response.completed`，也不会重复已经送达的模型输出。

zstd 默认继续开启。若本请求已经因 zstd 被拒绝而协商到 identity，同账号后续认证/流恢复沿用该编码；独立的新请求重新以 zstd 首发。这样 `zstd 400 → identity 200 失败 → identity 成功` 可以完成，账号设置和其它账号的编码策略不变。

普通 HTTP Forward 将成功后的 continuation 绑定和 quota snapshot 收到[响应完成 owner](../../backend/internal/application/service/openai_gateway_forward_response.go)，避免继续扩大编排文件；保持只有成功结果写入这些状态。

## 验证记录

原始回放确认 20 个恢复场景和 4 个输出后错误终止场景失败；修复后专项测试覆盖流式/非流式、普通/透传、顶层/嵌套/named/bare/JSON 信封、单次上限、已有用量、存储/后台/工具、取消及编码组合。逐项数值与原始日志摘要见 [CHECKS.json](CHECKS.json)，运行结果见 [RESULTS.json](RESULTS.json)。

| 检查 | 实际结果 |
| --- | --- |
| 专项测试 | 151 个测试及子项通过；覆盖 HTTP 200 恢复、输出后终态、取消/存储/工具/用量保护、zstd 组合及残留 flush |
| 扩大回归 | 12 个包、18,039 个测试及子项通过，0 失败；4 项按既有条件跳过 |
| Docker HTTP 矩阵 | 123 个场景符合预期：97 次成功、26 次预期失败；下游状态为 105 次 200、16 次 400、2 次 502 |
| HTTP 200 错误恢复 | 两个模型、流式/非流式、普通/透传及六种错误信封均恢复；重试保持完整输入和账号/会话身份摘要一致 |
| 输出后的失败 | 普通/透传各验证通用错误、正常 EOF 截断和网络重置，单次上游尝试，保留部分正文并返回分类的单个 response.failed |
| 200 已提交后的耗尽 | 先发送 codex.rate_limits 控制帧再连续失败；两个路径均最多重试一次，HTTP 200 内有 response.failed 且不含内部通用哨兵文案 |
| 未提交下游的耗尽 | 两条路径保持正确的 HTTP 502，文案为 Upstream service temporarily unavailable |
| 压缩与组合 | 新请求仍以 zstd 首发；四个组合场景均为 zstd 400 → identity 200 失败 → identity 成功；最终两个账号仍开启压缩 |
| 并发 | 4 个 worker、40 次请求全部成功；上游 80 次发送均使用 zstd，每条请求恰好重试一次、正文/身份一致 |
| PostgreSQL 结算 | 137 次成功请求对应 137 条用量记录，逐 Key 无重复；26 次失败没有用量记录，不估算虚构用量 |
| 最终静态检查 | golangci-lint run --concurrency=1 --timeout=15m ./... 通过，输出 0 issues |
| 格式与结构 | make check-layout、make check-docs、git diff --check、gofmt 与诊断脚本语法检查通过 |

扩大回归在 Go 1.26.11 Linux ARM64 容器中执行：

```sh
go test -json -tags=unit -p 1 -count=1 -timeout=15m \
  ./internal/application/service/... \
  ./internal/transport/http/... \
  ./internal/shared/httputil/...
```

4 个跳过项为旧 session reference adoption、5 万并发压力、真实 OpenAI token 估算对比及 DingTalk 禁用场景。首次静态检查发现 SA9004 常量类型标注问题，修正仅补充显式 string 类型，不改变运行逻辑；修正后另行重编译、重跑专项与静态检查，具体记录保存在 CHECKS 中。

常量类型标注修正前后的后端二进制 SHA-256 相同：`ec5d4f86b1c0715a458ce237ef95147e5d11fe22df5dfd6092223116bd4eb480`，因此 Docker 123 场景验证的二进制也是最终构建产物。8 个以 HTTP 200 返回失败的场景均逐项核对只有一个 `response.failed` 事件。

恢复成功后仍按已有 Ops 规则保存上游失败尝试，方便追踪重试；不能仅凭该历史尝试记录的 HTTP 200 判断最终客户端失败，应检查最终 `response.completed` / `response.failed` 和具体错误分类。

扩大回归曾发现发送失败终止事件前遗漏缓冲区残留正文；本次修复增加了先 flush 原文、补齐 SSE 空行再终止的步骤，并复用原测试验证。两项原有 flush 测试更新为检查原文前缀、单个失败终态和最终完整 flush；正常终态后的网络错误仍保持原来的成功结果。

## 部署和限制

无新配置、数据库迁移或前端变更，可部署后端二进制/镜像；回退代码即可恢复原行为。没有修改真实账号或部署到生产。

恢复仅适用于可安全重放的明确通用失败。已经输出内容的真实网络断流仍会以失败结束，并携带明确终态和分类；这不能保证上游网络永不中断。Docker 使用合成上游和临时 PostgreSQL/Redis，不能替代实际 OpenAI 账号的验证。
