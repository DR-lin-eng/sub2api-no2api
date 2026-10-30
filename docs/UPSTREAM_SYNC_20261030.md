# 上游主线增量同步审查（2026-10-30）

本轮以本项目 `ed0c9ee94eaf3b2e00cfb89677a1848bbba0f018` 为基线，冻结上游
`a3298b46ebe239fb0720b068a1eb3383955bdac2..d851a38477597157e78450d30af2d8504764d085`。
增量包含上游管理 MCP 合并 PR #8389 和其后的前端依赖安全修复提交。上一轮已移植的
`vue-i18n` v11、Node 24 与当前本地架构差异保持关闭，不重复处理。

| 上游条目 | 本轮决定 | 当前 owner 与边界 |
| --- | --- | --- |
| [#8389](https://github.com/Wei-Shaw/sub2api/pull/8389) | 兼容适配并关闭冻结差异 | 经过本轮专项审查后，MCP policy、OAuth/application、repository、transport 和 frontend owner 已建立；迁移改用 252，保留本项目 root OAuth2 issuer。安全审查、官方 SDK 互操作、真实 PostgreSQL/Redis 并发与旧节点撤销验证通过。 |
| `d851a384`（前端安全修复） | 适配并关闭 | `frontend/pnpm-workspace.yaml` 与锁文件增加 `decode-uri-component@<0.5.1` 覆盖；删除 v11 已不需要的 `__INTLIFY_JIT_COMPILATION__` 构建开关，并按实际渲染文案更新订阅卡测试。Node 24 工作流和 `vue-i18n` v11 已由本地基线覆盖。 |

机器台账见 [UPSTREAM_PRS.json](../diagnostics/upstream-sync-20261030/UPSTREAM_PRS.json)。前轮
明确暂缓的 #8389 已由本轮兼容实现 supersede；后续仅在 MCP owner、OAuth/MCP 权限契约、
脱敏策略、资源限制或 migration 252 兼容边界变化时重开。

## #8389 的安全、性能和升级审查

该 PR 在上游新增 78 个文件、7,250 行代码；本项目按当前 owner 重新组织为 103 个文件，
增加 MCP OAuth 存储迁移 252 和 Go SDK 依赖。无状态 Streamable HTTP 入口有 1 MiB 请求、
2 MiB 结果、30 秒工具超时，拒绝 JSON-RPC 批量，并对工具参数使用封闭 schema；动态注册只签发
带 HMAC 的 client ID，不把匿名注册写入数据库。Codex Security diff scan 覆盖 81 个变更项，
0 个 reportable findings。

每次工具调用仍会经过 Redis IP 限流、access credential 查询、grant 查询、用户状态查询，按分钟
触发一次 grant 使用时间写入，再复制请求进入内部管理路由并进行 JSON 解码/编码。启动时还为用户/管理员
和只读/读写组合构造四套工具 schema。对本项目而言，这会给管理面增加多次数据库往返和一套新的
鉴权/审计/迁移 owner；当前没有可复用的 application port、MCP scope 持久化或旧迁移回退契约，
因此没有把上游旧目录整包覆盖到当前项目。grant 验证和工具调用已收敛到当前 application
service，认证路径使用一次 joined credential/grant/user 查询；Docker benchmark 显示每 100 次
认证耗时低于上游四次查询形状。migration 252 的用户更新触发器覆盖旧节点停用、降权、改密、改邮箱
和删除路径，避免滚动升级/回退复活授权。

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
