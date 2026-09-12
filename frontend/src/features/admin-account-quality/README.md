# Admin Account Quality

账号质量巡检独立管理质量探测策略、分组切换、手动运行和结果快照。

- `data/dtos/accountQualityDtos.ts`: 质量设置、运行摘要和结果协议。
- `data/datasources/accountQualityDatasource.ts`: `/admin/account-quality` 请求 owner。
- `presentation/pages/AccountQualityPage.vue`: 质量设置、摘要和结果表。

管理页操作区提供“打开公开展示页”按钮，跳转到 `/monitor/quality/public`；质量探测单次超时默认 120 秒，可在质量巡检策略中自定义 30–300 秒。超时显示为错误且不会改变连续失败计数或分组。

质量设置使用 `account_quality_settings`，运行状态使用 `account_quality_state`；账号健康巡检使用另一组设置、状态和调度器。
