# Admin Cluster

本 feature 提供 `/admin/multi-instance` 的节点状态、负载、共享任务与滚动发布界面。部署前提和运维流程见 [多实例部署](../../../../deploy/MULTI_INSTANCE.md)。

- [data/datasources/adminClusterDatasource.ts](data/datasources/adminClusterDatasource.ts)：集群状态、节点改名及发布任务的创建、暂停、恢复、取消、确认、目标重试。
- [presentation/pages/MultiInstancePage.vue](presentation/pages/MultiInstancePage.vue)：加载、刷新与操作编排。
- [presentation/widgets](presentation/widgets/)：节点负载、汇总和版本发布面板。
- [presentation/clusterLocale.ts](presentation/clusterLocale.ts)：节点、任务、发布状态的显式文案映射。

节点 `node_id` 与进程 `runner_id` 不是同一标识；进程重启不应被解释为新增逻辑节点。节点状态、发布任务状态和单个目标状态也不能共用一个枚举。创建、取消和最终确认发布的后端路由要求 step-up，页面按钮不是权限事实源。

## 验证

从 `frontend/` 执行 `pnpm exec vitest run src/features/admin-cluster`。真实版本切换另需多节点环境验证，不能仅凭页面测试推断部署成功。
