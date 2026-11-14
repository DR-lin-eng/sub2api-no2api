# Issue #8636：Codex HTTP 400 与请求压缩

检查日期：2026-11-12。源码基线：`1d3e1ea2b`。本记录仅覆盖本地 Docker 测试，不代表生产部署或真实 OpenAI 账号验证。

## 讨论中已知的信息

[Issue #8636](https://github.com/Wei-Shaw/sub2api/issues/8636) 报告 v0.2.20 的 OpenAI OAuth 账号在 `gpt-6.1-sol`、`gpt-6-astra` 的 HTTP 流式 `/v1/responses` 请求中，间歇性收到上游 `400 {"detail":"Bad Request"}`，日志为 `upstream error: 400 message=Bad Request`。

2026-11-12 10:17（北京时间），作者 `litong0127-rgb` 在讨论中补充：“我们最后是发现 我们在内部部署的时候，对请求进行了zstd 的压缩，把压缩关掉就好了”。此前关于客户端登录状态的评论只是推测，没有对应复现证据。作者没有明确指出其部署中负责压缩的组件，因此不能由该评论进一步推断是客户端、Sub2API、代理还是上游某层的实现错误。

## 当前分支的源码事实

1. [入口请求体解码](../../backend/internal/shared/httputil/body.go) 根据 `Content-Encoding` 支持 zstd/gzip/deflate，成功后删除压缩头并更新长度。zstd 解码有 8 MiB window 和 64 MiB 输出限制。[网关模型校验](../../backend/internal/transport/http/server/middleware/gateway_model_validation.go) 会在请求分流前读取并复用该结果。
2. [OAuth 出站请求构造](../../backend/internal/application/service/openai_oauth_body_compression.go) 默认重新以 zstd level 3 压缩普通 OpenAI OAuth 的 HTTP 请求，设置 `Content-Encoding: zstd`，长度来自实际压缩字节。普通转发与自动透传共享这一构造器。API Key 和 SIWC 不启用这一压缩默认值。
3. 现有账号级 `extra.codex_request_compression_enabled` 接受布尔值，显式 `false` 可关闭出站压缩，缺省为开启。当前管理页面没有该字段的专用控件；管理员账号 API 可以保存它。
4. [普通转发的上游错误处理](../../backend/internal/application/service/openai_gateway_upstream_errors.go) 对没有额外错误码或字段信息的这类 400 返回 OpenAI 兼容的 `invalid_request_error / Bad Request`，不自动换号、不修改请求重试。[自动透传](../../backend/internal/application/service/openai_gateway_passthrough.go) 在同样的默认配置下保留 HTTP 400，但客户端文案为 `upstream_error / Upstream request failed`；内部日志仍记录 `Bad Request`。[拒绝字段恢复](../../backend/internal/application/service/openai_responses_rejected_field_retry.go) 只对明确字段拒绝生效。这些行为不能据此进一步判断上游拒绝的是压缩编码还是输入内容。

## Docker 验证

工具链为 Go 1.26.11、Linux ARM64、Docker 29.2.1、PostgreSQL 18.1、Redis 8.4。实际完成后的逐场景数值见 [RESULTS.json](RESULTS.json)，原始日志 SHA-256 和统计见 [CHECKS.json](CHECKS.json)。后端二进制 SHA-256 为 `25fb8f44269be7b0273ac9fbd25d5c9b7b0d2910a641e9ae08146df2f4474bf3`。

| 检查 | 结果 |
| --- | --- |
| 基础回归 | 3 个包、29 个测试及子项通过，涵盖出站压缩范围、真实解码、入口限额/损坏帧、模型校验与明确字段重试边界 |
| HTTP 场景 | 45 项全部符合预期：28 次成功、17 次预期 400；其中 4 次损坏的编码声明在入口被拒绝，没有访问上游 |
| 上游支持 zstd | 缺省压缩请求全部成功；假上游实际解码并核对完整输入摘要 |
| 上游拒绝 zstd | 缺省压缩得到同样 400；账号开关设为 false 后两个模型、两种响应模式和两条转发路径全部成功 |
| 客户端压缩输入 | 出站压缩关闭时，合法 zstd 输入在普通/透传、流式/非流式四个场景全部成功 |
| 无关的通用 400 | 假上游拒绝所有请求时，关闭压缩仍返回 400；未做盲目重试，每个场景只有 1 次上游调用 |
| 重复并发请求 | 4 个并发 worker 发出 40 次请求，20 次流式、20 次非流式，40/40 成功且上游调用恰好 40 次 |
| 可靠结算 | 68 次成功请求对应 68 条用量记录；17 次失败没有用量记录；逐 API Key 校验无重复结算 |
| 开关恢复与账号状态 | API 保存的 false 已落库；重新设为 true 后再次观察到 zstd 和预期 400；验证结束时两个测试账号仍为 active、可调度、无账号错误 |
| 原始日志与环境清理 | 捕获 13 条 `upstream error: 400 message=Bad Request`；本次容器、内部网络和临时数据卷均已清理 |
| 文档与格式 | `make check-docs`、`git diff --check` 通过 |

测试同时覆盖两个报告模型、流式/非流式、普通/自动透传、客户端 zstd 输入、错误编码声明、账号开关持久化与恢复、重复并发请求，以及真实 PostgreSQL 中的可靠计费记录。压缩请求在假上游真实解码并校验输入摘要，避免仅依据头部推断编码正确。

以下结论需要区分：支持 zstd 的假上游能够正确读取当前压缩器的请求；一个拒绝 zstd 的上游能够触发与 issue 相同的 400 及日志；显式关闭出站压缩能够恢复后者。这些结果证明了可行的触发路径与现有缓解方法，不能确认用户此前那一次 400 的根因，也不能证明真实 ChatGPT 服务拒绝 zstd。

## 缓解与后续排查

若再次遇到同样错误，可对受影响 OAuth 账号进行有限范围对照：保留现有 `extra`，加入布尔值 `"codex_request_compression_enabled": false`，通过 `PUT /api/v1/admin/accounts/{id}` 保存后观察。先 GET 当前账号并合并此字段；通用更新 API 的 `extra` 有替换语义，单独提交一个字段可能清除其它配置。恢复为 `true` 可重新启用。

这个开关控制 Sub2API 到 OAuth 上游的 HTTP 请求压缩。若内部代理另外执行压缩、重复压缩、错误解压或删除 `Content-Encoding`，还需要检查对应组件。`Accept-Encoding` 是响应压缩协商，单独修改它不能替代关闭请求体压缩。

下一次失败应保留 UTC/本地时间、网关请求 ID、上游 `x-request-id`、实际传输协议、账号 ID/类型、模型、是否经过中继，以及失败和成功请求的 `Content-Encoding` 与实际长度。对照须使用同一账号、模型和输入；日志避免包含令牌及完整对话。不能只凭 `Bad Request` 将所有 400 自动改为换号或重试。

本次只新增诊断夹具与证据，没有修改压缩默认值、生产账号设置或网关运行逻辑。
