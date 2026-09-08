# Admin Subscriptions

本 feature 提供 `/admin/subscriptions` 的订阅分配、批量分配、延期、撤销、恢复和日/周/月额度重置。

- [data/datasources/adminSubscriptionsDatasource.ts](data/datasources/adminSubscriptionsDatasource.ts)：全站、按用户/分组的订阅查询，以及管理动作。
- [presentation/pages/SubscriptionsPage.vue](presentation/pages/SubscriptionsPage.vue)：筛选列表、详情和操作编排。
- [subscriptionStatus.ts](subscriptionStatus.ts)：订阅状态共享展示入口。

撤销/恢复、延期和额度重置是不同操作；额度窗口重置不等于延长有效期。状态和网关准入仍以后端 subscription service 为准。套餐销售与订单在 [admin-orders](../admin-orders/README.md)，用户进度在 [subscriptions](../subscriptions/README.md)，避免在本 feature 重建支付或账务规则。

## 验证

当前目录没有独立 Vitest。从 `frontend/` 执行 `pnpm run typecheck` 和 `pnpm exec vitest run src/features/subscriptions/__tests__/subscriptionStatus.spec.ts`；修改管理操作时补充 payload、失败恢复和写后刷新测试，并验证后端订阅与计费准入。
