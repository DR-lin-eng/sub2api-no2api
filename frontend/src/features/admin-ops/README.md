# Admin Operations

运维仪表盘 feature 负责系统健康、实时流量、告警、并发和运行设置。

- `data/dtos/opsDashboardDtos.ts`: 仪表盘 overview、snapshot、趋势、延迟和错误分布协议。
- `data/dtos/opsLogDtos.ts`: 请求明细、系统日志、运行时日志配置、清理和 sink health 协议。
- `data/dtos/opsMetricsDtos.ts`: 图片、Token、用户用量、并发、可用性和实时流量指标协议。
- `data/datasources/opsDashboardQueries.ts`: 仪表盘 snapshot 与独立趋势只读查询。
- `data/datasources/opsLogQueries.ts`: 请求明细、系统日志、运行时配置和 sink health 只读查询。
- `data/datasources/opsLogActions.ts`: 运行时日志配置保存/重置和系统日志清理操作。
- `data/datasources/opsMetricsQueries.ts`: 独立指标、并发快照和实时流量摘要只读查询。
- `data/datasources/adminOpsDatasource.ts`: 迁移期兼容 facade，以及尚待拆分的错误、告警和设置协议与操作。
- `presentation/pages/`: 路由级快照加载与卡片编排。
- `presentation/widgets/`: 工具栏、健康概览、指标网格和设置面板。
- `presentation/composables/`: 页面局部实时流量生命周期。

页面、指标 widget、请求明细和系统日志组件直接调用明确 Query/Action owner，并继续通过静态 import 归入原运维路由 chunk。新增指标优先扩展现有 snapshot 与对应 widget，保留混合版本部署所需的兼容回退，避免重新引入逐卡请求。旧 `opsAPI` 仅作为迁移兼容出口；新 snapshot、metrics 或 log 调用不得继续扩展该对象。

验证入口：

```sh
pnpm exec vitest run src/features/admin-ops
pnpm run typecheck
```
