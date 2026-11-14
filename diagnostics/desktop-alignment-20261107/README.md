# 2026-11-07 桌面请求适配与 Docker 验证

来源为用户提供的 20261107 逆向报告，macOS `26.1103.41825 / build 25679`、Linux
`26.1103.32239 / build 25471`。报告及实际读取的关键证据哈希见 [SOURCE_HASHES.json](SOURCE_HASHES.json)。
源码基线为 `345e85e7e`；没有执行输入目录的 App、CLI、addon 或回滚脚本。

改动及端点边界见 [桌面请求对照](../../docs/codex/desktop-request-alignment.md)。本轮修复：

- 已知账号、用户一致时，仍检查个人/工作区套餐类别；跨类别终止当前请求的凭证替换和重放。
- 请求主动刷新、401 后刷新及等待其他 worker 的缓存结果使用相同身份校验。
- access token 的 JWT `exp` 与同一 token 对应的 `expires_at` 取较早值；窗口为 5 分钟。
- WS 池将套餐类别加入兼容键；同类别升级继续复用，不兼容的空闲连接被关闭并重新握手。

测试使用 Docker `golang:1.26.10-alpine`、Linux ARM64，容器设置 `--network none`、
`GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off CGO_ENABLED=0`，复用现有 Go module/build 缓存卷。
生产源码以只读方式挂载。模拟 OAuth 凭据与本地 `httptest` HTTP/SSE/WS 不访问 OpenAI。

在仓库根目录执行，以下为 Docker 内命令（工作目录为挂载的 `backend/`）：

```sh
go test -json -tags=unit -p 2 -count=1 -timeout=10m ./internal/application/service \
  -run 'DesktopPlan|DesktopJWT|DesktopRefreshPlan|DesktopNewer|DesktopExpired|DesktopExpiry|DesktopNewVersions|DesktopWaitedToken|OAuthPrincipalChanges|RefreshPrincipalChange|TeamGenericUnauthorized|TokenExpired|CodexDesktop|DesktopAlignment'
go test -json -tags=unit -p 2 -count=1 -timeout=15m \
  ./internal/application/service/... ./internal/transport/http/... ./internal/shared/openai/...
go build -o /tmp/sub2api ./cmd/server
```

验证结果记录在 [RESULTS.json](RESULTS.json)；压缩原始测试输出存于 [records](records/)。

| 验证 | 最终结果 |
| --- | --- |
| 原始源码加回归 fixture | 5 组按预期失败，分别复现套餐类别、JWT 提前/到期刷新、刷新后跨类别使用、过期缓存及 WS 类别复用问题 |
| Docker 最终专项 | 161 个测试及子场景通过，退出码 0 |
| Docker 相关完整回归 | 12 个包通过；17742 个测试及子场景 pass 事件，4 个既有测试跳过，退出码 0 |
| Docker 后端编译 | Go 1.26.10 / Linux ARM64，退出码 0 |

基线复现将原 HEAD 的 `backend/` 导出到独立临时目录，只加入新回归及修改后的 WS 测试；
省略一个直接调用新期限 helper 的测试以保持原始源码可编译，没有将新生产 helper 加入基线。
原始实现应在套餐类别切换、JWT 期限/5 分钟刷新和 WS 类别复用测试中失败。

仓库根目录执行 `make check-docs` 与 `make -C backend check-layout`。
布局门禁在基线已存在 `payment_fulfillment_test.go` 1201 行超过 1200 行的问题；
`git show HEAD:backend/internal/application/service/payment_fulfillment_test.go | wc -l` 同为 1201。
本次未修改该文件。

本轮不构成真实 OpenAI 推理、Team 实号 401、生图、Apple 证明或官方线上 Statsig 配置验证；
没有部署、发布或修改上游账号。APNs、执行器并发和 Library 生图的独立协议边界已写入对照文档。
