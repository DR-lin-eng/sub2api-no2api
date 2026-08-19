# Admin Accounts

管理账号 feature 负责账号列表、创建/编辑、批量更新、授权和用量展示。

- `data/dtos/adminAccountDtos.ts`: 账号创建/更新、混合渠道预检、Codex 导入、模型和临时不可调度状态等账号专属协议类型；`src/types` 仅保留兼容转发。
- `data/datasources/adminAccountQueries.ts`: 列表、ETag、详情、摘要、统计、用量、临时不可调度状态、Ollama Cloud 状态、模型、Antigravity 默认映射和上游计费设置/快照查询。
- `data/datasources/adminAccountActions.ts`: 账号创建/更新、OAuth 凭据应用/错误清理、Codex session/PAT 导入、上游模型同步、CPA 测试、页面批量操作、导出、账号状态维护、重复账号、上游计费探测、额度查询和 Ollama Cloud 配置动作。
- `data/datasources/adminAccountOAuthActions.ts`: 通用账号与 OpenAI OAuth 授权 URL、code exchange、cookie 授权和 refresh token 动作；旧 `accountsAPI` 继续兼容转发。
- `data/datasources/adminAccountsDatasource.ts`: 旧 `accountsAPI` 兼容聚合与尚未迁移的账号操作。
- `data/datasources/scheduledTestsDatasource.ts`: 账号定时测试计划、启停、删除和结果查询。
- `presentation/pages/`: 列表查询、筛选、刷新和对话框编排。
- `presentation/widgets/create/`: 创建表单的领域字段。
- `presentation/widgets/edit/`: 编辑表单的领域字段。
- `presentation/composables/`: 有界的表单策略、OAuth 与提交编排。

账号列表页保留路由、请求和弹窗编排，表格 DOM 由 `AccountsTableView.vue` 静态承载。列表列偏好、展示映射、今日统计和上游额度分别由同域 composable 管理；表格只消费 `accountTableViewContext.ts` 的 typed context，不直接请求 API 或创建 watcher、timer。`oauth_quota=exhausted` 在服务端分页前筛选已持久化的 OAuth 用量快照（任一已知窗口达到 100%）；缺少快照的账号保持未知，不会被误判为耗尽。

请求生命周期由 presentation 明确持有：账号页负责列表 AbortController、ETag、筛选和写后刷新；今日统计 composable 用请求序号忽略过期响应；上游计费 composable 负责费率 ETag、轮询暂停条件、探测后的列表刷新和额度缓存失效。重复账号 Action 在请求成功前将幂等键同时保存在内存与 `sessionStorage`，失败重试和页面重载继续复用同一个键。

当前接口 owner 盘点：列表、详情、摘要、统计、用量和只读快照属于 Query；创建、更新、删除、授权、批量操作、导入导出与主动探测属于 Action；账号专属请求/响应类型属于 `data/dtos/adminAccountDtos.ts`。迁移按调用链分片推进，兼容 `accountsAPI` 在所有旧消费者迁完前保持相同方法和响应形状，兼容 `@/types` 继续转发已迁移类型。账号页和批量编辑对话框直接依赖 Query/Action owner，不再经过统一 admin barrel。

账号额度通知的账号级阈值仍由 `useQuotaNotifyState.ts` 管理；其中全局启用状态直接读取 `admin-settings` datasource，失败时保持关闭，不再通过统一 admin barrel 跨域访问。

定时测试面板直接依赖 `scheduledTestsDatasource.ts`，由面板继续持有打开时加载、创建后刷新、局部更新、删除和结果展开状态。

OAuth composable 和创建账号的 OAuth 兑换编排直接依赖账号 OAuth Action、账号 Action owner 或对应平台 datasource。授权 URL、state/session、代理参数、code exchange、cookie 授权、refresh token 和旧服务器 capabilities fallback 的请求语义保留在 feature data；composable 继续拥有 loading、错误反馈和凭据组装。

两个重新授权对话框直接依赖账号 Action 与 OAuth Action owner：普通对话框保留 `update + clear-error` 的原有写入顺序，管理员对话框保留 `apply-oauth-credentials` 的增量 extra 合并和服务端 token 缓存失效语义；成功后仍分别发出原有 `reauthorized` 事件并关闭对话框。

创建和编辑对话框拥有各自的 reactive form，并在 setup 中同步装配 watcher、授权和提交 composable。字段组件只通过明确的 typed context 读写表单与触发动作，不持有 API 或 Store。

创建/编辑链路直接依赖账号 Query/Action owner：创建、更新、混合渠道预检、Codex 导入、上游模型同步、CPA 测试和创建后主动计费探测均不再经过统一 admin barrel。Web Search 全局开关与 TLS 指纹 profile 列表继续由 `admin-settings` datasource 所有。旧 `accountsAPI` 保持同名函数身份，供旧导入和其他兼容消费者使用；数据导入与 CRS 预览/同步的运行时调用已直接进入明确 owner。

- `accountEditorContext.ts`: 字段组件的最小类型契约。
- `accountFormPolicy.ts`: 创建和编辑共用的纯表单转换。
- `useCreateAccountEditorPolicy.ts`: 创建表单 watcher 与字段动作。
- `useCreateAccountOAuthActions.ts`: 创建账号的 OAuth exchange/import/batch 流程。
- `accountEditUpdatePayload.ts`: 按账号类型和平台构造编辑 payload。
- `useEditAccountSubmission.ts`: 编辑校验、风险确认与更新请求编排。
- `useCPATestConnection.ts`: CPA 未保存表单的连接测试、策略参数和反馈状态。
- `useAccountsUpstreamBilling.ts`: 上游计费探测、额度缓存、批量查询和定时刷新。
- `useAccountColumnPreferences.ts`: 列可见性迁移与服务端派生查询参数。
- `useAccountTodayStats.ts`: 当前页今日统计的请求并发保护。
- `useAccountTablePresentation.ts`: 纯列定义、徽标和单元格格式化。
- `BulkEditCodexThinkingTagOption.vue`: 批量更新 Codex thinking tag 规范化开关，保持批量编辑主对话框在维护上限内。

新增平台字段时同步检查创建、编辑和批量更新 payload。不要复制无边界表单、把完整 Pinia Store 传入字段组件，或把控制器重新堆回单一 SFC。运行时代码保持在 1500 行以内，新增职责应进入现有有界组件或 composable。

验证入口：

```sh
pnpm exec vitest run src/features/admin-accounts
pnpm run typecheck
```
