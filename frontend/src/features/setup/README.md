# Setup

本 feature 持有首次安装 `/setup` 向导：数据库与 Redis 参数、连接测试、管理员和服务参数，以及安装完成后的跳转。

- [data/datasources/setupDatasource.ts](data/datasources/setupDatasource.ts)：`/setup/status`、`test-db`、`test-redis`、`install`。
- [presentation/pages/SetupWizardPage.vue](presentation/pages/SetupWizardPage.vue)：步骤与提交编排。
- 后端事实源：[bootstrap/setup](../../../../backend/internal/bootstrap/setup/README.md)。

Setup API 不在 `/api/v1` 下，当前保留专用 Axios client，并通过共享 URL helper 解析根地址；这是已有初始化入口，不是新增普通业务 client 的范例。数据库、Redis 和管理员密码只用于安装流程，不能写入日志或浏览器持久缓存。

服务完成安装后，路由守卫依据 `needs_setup` 与实际登录状态决定后续跳转，不能无条件把普通用户送入管理页。当前 feature 没有独立 Vitest；从 `frontend/` 执行 `pnpm exec vitest run src/core/routes` 和 `pnpm run typecheck`，安装动作另需后端 bootstrap/setup 测试及独立空数据库。
