# Admin Users

管理员用户 feature 负责用户列表、属性、分组、余额、平台额度和批量限制。

- `data/dtos/`: 用户身份绑定、批量限制、余额历史和平台额度传输协议。
- `data/datasources/`: 用户管理与属性请求；`adminUsersDatasource.ts` 继续提供 `usersAPI` 兼容出口。
- `presentation/pages/`: 主列表请求、选择状态、二级数据加载和对话框编排。
- `presentation/widgets/`: 表格工具栏及各领域编辑对话框。

`admin-users` presentation 直接依赖用户、属性、分组、用量和 API Key 的明确 datasource owner，不使用 `@/api/admin`。`UsersPage.vue` 继续持有 AbortController、300ms 搜索防抖、50ms 二级数据延迟、localStorage 偏好和所有 API 调用。工具栏通过 props/emits 同步筛选与列设置，不自行发请求。新增列表字段时同时检查列可见性、批量 secondary-data 条件和旧偏好迁移。

共享分组选择组件必须同时通过用户与管理员路由的中英文词表 scope 验证，禁止把 `admin.*` 翻译键用于普通用户只加载 `user` scope 的界面。

验证入口：

```sh
pnpm exec vitest run src/features/admin-users
pnpm exec vitest run src/common/widgets/data/__tests__/GroupOptionItem.spec.ts src/core/i18n/__tests__/routeLocaleCoverage.spec.ts
pnpm run typecheck
```
