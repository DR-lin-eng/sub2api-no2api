# Prompt Audit

本 feature 提供 `/admin/prompt-audit` 的 Prompt 审计策略、端点池、运行状态和事件工作区。它独立于 [操作审计](../admin-audit/README.md) 与 [内容风控](../admin-risk-control/README.md)。

- [data/datasources/promptAuditDatasource.ts](data/datasources/promptAuditDatasource.ts)：配置、探测、runtime、事件详情和删除协议。
- [domain](domain/)：审计类型、视图模型与纯转换。
- [presentation/widgets](presentation/widgets/)：PolicyPanel、EndpointPool、RuntimeOverview、事件详情和筛选删除。
- [presentation/pages/PromptAuditPage.vue](presentation/pages/PromptAuditPage.vue)：查询和动作编排。

后端 owner 是 [securityaudit](../../../../backend/internal/modules/securityaudit/README.md)。同步防护与异步审计有不同的等待/失败语义，保存配置不代表端点探测已经成功。按筛选删除应保留预览与实际删除的区分；事件原文和敏感字段遵守服务端返回边界。

从 `frontend/` 执行 `pnpm exec vitest run src/features/prompt-audit`。策略或队列字段变化还需后端模块回归及 [路由覆盖测试](../../../../backend/internal/transport/http/server/routes/prompt_audit_route_coverage_test.go)。
