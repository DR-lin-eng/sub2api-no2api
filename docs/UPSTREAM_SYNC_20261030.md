# 上游主线增量同步审查（2026-10-30）

本轮以本项目 `ed0c9ee94eaf3b2e00cfb89677a1848bbba0f018` 为基线，冻结上游
`a3298b46ebe239fb0720b068a1eb3383955bdac2..d851a38477597157e78450d30af2d8504764d085`。
增量包含上游管理 MCP 合并 PR #8389 和其后的前端依赖安全修复提交。上一轮已移植的
`vue-i18n` v11、Node 24 与当前本地架构差异保持关闭，不重复处理。

| 上游条目 | 本轮决定 | 当前 owner 与边界 |
| --- | --- | --- |
| [#8389](https://github.com/Wei-Shaw/sub2api/pull/8389) | 专项暂缓，未合入 | 上游旧 `handler/service/repository` 树新增 OAuth 管理 MCP；本项目没有对应的 management MCP/application port owner。其 `249_mcp_oauth.sql` 与本项目已有 `249_account_model_metadata_rolling_writes.sql` 冲突，当前迁移已到 307；直接合并会绕过 `transport -> application -> domain -> infrastructure` 边界。 |
| `d851a384`（前端安全修复） | 适配并关闭 | `frontend/pnpm-workspace.yaml` 与锁文件增加 `decode-uri-component@<0.5.1` 覆盖；删除 v11 已不需要的 `__INTLIFY_JIT_COMPILATION__` 构建开关，并按实际渲染文案更新订阅卡测试。Node 24 工作流和 `vue-i18n` v11 已由本地基线覆盖。 |

机器台账见 [UPSTREAM_PRS.json](../diagnostics/upstream-sync-20261030/UPSTREAM_PRS.json)。#8389
保留为明确专项暂缓；后续只有建立本项目 owner、迁移编号和 OAuth/MCP 权限契约后才重开，避免
未来把同一旧树差异反复检查成“已实现”。

## #8389 的安全、性能和升级审查

该 PR 在上游新增 78 个文件、7,250 行代码、MCP OAuth 存储迁移和 `go-sdk` 依赖升级。其
无状态 Streamable HTTP 入口有 1 MiB 请求、2 MiB 结果、30 秒工具超时，拒绝 JSON-RPC 批量，
并对工具参数使用封闭 schema；动态注册只签发带 HMAC 的 client ID，不把匿名注册写入数据库，
这些边界本身没有发现应直接回拣到当前主线的安全修复。

每次工具调用仍会经过 Redis IP 限流、access credential 查询、grant 查询、用户状态查询，按分钟
触发一次 grant 使用时间写入，再复制请求进入内部管理路由并进行 JSON 解码/编码。启动时还为用户/管理员
和只读/读写组合构造四套工具 schema。对本项目而言，这会给管理面增加多次数据库往返和一套新的
鉴权/审计/迁移 owner；当前没有可复用的 application port、MCP scope 持久化或旧迁移回退契约，
因此不能把上游旧目录整包合入来换取功能。未来专项实现应先把 grant 验证和工具调用收敛到当前
application service，明确每调用数据库查询预算并对高并发工具列表/调用做 Docker benchmark，再接入
新的迁移编号和同库回退矩阵。

## 平滑升级与性能

本轮新增的依赖覆盖只影响前端解析树，不改变业务 API、数据库、缓存、凭据或运行时配置；旧节点
会继续使用原锁文件依赖。`vue-i18n` v11 已在本项目基线中运行，删除过时构建宏不会改变消息
渲染契约，测试改为断言真实的英文文案。回退只需恢复工作区提交，不执行 Down SQL。

`decode-uri-component` 覆盖不在网关请求热路径中增加代码；它只约束未来解析依赖的最低版本。
本轮没有发现需要重构的性能回归。生产构建仍报告既有的超大前端 chunk 警告，未因本轮依赖修复
扩大分包或引入新的运行时模块。

## Docker 验证

使用项目 `Dockerfile` 的 Node 24、pnpm 11.17.0 和 Go 1.26.10 构建验证。生产镜像构建、前端
锁文件校验与 Vite 构建通过；镜像二进制 `-version` 输出 `Sub2API 0.1.230`，运行时资源、入口
脚本和可执行文件检查通过。Docker 后端构建阶段的 `go test -tags=unit ./... -count=1` 通过；
源码布局门禁只报告此前已登记的 `payment_fulfillment_test.go`（1201 行）和 `pricing_service.go`
（1218 行）两处既有超限，本轮没有扩大 allowlist 或重复重构。完整命令和结果摘要保存在
[VERIFICATION.md](../diagnostics/upstream-sync-20261030/VERIFICATION.md)。
