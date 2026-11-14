# 2026-11-06 桌面请求适配验证

输入为用户提供的 20261030、20261031、20261101、20261106 四份逆向报告，以及最后一版的文档补丁和 `VERIFICATION.txt`。文件哈希见 [SOURCE_HASHES.json](SOURCE_HASHES.json)。补丁只在临时文档副本上复原，结果与 20261106 报告逐字节相同；没有将该文档补丁应用到产品源码。

源码起点为 `f7babf321`。改动的协议范围与端点边界见 [桌面对照文档](../../docs/codex/desktop-request-alignment.md)。

- 模型 Responses/Compact 发送点清除旧 integrity receipt，不加载或回存 `X-OAI-IS` 更新状态；metadata 同时清理完整性和本地路由字段。
- Workspace discovery 在出站/缓存命中前校验已知 account/user，并显式发送 `Cache-Control: no-store`。
- 401 刷新仍只尝试一次；若刷新改变已知主体，终止本轮重发。HTTP、透传、SSE、WS 拨号与 WS 错误事件共享该边界。
- WS 池把最终账号/FedRAMP 头与 credential principal 摘要加入兼容键；身份不变复用，身份改变替换空闲连接。

验证使用 Docker 内 Go 1.26.10、Linux ARM64、现有模块/构建缓存；Go 测试和编译步骤关闭外网。专项测试包含构造测试、模拟 OAuth 刷新和传输故障，以及现有 `httptest` 真实本地 HTTP/SSE/WebSocket 握手。上述结果不代表真实 OpenAI 账号、Apple 硬件证明、服务端风控或生产 TTFT 已通过验收。

复现相关测试（在 `backend/`，使用仓库 Go 工具链）：

```sh
go test -tags=unit -count=1 -run 'RefreshPrincipalChange|TokenExpired|TeamGenericUnauthorized|DesktopAlignment|CodexDesktop|OpenAICodexDesktopHeaders|OpenAIWSPoolRebuildsHandshakeWhenOAuthPrincipalChanges|OpenAIWorkspaceRouting|DiscoveredWorkspaceRouting|OpenAIIntegrity' ./internal/application/service
go test -tags=unit -p 2 -count=1 -timeout=15m ./internal/application/service/... ./internal/transport/http/... ./internal/shared/openai/... ./internal/shared/remotecontrol/... ./internal/platform/liveattestation/...
go build ./cmd/server
```

仓库根目录另运行 `make check-docs` 与 `make -C backend check-layout`。检查结果如下；机器结果与测试源码哈希见 [RESULTS.json](RESULTS.json)，仓库既有门禁问题的基线证据见 [REPOSITORY_CHECKS.json](REPOSITORY_CHECKS.json)。

| 验证 | 结果 |
| --- | --- |
| 报告补丁复原 | 退出码 0，与 20261106 报告逐字节相同 |
| 原始实现运行新增回归 | 6 个测试组按预期失败，复现旧完整性状态、控制字段清理、WS 主体复用、HTTP/WS 跨主体重发和 workspace 缓存复用问题；没有编译失败 |
| 修改后的相关完整回归 | 14 个测试包通过，覆盖 application/service、HTTP transport、OpenAI shared、remotecontrol 和 liveattestation |
| 最终专项复验 | workspace 缓存用例补强后通过全部专项场景；生产 Go 文件没有再改变 |
| 后端编译 | Docker Linux ARM64 / Go 1.26.10 通过 |
| 格式与差异 | gofmt 和 git diff --check 通过 |
| 本轮文档本地链接 | 无缺失 |
| make check-docs | 基线已有缺失链接 docs/PR169_MEDIA_STUDIO_REVIEW.md，未通过 |
| make -C backend check-layout | 基线已有 payment_fulfillment_test.go 1201 行、pricing_service.go 1218 行超限，未通过 |

原始记录在 [records](records/)：`baseline-final.log.gz`、`regression-final.log.gz`、`focused-final.log.gz`。`regression.log.gz` 保留首轮完整回归中新增测试误检查输入 header 副本的失败；修正为检查发送工厂返回的 header 后，完整回归和最终专项均通过。未将这次测试断言错误当成产品修复证据。

所有验证使用模拟 OAuth 凭据和隔离本地上游。没有部署、发布、修改远端账号或发起真实 OpenAI 推理请求；没有运行 GitHub CI。
