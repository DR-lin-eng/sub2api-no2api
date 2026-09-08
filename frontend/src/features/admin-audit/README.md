# Admin Audit

本 feature 提供 `/admin/audit-logs` 的管理操作审计查询，不是 Prompt 内容审计或网关用量日志。

- [data/datasources/adminAuditDatasource.ts](data/datasources/adminAuditDatasource.ts)：分页筛选、单条详情与清空协议。
- [presentation/pages/AuditLogPage.vue](presentation/pages/AuditLogPage.vue)：查询、详情和清空交互。
- 后端事实源：[管理路由](../../../../backend/internal/transport/http/server/routes/admin.go) 的 `registerAuditLogRoutes`，以及 middleware、service、repository 的 `audit_log*` 实现。

管理路由在认证后记录变更操作及敏感读取。清空接口要求请求体中的现场 `totp_code`，不以已有 step-up 窗口替代；界面应以服务端结果为准。记录中的脱敏字段不能被前端恢复成原始凭据。

## 验证

当前目录没有独立 Vitest。修改时从 `frontend/` 运行 `pnpm run typecheck` 与 `pnpm exec vitest run src/core/routes/__tests__/adminRouteAccess.spec.ts`，并补充列表、详情、TOTP 错误及成功后的刷新测试。权限与清理动作另需后端审计相关测试。
