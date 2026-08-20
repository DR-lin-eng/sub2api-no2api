# Admin Account Inspection

账号巡检 feature 负责管理员的巡检策略、手动执行、最近一次快照和账号结果分页。

- `data/dtos/accountInspectionDtos.ts`: 设置、运行摘要和账号结果协议。
- `data/datasources/accountInspectionDatasource.ts`: 账号巡检 Query/Action 请求 owner。
- `presentation/pages/AccountInspectionPage.vue`: 设置、摘要、筛选和结果表编排。

巡检默认不启用自动 runner；手动执行遵循当前保存的自动停调开关。API Key 的缓存命中率与倍率阈值为 0 时只展示，不作为异常条件。
