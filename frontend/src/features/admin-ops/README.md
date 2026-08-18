# Admin Operations

运维仪表盘 feature 负责系统健康、实时流量、告警、并发和运行设置。

- `data/dtos/opsDashboardDtos.ts`: 仪表盘 overview、snapshot、趋势、延迟和错误分布协议。
- `data/dtos/opsAlertDtos.ts`: 告警规则、事件、筛选和静默请求协议。
- `data/dtos/opsErrorDtos.ts`: 统一与拆分错误列表、详情、分页筛选和关联上游详情协议。
- `data/dtos/opsLogDtos.ts`: 请求明细、系统日志、运行时日志配置、清理和 sink health 协议。
- `data/dtos/opsMetricsDtos.ts`: 图片、Token、用户用量、并发、可用性和实时流量指标协议。
- `data/dtos/opsSettingsDtos.ts`: 通知、运行时、高级设置、保留策略和指标阈值协议。
- `data/datasources/opsAlertQueries.ts`: 告警规则、事件列表与详情查询。
- `data/datasources/opsAlertActions.ts`: 告警规则写入、事件状态和静默操作。
- `data/datasources/opsDashboardQueries.ts`: 仪表盘 snapshot 与独立趋势只读查询。
- `data/datasources/opsErrorQueries.ts`: 统一错误、request/upstream 拆分错误、详情和关联上游错误查询。
- `data/datasources/opsErrorActions.ts`: 统一错误与 request/upstream 拆分错误的 resolved 状态操作。
- `data/datasources/opsLogQueries.ts`: 请求明细、系统日志、运行时配置和 sink health 只读查询。
- `data/datasources/opsLogActions.ts`: 运行时日志配置保存/重置和系统日志清理操作。
- `data/datasources/opsMetricsQueries.ts`: 独立指标、并发快照和实时流量摘要只读查询。
- `data/datasources/opsSettingsQueries.ts`: 通知、运行时、高级设置、统一快照和指标阈值查询。
- `data/datasources/opsSettingsActions.ts`: 通知、运行时、高级设置和指标阈值保存操作。
- `data/datasources/opsRealtimeSubscription.ts`: QPS WebSocket 鉴权、状态、陈旧检测与重连生命周期。
- `data/datasources/adminOpsDatasource.ts`: 旧 `src/api/admin` 使用的纯兼容 facade；不再拥有协议或请求实现。
- `presentation/pages/`: 路由级快照加载与卡片编排。
- `presentation/widgets/`: 工具栏、健康概览、指标网格和设置面板。
- `presentation/composables/`: 页面局部实时流量生命周期。

页面、指标、错误、日志、告警和设置组件直接调用明确 Query/Action owner，并继续通过静态 import 归入原运维路由 chunk。`admin-usage` 的错误页签直接读取公开的错误 Query/DTO owner，仍使用 legacy unified endpoint 保持筛选与详情行为；告警规则跨 feature 读取分组时直接使用 `admin-groups` datasource。新增指标优先扩展现有 snapshot 与对应 widget，保留混合版本部署所需的兼容回退，避免重新引入逐卡请求。旧 `opsAPI` 仅作为兼容出口，不得继续承载实现或新增调用。

验证入口：

```sh
pnpm exec vitest run src/features/admin-ops
pnpm run typecheck
```
