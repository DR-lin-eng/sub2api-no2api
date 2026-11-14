# 画图模型独立调度验证（2026-11-09）

## 行为与边界

网关设置新增 `image_independent_scheduling_enabled`，默认关闭。开启后，Images 端点及 GPT Image、Grok 图片、Gemini 图片模型可使用状态正常但 `schedulable=false` 的账号。错误、禁用、删除、过期及错误冷却账号仍被排除；账号/模型限流、并发、分组权限和失败排除继续检查。普通文本请求不放宽调度资格。

独立候选从数据库加载，不读写普通调度快照或候选索引。粘性会话、previous_response_id 和等待槽位后的复查使用请求策略；WS 切换到文本模型时恢复普通资格。设置保存后当前节点立即更新，其他节点最多 5 秒刷新。无需表结构迁移；回退前关闭开关。

## Docker 验证

- 固定 Go 1.26.10 的 service 全量单元回归通过；新增独立调度、Gemini、图片模型冷却、粘性/previous_response_id、失败排除及等待后健康复查通过。
- HTTP 全量回归通过：handler、admin handler、DTO、management MCP、server/API 契约、middleware、routes。
- Repository 单元回归通过；PostgreSQL 18 + Redis 8 的候选查询集成回归通过，覆盖有分组/无分组、单/多平台、正常、停调、错误、禁用、删除、过期、限流、过载和临时冷却。
- 前端 typecheck、lint 通过；设置页 57 项测试和静态文案/路由语言覆盖通过。新增测试覆盖默认值、开启保存、刷新回填与关闭保存。
- 完整生产 Docker 镜像从当前源码构建通过：`sub2api:image-independent-scheduling-20261109`，最终镜像 ID 为 `sha256:a95fe315215c15a3f25ecc3df8b9372364a63ee7a25154ccdd83968e5d24a297`。Docker Hub 语法镜像鉴权曾返回临时 EOF，最终使用相同 Dockerfile 内容及 Docker 内置构建前端完成构建。
- 实际 Docker 网关 + 临时数据库/Redis + 本地模拟图片上游通过下表，不调用真实收费上游。

| 场景 | 结果 |
| --- | --- |
| 开关关闭，正常账号与停调账号并存 | 图片使用正常且可调度的账号 |
| 开关开启，正常账号与停调账号并存 | 图片可使用优先级更高的健康停调账号 |
| 流式与非流式 Images | 均完成输出 |
| 保存其他设置 | 独立调度开关保留 |
| 仅剩健康停调账号时请求文本 | 返回 503，未访问上游 |
| 图片账号转为错误 | 使用另一个健康停调账号 |
| 所有图片账号均为错误 | 返回 503，不向错误账号发送图片请求 |

Playwright 实际浏览器在 1440 px 桌面和 390 px 手机宽度验证：开关可操作，保存后刷新回读为开启；手机无横向溢出。测试容器与网络在验收后清理，不涉及生产部署。

## 已有检查问题

以下问题在本次修改前的 `HEAD` 已存在，对应文件没有改动：

- 后端源码布局检查：`backend/internal/application/service/payment_fulfillment_test.go` 为 1201 行，超出 1200 行限制；已用 `git show HEAD:...` 确认。
- 前端 `settingsGatewayResilienceModularization.spec.ts` 的 Codex 卡片模板哈希失配。卡片文件与 `HEAD` 完全相同；当前/基线哈希为 `f72f76ef998051b6103723348ab9ed9cdfd37c7353bc88676583558c8b56cb03`，测试仍预期 `c78190ebe0430d38d939afcd187759beb1e88dd0fda341eb303e65facf4fdb3e`。设置 feature 与语言覆盖合计 151 项通过，仅此 1 项失败。

`make check-docs` 与 `git diff --check` 通过。真实 ChatGPT/OAuth 出图和生产环境未测试。
