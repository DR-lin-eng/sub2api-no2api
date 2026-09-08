# Activity Center

本 feature 同时持有用户活动列表、详情、参与记录和管理员活动编辑、记录组件。用户入口为 `/activity-center`，管理员入口为 `/admin/activity-center/campaigns`；记录组件的实际挂载以路由和页面引用为准。

## 代码归属

- [data/datasources](data/datasources/)：用户活动、抽奖、签到、兑换、记录与管理员 CRUD 请求。
- [presentation/pages](presentation/pages/)：活动加载、编辑、参与和结果展示。
- [presentation/activityHtml.ts](presentation/activityHtml.ts)：活动 HTML 展示清洗。
- [presentation/checkinCalendar.ts](presentation/checkinCalendar.ts)：签到日历展示转换。
- [presentation/widgets/ActivityCardCodeDialog.vue](presentation/widgets/ActivityCardCodeDialog.vue)：中奖卡密结果展示。

## 功能边界

`activity_center_enabled` 控制用户活动访问，后端仍校验时间窗、分组资格、库存、每日限制和重复签到。前端动画与中奖结果展示不决定奖励。类型、配置和奖励流程见 [活动专题](../../../../docs/ACTIVITY_CENTER.md)；后端规则由 [activitycenter](../../../../backend/internal/modules/activitycenter/README.md) 持有。

## 验证

从 `frontend/` 执行 `pnpm exec vitest run src/features/activity-center`。重点覆盖活动详情、管理员编辑、HTML 清洗和用户 DTO。修改抽奖或签到字段还要运行后端领域、奖励与事务测试。
