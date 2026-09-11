# Admin Account Inspection

账号巡检 feature 负责管理员的巡检策略、手动执行、最近一次快照和账号结果分页。
启用账号级质量监控后，worker 按配置间隔对支持的平台账号发起独立探测；连续失败达到阈值时保存原分组并绑定降智分组，连续恢复通过后以 compare-and-preserve 方式恢复原分组。未配置降智分组时仅记录质量状态，不自动停调。

- `data/dtos/accountInspectionDtos.ts`: 设置、运行摘要和账号结果协议。
- `data/datasources/accountInspectionDatasource.ts`: 账号巡检 Query/Action 请求 owner。
- `presentation/widgets/QuotaUsageDistributionChart.vue`: 完整巡检快照的额度使用率分布。
- `presentation/pages/AccountInspectionPage.vue`: 设置、摘要、筛选和结果表编排。

质量监控设置字段位于 `AccountInspectionSettings`：`quality_monitoring_enabled`、`quality_interval_minutes`、`quality_failure_threshold`、`quality_recovery_threshold`、`quality_degraded_group_id` 和 `quality_max_concurrent`。探测提示词默认使用形状/口味保证数问题，要求最后一行输出 `ANSWER=整数`；判分使用独立数字 21，结果只保存状态、计数、延迟、错误和最近 24 次探测历史，不保存探测回答。

巡检默认不启用自动 runner；手动执行遵循当前保存的自动停调开关。API Key 的缓存命中率与倍率阈值为 0 时只展示，不作为异常条件。

额度分布由后端在结果分页或截断前汇总。每个账号取仍有效额度窗口中的最高使用率；`90-100%` 包含恰好 100%，`>100%` 仅统计已超过额度上限的账号，平均值不截断超额部分。无法取得有效额度上限或利用率的账号单独计入未知数量。
