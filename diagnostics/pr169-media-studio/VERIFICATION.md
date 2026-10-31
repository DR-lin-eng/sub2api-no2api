# PR #169 验证记录

冻结来源、范围关闭与升级边界见 [审查文档](../../docs/PR169_MEDIA_STUDIO_REVIEW.md)。

## 验证结果

| 检查 | 结果 |
| --- | --- |
| 冻结 PR 的安全差异审查 | 195 项源文件已核对；5 项原始问题，已在适配分支修复；独立异步图片增强按用户要求排除 |
| Docker 原始源码复现 | SSE 图片计数、视频分辨率别名、同一冻结先结算后释放的 PostgreSQL 复现均通过 |
| 最终 Docker Go 1.26.10 全单元 | `go test -p 1 -tags=unit ./... -count=1` 通过 |
| 既有 MCP SDK 互通 | `go test -tags=mcp_go_client_oauth ./internal/transport/http/server/routes -run TestMCPOAuthOfficialGoSDKInteroperability -count=1` 通过 |
| Docker 集成 | 完整 service 集成通过；全新数据库的完整 repository 集成通过（16.327s），含六项原子冻结/结算/回放/并发回归 |
| 最终前端 typecheck / ESLint | Node 24、pnpm 11.17.0 Docker 中通过 |
| 项目关键前端与媒体工坊测试 | 37 files / 318 tests 通过；范围修正后的媒体工坊六文件 38 tests 通过 |
| 模板字段性能 | 每请求解析 1872 ns、1721 B、28 allocs；快照预编译 62.38 ns、0 B、0 allocs |
| 生产 Dockerfile | 最终候选 `69672f443` 镜像构建通过；`-version` 显示对应提交，运行资源检查通过 |
| 同库升级/回退/恢复 | 迁移数 `308 → 313 → 313 → 313`；业务摘要全程不变，新增 JSON 列默认空对象，未新增历史/作品库表 |
| 浏览器 UI | 普通用户工作区、作品库、中文本地缓存提示正常；1280×720 和 390×844 均无横向溢出 |
| 文档/差异检查 | `make check-docs`、`git diff --check` 通过 |
| 源码布局 | 本次新增超限已拆分；只余主线既有 `payment_fulfillment_test.go` 1201 行、`pricing_service.go` 1218 行，未重复重构或扩大 allowlist |

完整单元和关键测试日志以 gzip 保存在 [records](records/)。同库验证使用
[runtime/check.py](runtime/check.py) 和 [compose.yaml](runtime/compose.yaml)，仅绑定本机
127.0.0.1:18489；从现有基线镜像启动全新数据库，不接触部署环境。输出见
[result.json](runtime/result.json)；浏览器结果见 [browser-result.json](runtime/browser-result.json)。
未向真实上游提交生成请求或付费任务。UI 检查中的管理员 fixture 降权后，旧浏览器缓存触发的
管理员设置/版本请求收到拒绝，未绕过运营确认或修改真实管理员账号。

首次完整 integration 使用先前聚焦测试留下的数据，导致账号列表和统计断言受污染，且 transport
在源码编辑中途编译。该失败保留为 `backend-integration-initial.log.gz`。新增测试已补齐清理；
在全新数据库重跑完整 repository 套件通过，最终冻结源码的全单元和相关 transport 测试通过。
首次升级 fixture 未等待 PostgreSQL 健康，应用连接被拒绝；脚本增加 `--wait` 后四阶段验证通过。

Codex Security 原始差异扫描：`e9581c2e-d83a-40f6-b528-7bc40b52e9ef`，已完成。它记录冻结 PR
的缺陷，不将适配分支描述成原始 PR 无问题。原始复现与报告保存在扫描的托管 artifact 目录中。

后续仅测试 helper 移位、无效赋值修正后的 SSE/归属定向回归通过（service 0.224s、middleware 0.029s）。本次验收专用的应用、PostgreSQL、Redis 容器及网络已清理。

## CI 与发布

整合 PR 为 [#174](https://github.com/DR-lin-eng/sub2api-no2api/pull/174)，运行时代码候选为 `69672f443`。合并门禁包括 CI、Security Scan 和 Docker Image；检查状态以该 PR 的精确 HEAD 检查为准。合并后继续核对主线同一合并提交的工作流，不以旧提交或分支状态代替。
