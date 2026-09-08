# Affiliate

本 feature 同时持有 `/affiliate` 用户邀请返利页，以及 `/admin/affiliates/invites`、`rebates`、`transfers` 管理记录页。

- [presentation/pages](presentation/pages/)：邀请链接、返利额度、邀请记录、转入余额及管理员筛选。
- [data/datasources/adminAffiliatesDatasource.ts](data/datasources/adminAffiliatesDatasource.ts)：管理员专属设置、返利率和记录查询。
- 用户详情与额度转入请求当前归属 [profileDatasource.ts](../profile/data/datasources/profileDatasource.ts) 的 `getAffiliateDetail`、`transferAffiliateQuota`，不是本目录另建的用户 datasource。
- 后端规则来自 [affiliate_service.go](../../../../backend/internal/application/service/affiliate_service.go) 和对应 repository。

返利额度转入账户余额不等于现金提现。生效返利率、可转入额度和最终余额以服务端返回为准，转入成功后再刷新用户信息与记录；不要在前端自行累加返利作为账务事实。

从 `frontend/` 执行 `pnpm exec vitest run src/features/affiliate`。费率、幂等或账务变更还需 affiliate service/repository 回归。
