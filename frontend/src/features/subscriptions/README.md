# User Subscriptions

本 feature 提供 `/subscriptions` 的订阅列表、配额进度、平台用量与套餐展示组件。购买和订单履约由 [billing](../billing/README.md) 持有，后台分配由 [admin-subscriptions](../admin-subscriptions/README.md) 持有。

- [data/datasources/subscriptionsDatasource.ts](data/datasources/subscriptionsDatasource.ts)：本人订阅、有效订阅、进度和汇总。
- [presentation/stores/subscriptionsStore.ts](presentation/stores/subscriptionsStore.ts)：60 秒缓存、请求去重、代际失效和轮询生命周期。
- [presentation/widgets](presentation/widgets/)：套餐卡、平台成本/额度与用量分布。
- [subscriptionStore.ts](subscriptionStore.ts)、[subscriptionPlanCard.ts](subscriptionPlanCard.ts)、[subscriptionStatus.ts](subscriptionStatus.ts)：跨 feature 的具体公开入口。

有效期、日/周/月窗口与平台额度是不同维度，以服务端进度为准。退出或切换用户时清理 Store，避免旧请求回填；套餐价格和币种复用 billing 的稳定协议与展示规则。

从 `frontend/` 执行 `pnpm exec vitest run src/features/subscriptions src/features/billing`。变更缓存、共享卡片或状态时覆盖失效响应、未知值、币种与用户/管理员词表。
