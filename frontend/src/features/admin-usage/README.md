# Admin Usage

管理员用量域负责用量明细、统计、筛选、导出、清理任务和错误请求联动。

## Owner

- `data/datasources/adminUsageDatasource.ts`: 用量列表、统计、搜索和清理任务协议；`adminUsageAPI` 仅保留兼容对象，新 presentation 调用使用命名导出。
- `presentation/pages/UsagePage.vue`: 用量、错误与用户排行三类视图的页面编排。
- `presentation/widgets/UsageFilters.vue`: 用户、API Key、账号、分组、模型及请求属性筛选。
- `usageTable.ts`: 用户端与管理员端共享用量表的稳定公开入口；内部实现仍由本 feature 持有。
- `usageStatsCards.ts`: 用户端与管理员端共享统计卡片的稳定公开入口。
- `admin-dashboard/data/datasources/adminDashboardDatasource.ts`: 模型统计和聚合快照的跨域数据 owner。
- `admin-users/userBalanceHistoryDialog.ts`: 余额历史弹窗的稳定公开组件出口。
- `admin-ops/errorLogTable.ts`、`errorDetailDialog.ts`: 错误表格与详情弹窗的稳定公开组件出口。

## 约束

- presentation 不得依赖 `@/api/admin`、`adminAPI` 或 `adminUsageAPI`。
- 跨 feature 组件只能通过具体命名的公开出口引用，不得直接导入其他 feature 的 `presentation/`。
- 共享用量表和统计卡片必须显式传入 `audience="user"` 或 `audience="admin"`。用户 audience 即使收到异常的管理员字段，也不得渲染账号、上游模型/端点、映射链、渠道、账号费用或账号计费信息。
- 列表 AbortSignal、300ms 搜索防抖、防陈旧序列、分页/筛选、导出分批和清理轮询时序必须保持不变。
- `/admin/usage` 路由依赖的中英文 key 由全路由 locale scope 测试覆盖。

## 验证

```sh
pnpm exec vitest run src/features/admin-usage
pnpm exec vitest run src/core/i18n/__tests__/routeLocaleCoverage.spec.ts
pnpm run lint:check
pnpm run typecheck
```
