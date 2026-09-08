# Admin Dashboard

本 feature 持有 `/admin/dashboard` 的全站经营与用量汇总、趋势、模型/分组统计和排名；实时运维排障由 [admin-ops](../admin-ops/README.md) 持有。

- [data/datasources/adminDashboardDatasource.ts](data/datasources/adminDashboardDatasource.ts)：管理仪表盘查询、聚合快照和批量用户/Key 用量；也被其他管理 feature 消费。
- [presentation/pages/DashboardPage.vue](presentation/pages/DashboardPage.vue)：筛选、请求和图表编排。
- [__tests__/DashboardPage.spec.ts](__tests__/DashboardPage.spec.ts)：现有页面回归。

统计的时间范围、时区、分组和费用口径必须随请求保留。聚合快照与实时信号并非同一刷新时刻，不能用管理端统计替代账务事实或把全站接口复用于普通用户仪表盘。

从 `frontend/` 执行 `pnpm exec vitest run src/features/admin-dashboard`；共享 datasource 变更还需检查 admin-usage、admin-users 和后端 dashboard/usage 聚合测试。
