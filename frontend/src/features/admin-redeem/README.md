# Admin Redeem Codes

本 feature 持有 `/admin/redeem` 的卡密列表、生成、导出、删除和批量更新。用户兑换由 [billing](../billing/README.md) 持有，也可从活动中心调用同一兑换接口。

- [data/datasources/adminRedeemDatasource.ts](data/datasources/adminRedeemDatasource.ts)：卡密请求与批量更新协议。
- [presentation/pages/RedeemPage.vue](presentation/pages/RedeemPage.vue)：筛选、选择、批量动作和导出。
- [presentation/redeemLocale.ts](presentation/redeemLocale.ts)：类型与状态的文案映射。

批量编辑区分“字段未选中”与“显式更新为零或空值”；已使用卡密的敏感字段受后端约束。导出内容包含可兑换凭据，不作为日志或普通页面缓存。发放与兑换规则来自 [redeem_service.go](../../../../backend/internal/application/service/redeem_service.go)；活动膨胀的影响见 [活动中心](../../../../docs/ACTIVITY_CENTER.md)。

从 `frontend/` 执行 `pnpm exec vitest run src/features/admin-redeem`。批量字段、已使用卡密和兑换事务还需对应后端回归。
