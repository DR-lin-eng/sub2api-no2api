# Admin Proxies

管理代理 feature 负责代理列表、创建和编辑、连接与质量检测、自动均衡分配、失活迁移、批量操作及数据导入导出。

- `data/datasources/`: 代理管理 API 与请求协议。
- `presentation/pages/`: 路由级查询、取消、搜索防抖、批量并发和弹窗编排。
- `presentation/widgets/ProxyTable.vue`: 表格渲染与同步交互转发。
- `presentation/widgets/CreateProxyDialog.vue`: 标准和批量创建表单。
- `presentation/widgets/EditProxyDialog.vue`: 编辑表单及密码脏状态输入。
- `presentation/widgets/ProxyAutoAssignmentDialog.vue`: 强制自动分配、定期测活、失败阈值和手动均衡设置。
- `presentation/widgets/ProxyPageDialogs.vue`: 删除、导入导出、质量报告和关联账号弹窗。
- `presentation/proxyPageContext.ts`: 页面与领域 widget 之间的最小类型契约。

页面是 API 请求和生命周期的唯一 owner。Widget 通过 typed context 读写页面局部状态并调用页面动作，不创建 Store、API client、watcher 或动态 import；所有 widget 保持静态导入，因此仍随代理路由一次加载。

修改查询或批量流程时，必须保留请求取消、300ms 搜索防抖、并发上限、选择状态和刷新时机。修改编辑器时同步检查表单 ID、密码脏状态、有效期换算及创建/更新 payload。

验证入口：

```sh
pnpm exec vitest run src/features/admin-proxies
pnpm exec eslint src/features/admin-proxies --ext .ts,.vue
pnpm run typecheck
```
