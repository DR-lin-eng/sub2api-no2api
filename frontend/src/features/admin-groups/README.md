# Admin Groups

管理分组 feature 负责分组列表、创建/编辑、排序及定价和路由辅助对话框。

- `data/datasources/`: 分组协议、CRUD 和辅助配置 API。
- `presentation/pages/`: 列表查询、分页、筛选和对话框编排。
- `presentation/widgets/`: 共享编辑器、领域字段和辅助对话框。
- `presentation/composables/`: 可复用交互状态与纯转换。

创建与编辑使用共享的静态编辑器，但继续保留独立 form、watcher 和提交 payload；模式差异通过 typed context 或显式 props/actions 表达。不要复制整套编辑模板，不要用动态组件改变路由 chunk、挂载时机或表单状态。

分组列表的今日、昨日和累计费用使用服务端配置时区的统一口径。datasource 不发送浏览器时区；页面必须兼容滚动升级期间旧节点暂未返回 `yesterday_cost` 的情况，并将缺失值显示为 0。

共享编辑器的 `BaseDialog`、`form` 和 footer 由 `GroupEditorDialog.vue` 持有；核心/订阅、Antigravity、Anthropic/OpenAI、账号过滤与模型路由字段分别由同目录静态 widget 展示。子 widget 只消费既有 `groupEditorContext`，不得新增请求、watcher 或第二份表单状态。

验证入口：

```sh
pnpm exec vitest run src/features/admin-groups
pnpm run typecheck
```
