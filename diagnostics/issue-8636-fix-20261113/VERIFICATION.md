# 默认压缩开启的 Codex 400 恢复

日期：2026-11-13。基线：`1d3e1ea2b`。仅在本地 Docker 中使用合成 OAuth 账号验证，没有访问真实 OpenAI 账号或部署到生产。

## 实现

[HTTP 压缩恢复](../../backend/internal/application/service/openai_oauth_compression_recovery.go)接在普通 Responses 与自动透传的实际发送点，覆盖 HTTP 流式、非流式及 compact。原有 zstd 压缩器和账号开关的默认值不变。

首次请求仍以 zstd 发送。仅在已压缩的 OpenAI OAuth 请求收到单字段 `400 {"detail":"Bad Request"}`，或单字段 `detail` 明确指出不支持 zstd/Content-Encoding 的 415 时，允许一次编码兼容重试。结构化参数错误、context limit、policy、未知附加字段、损坏/截断的错误响应和普通传输错误不触发这条恢复。

重试由 `GetBody` 取实际发送的压缩快照，通过既有有界 decoder 解码；保留 JSON 原始字节、模型、完整历史、工具配对、metadata 大整数和账号/会话身份，删除 Content-Encoding 并更新长度。使用相同账号、URL、Host、代理和 TLS 配置，经现有发送链重新生成本次 proof、执行准入并维持心跳与超时。不重新进入请求转换，不裁剪或猜测无效字段。

请求级标记在同一个 Gin context 内共享，跨同号传输重试或换号也不能补充第二次编码恢复；透传恢复消耗原有总发送预算。下一条独立请求仍默认压缩，不缓存“该账号不支持压缩”，不更新数据库中的压缩设置。

安全重放检查同时验证转换前请求和真实发送的请求，因此不会因普通 OAuth 转换已删除 `store` 或 `previous_response_id` 而绕过限制。存储/后台响应、conversation/增量 continuation、上游执行工具（包括 namespace 和 additional_tools 内的工具）、已输出语义内容、请求取消、蒸馏组及明确要求保持连接的 continuation 不进入恢复。完整的客户端工具历史可原样重放。仅 SSE 注释/可重放控制帧不阻止恢复。

## 验证方法与证据

使用 Go 1.26.11 / Linux ARM64、PostgreSQL 18.1、Redis 8.4 与 Docker。后端从当前修复源码构建，应用与假上游位于无外网的 Docker 内部网络。HTTP 请求通过应用容器内 curl 发送，假上游真实解码 zstd 并比较正文、身份摘要和实际 Content-Length；PostgreSQL 中逐 API Key 检查可靠结算。

原始代码对照使用 Go overlay，只将 `openai_gateway_forward.go`、`openai_gateway_passthrough.go` 替换为基线内容，测试与其它源码保持一致。新增恢复场景在基线失败，在修复中通过。详细数值与检查记录见 [CHECKS.json](CHECKS.json)，HTTP 明细见 [RESULTS.json](RESULTS.json)。

| 检查 | 结果 |
| --- | --- |
| 原始代码对照 | 6 个 Forward 子场景与 2 个请求级恢复子场景按预期失败，证实基线没有此恢复；修复专项测试及子项 67 项通过 |
| HTTP Docker 场景 | 61 项符合预期：45 次成功、16 次预期失败；同时覆盖两个报告模型、流式/非流式、普通转发/自动透传、合法/损坏的客户端压缩输入与账号开关兼容性 |
| 压缩拒绝恢复 | 裸 400 与显式 zstd 415 的恢复按 `zstd → identity` 顺序发送；逐次核对完整正文和账号/会话身份摘要一致、Content-Length 等于实际字节数 |
| 正常及不可恢复错误 | 支持 zstd 的上游只收到一次压缩请求；结构化参数错误只发送一次；所有编码均拒绝的上游至多收到两次后仍返回原有错误格式 |
| 默认开启的并发测试 | 4 个 worker 发出 40 个逻辑请求，40/40 成功；上游收到 40 次压缩首发和 40 次未压缩恢复，各请求正文与身份一致 |
| PostgreSQL 可靠结算 | 85 次成功请求对应 85 条用量记录；逐 API Key 无重复，16 次失败无记录；两个账号最终仍开启压缩、保持 active/可调度且无账号错误 |
| 首次扩大回归 | 17,976 项通过，1 项图片断连测试因 1 秒数据等待超时失败，4 项按原测试条件跳过；同阶段静态检查进程退出 137、无诊断输出 |
| 图片测试复核 | 基线和修复单独各运行 10 次均通过，该测试走独立 Images 路径而不经过新增 Responses 恢复 |
| 顺序扩大回归 | 12 个包、17,977 个测试及子项通过、0 失败；4 项依原测试条件跳过，涵盖 service、HTTP handler/admin/routes/middleware 和共享 HTTP 编解码 |
| 顺序静态检查 | `golangci-lint run --concurrency=1 --timeout=15m ./...` 通过，输出 `0 issues` |
| 源码结构门禁 | `make check-layout` 通过 |
| 文档与格式 | `make check-docs`、`git diff --check`、gofmt 与诊断脚本语法检查通过 |

端到端假上游从合成输入推导确定的响应，不模拟模型自身随机行为。新建的两个测试账号虽然包含显式关闭压缩的回归对照，但 40 次并发测试、全部压缩恢复场景和最终账号状态均保持压缩开启。不会把关闭开关后的成功计为压缩恢复成功。

原始 JSONL、应用日志、二进制及基线 overlay 位于 `/private/tmp/sub2api-issue-8636-fix-20261113/`，文件摘要保存在 CHECKS 中。最终后端二进制 SHA-256 为 `f30043452d7b991898cf3247e36cf2ce1033ff3db0e9e743c99d8d50d8718553`。扩大回归命令为：

```sh
go test -json -tags=unit -p 1 -count=1 -timeout=15m \
  ./internal/application/service/... \
  ./internal/transport/http/... \
  ./internal/shared/httputil/...
```

该命令在上述 Go 1.26.11 Docker 检查镜像中从 `backend/` 执行，使用 `GOMAXPROCS=2`、`GOMEMLIMIT=1200MiB`，源码只读且容器禁网。4 个跳过项分别是 DingTalk 禁用测试、旧 session reference adoption、5 万并发压力测试和依赖真实 OpenAI API 的 token 估算对比，具体测试名见 CHECKS。

## 升级、回退与边界

无数据库迁移、无新配置、不要求关闭压缩或修改账号。正常支持 zstd 的上游仍只接收一次压缩请求；遇到可恢复的压缩拒绝时，当前逻辑请求最多多一次发送和一次解码。若未压缩重试仍失败，保留第二次响应的原有错误处理，不无限重试。

可通过回退本次源码/二进制恢复旧行为，账号设置和缓存无需迁移。已有显式关闭压缩的账号仍遵循原配置，本次修复不自动开启它们；要验证默认行为，应选择原本就开启压缩的账号。

裸 400 没有足够信息证明压缩是其根因；这里只提供有界、受重放约束的兼容恢复。若上游对未压缩请求也拒绝，或代理在内部重新压缩、错误解压或移除编码头，这条恢复可能仍失败。Docker 证明的是受控协议路径，不能证明生产曾发生的某次故障已经确诊，也不能保证真实 ChatGPT 所有 400 都消失。
