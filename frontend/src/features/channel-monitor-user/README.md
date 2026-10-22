# User Channel Monitor

本 feature 提供只读监控组件与 `/monitor/public` 共享页面；登录后的 `/monitor` 页面由 [channels-user](../channels-user/README.md) 组合这些组件。

- [data/datasources/channelMonitorUserDatasource.ts](data/datasources/channelMonitorUserDatasource.ts)：登录接口 `/channel-monitors` 与共享接口 `/channel-status-share`，含列表、详情和批量状态。
- [presentation/widgets](presentation/widgets/)：按 AI 类型（provider）聚合的状态卡片，卡片内分别展示主动探测与被动流量，各分组保留独立状态、可用率、延迟、时间线和详情。
- [presentation/composables/useChannelMonitorFormat.ts](presentation/composables/useChannelMonitorFormat.ts)：指标与时间展示。
- [channelMonitorLocale.ts](channelMonitorLocale.ts)：共享文案入口。

共享页面不是管理员数据的匿名版本；共享开关和返回内容由 [公共监控路由](../../../../backend/internal/transport/http/server/routes/channel_monitor_public.go) 及 handler 控制。`unknown`、`error`、`failed` 等状态应分别展示，不把缺失延迟当成 0ms 或健康。

提供商卡片保留整卡折叠和综合健康度排序，自动刷新不重置本地折叠状态。折叠按钮提供本地化标签和展开状态；7/15/30 天可用率继续使用各自窗口的数据。

管理员通过 `channel_monitor_timeline_points` 配置登录页及共享页卡片的时间线柱数，范围为 1–60 的整数，默认 20。旧服务端缺少该字段或本地缓存无效时使用 20；只影响显示，不改变采样、保留时间、可用率窗口或详情数据。跨 feature 复用格式化能力的稳定入口为 [index.ts](index.ts)。

从 `frontend/` 执行 `pnpm exec vitest run src/features/channel-monitor-user`，并核对后端用户/公共监控测试。新增状态需覆盖中英文及未来未知值。
