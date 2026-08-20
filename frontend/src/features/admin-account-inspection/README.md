# Admin Account Inspection

账号巡检 feature 负责管理员的巡检策略、手动执行、最近一次快照和账号结果分页。

- `data/dtos/accountInspectionDtos.ts`: 设置、运行摘要和账号结果协议。
- `data/datasources/accountInspectionDatasource.ts`: 账号巡检 Query/Action 请求 owner。
- `presentation/widgets/QuotaUsageDistributionChart.vue`: 完整巡检快照的额度使用率分布。
- `presentation/pages/AccountInspectionPage.vue`: 设置、摘要、筛选和结果表编排。

巡检默认不启用自动 runner；手动执行遵循当前保存的自动停调开关。API Key 的缓存命中率与倍率阈值为 0 时只展示，不作为异常条件。

额度分布由后端在结果分页或截断前汇总。每个账号取仍有效额度窗口中的最高使用率；`90-100%` 包含恰好 100%，`>100%` 仅统计已超过额度上限的账号，平均值不截断超额部分。无法取得有效额度上限或利用率的账号单独计入未知数量。
