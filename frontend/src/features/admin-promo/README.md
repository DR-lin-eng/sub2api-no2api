# Admin Promo Codes

本 feature 提供 `/admin/promo-codes` 的注册优惠码管理；它与 [卡密兑换](../admin-redeem/README.md) 及支付订单不是同一流程。

- [data/datasources/adminPromoDatasource.ts](data/datasources/adminPromoDatasource.ts)：分页筛选、创建、修改、删除和使用记录。
- [presentation/pages/PromoCodesPage.vue](presentation/pages/PromoCodesPage.vue)：优惠码配置与记录弹窗。
- 注册校验从 [auth 路由](../../../../backend/internal/transport/http/server/routes/auth.go) 继续追踪；管理接口从 [admin 路由](../../../../backend/internal/transport/http/server/routes/admin.go) 的 `registerPromoCodeRoutes` 进入。

状态、使用次数限制和有效期必须由后端校验。管理员修改优惠码后应刷新列表与详情；页面显示剩余次数不代表可以跳过注册时的再次校验。

## 验证

当前目录没有独立 Vitest。从 `frontend/` 运行 `pnpm run typecheck` 并补充 CRUD、次数上限、过期和注册消费的相关回归；不要把未存在的 feature 测试当成已覆盖。
