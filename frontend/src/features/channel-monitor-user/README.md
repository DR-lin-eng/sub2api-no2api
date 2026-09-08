# User Channel Monitor

本 feature 提供只读监控组件与 `/monitor/public` 共享页面；登录后的 `/monitor` 页面由 [channels-user](../channels-user/README.md) 组合这些组件。

- [data/datasources/channelMonitorUserDatasource.ts](data/datasources/channelMonitorUserDatasource.ts)：登录接口 `/channel-monitors` 与共享接口 `/channel-status-share`，含列表、详情和批量状态。
- [presentation/widgets](presentation/widgets/)：监控卡片、时间线、可用率、延迟和详情。
- [presentation/composables/useChannelMonitorFormat.ts](presentation/composables/useChannelMonitorFormat.ts)：指标与时间展示。
- [channelMonitorLocale.ts](channelMonitorLocale.ts)：共享文案入口。

共享页面不是管理员数据的匿名版本；共享开关和返回内容由 [公共监控路由](../../../../backend/internal/transport/http/server/routes/channel_monitor_public.go) 及 handler 控制。`unknown`、`error`、`failed` 等状态应分别展示，不把缺失延迟当成 0ms 或健康。

从 `frontend/` 执行 `pnpm exec vitest run src/features/channel-monitor-user`，并核对后端用户/公共监控测试。新增状态需覆盖中英文及未来未知值。
