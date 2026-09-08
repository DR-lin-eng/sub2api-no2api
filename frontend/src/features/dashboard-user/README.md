# User Dashboard

本 feature 持有 `/dashboard` 的本人余额/用量摘要、趋势、模型分布、近期记录、API Key 统计与快捷入口。

- [presentation/pages/DashboardPage.vue](presentation/pages/DashboardPage.vue)：请求、刷新、筛选和生命周期。
- [presentation/widgets](presentation/widgets/)：统计、图表、近期记录、Key 用量和快捷动作。
- 本目录没有独立 datasource；用量查询由 [usageDatasource.ts](../usage/data/datasources/usageDatasource.ts) 提供，用户身份来自 auth/profile，Key 和订阅由各自 feature 提供。

这里消费当前用户范围内的 `/usage/dashboard` 接口，不复用 `/admin/dashboard`。快照、近期记录与待结算值有各自刷新语义；不能把待结算量误认为已完成账务。快捷入口还需遵循站点功能开关。

从 `frontend/` 执行 `pnpm exec vitest run src/features/dashboard-user src/features/usage`；数据口径变化需同步后端 usage dashboard 测试。
