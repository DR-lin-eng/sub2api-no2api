# Admin Channel Monitor

本 feature 提供 `/admin/channels/monitor` 的监控配置、手动执行、历史结果、复制和请求模板管理。用户只读视图由 [channel-monitor-user](../channel-monitor-user/README.md) 持有。

- [adminChannelMonitorDatasource.ts](data/datasources/adminChannelMonitorDatasource.ts)：监控配置与结果协议，包含 `active`/`passive` 模式、主模型与附加模型。
- [adminChannelMonitorTemplateDatasource.ts](data/datasources/adminChannelMonitorTemplateDatasource.ts)：模板 CRUD、应用到监控和关联查询。
- [presentation/widgets](presentation/widgets/)：请求覆盖参数、表单、Key 选择、过滤与执行结果。
- [ChannelMonitorPage.vue](presentation/pages/ChannelMonitorPage.vue)：列表、选择、请求和弹窗编排。

主动监控发起探测，被动监控从实际请求观测中汇总；应区分探测延迟、ping 延迟和可用率。创建配置后再核对手动执行结果与历史状态，不以保存成功代表渠道正常。复制动作保留既有幂等语义。

后端入口为 [channel_monitor_service.go](../../../../backend/internal/application/service/channel_monitor_service.go) 及同前缀 checker、runner、passive 和 repository；共享页面公开性由后端独立控制，不自动公开全部管理员配置。

## 验证

从 `frontend/` 执行 `pnpm exec vitest run src/features/admin-channel-monitor src/features/channel-monitor-user`，覆盖复制、Grok 配置、动作菜单与用户状态展示。探测和模板变更还需对应后端测试。
