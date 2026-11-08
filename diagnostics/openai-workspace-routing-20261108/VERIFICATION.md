# OpenAI 工作区发现 401 与无效 WS 回退修复

基线：`77a8d0a88`。本记录对应本地源码修复及 Docker 验证，不代表生产部署或真实账号验证。

## 问题与修复

WHAM `GET /backend-api/wham/accounts/check` 返回 401 时，旧实现直接生成
`openai_workspace_routing_unavailable`、状态 502 的账号 failover。非流式预热请求随后将这个
本地路由错误识别为 WS 临时 5xx，记录 `ws_http_fallback / non_stream_transient`，再经 HTTP
重复同一发现。请求未到达模型接口，也未进入模型 401 的刷新、账号错误标记和停止调度流程。

现在发现接口的 401/403 会使用默认官方 Codex 路由，按凭据 owner、当前 token 摘要和工作区
缓存 60 秒；并发请求、WS URL/握手头和 HTTP 回退共用该结果。换 token 立即重新发现，
账号/用户主体校验仍在缓存读取前执行。默认路由保持选定账号头并禁止 HTTP 重定向。
其它发现错误、非法路由和主体不一致仍终止当前账号尝试，且不触发非流式 WS 临时 5xx 恢复。

实际模型返回 401 时继续运行原有鉴权策略：具备刷新条件时至多刷新一次；确认的
`Unauthorized`、撤销或缺失刷新凭据等错误写入账号错误状态并立即阻断调度；可恢复的
过期错误仍按既有规则冷却。本次未修改这些错误策略，也不会因为辅助发现接口被拒绝就
把仍能访问模型的账号误标为失效。

## Docker 证据

工具链：Go 1.26.10、Linux ARM64、Docker 29.2.1。测试容器使用 `--network none`，
通过仓库现有 mock upstream 和本地 HTTP/WS 服务验证协议；未使用真实 OpenAI 凭据。

| 检查 | 结果 |
| --- | --- |
| 原始 HEAD 对照回放 | 新增 8 个顶层测试全部失败，含表格子项共 25 个失败；重现 401→502 和账号错误处理未触发 |
| 定向修复回归 | 108 项通过，覆盖 HTTP/透传、流式/非流式、WS 预热、WS→HTTP、单次刷新、账号错误持久化调用和运行态阻断 |
| 服务、HTTP、OpenAI 共享协议回归 | 12 个包通过，17,827 项通过、4 项跳过；计数包括顶层与表格子项 |
| Docker 后端构建及版本命令 | 通过，构建产物可执行，报告版本 `0.1.235` |
| Docker 竞态检查 | 61 项通过，无竞态报告 |
| Docker 后端静态检查 | `golangci-lint` 报告 `0 issues` |
| 文档链接检查与 diff 空白检查 | 通过 |
| 后端 source-layout 门禁 | 基线已有失败：`payment_fulfillment_test.go` 为 1201 行，限制 1200；该文件与 HEAD 完全一致 |

完整 Go 回归命令：

```sh
go test -json -tags=unit -p 2 -count=1 -timeout=15m \
  ./internal/application/service/... \
  ./internal/transport/http/... \
  ./internal/shared/openai/...
```

竞态检查使用本目录 `Dockerfile.test`，在相同源码与 Go 模块缓存挂载下执行：

```sh
go test -json -race -tags=unit -p 1 -count=1 -timeout=10m \
  ./internal/application/service \
  -run 'WorkspaceRoutingDiscoveryDenied|ForwardWorkspaceDiscovery|CodexPrewarmNonStream|TransientHTTPRecovery|WSNonStreamDoesNotRecoverWorkspaceRoutingFailureThroughHTTP'
```

静态检查执行 `golangci-lint run --timeout=10m ./...`；构建执行
`go build -p 1 -o /results/sub2api ./cmd/server` 与 `/results/sub2api -version`。
修复前回放通过 Go overlay 使用 HEAD 的两个实现文件，只向旧缓存结构添加不参与行为的
`expiresAt` 字段以编译新增到期测试，不改变旧版运行逻辑，不改动当前工作区实现。

跳过项为原测试自身的条件：DingTalk OAuth 禁用场景、旧版空 session reference adoption、
5 万并发压力测试及依赖真实 OpenAI API 的 token 估算对比。原始 JSONL 与构建日志保存在
本机 `/private/tmp/sub2api-workspace-routing-20261108/`；`RESULTS.json` 保存结果摘要与 SHA-256。

生产账号状态尚未修改。本地错误持久化断言验证的是 repository `SetError` 调用与运行态阻断，
未执行 PostgreSQL/Redis 或生产管理端的数据写入。部署后需核对实际模型鉴权结果和账号状态。
