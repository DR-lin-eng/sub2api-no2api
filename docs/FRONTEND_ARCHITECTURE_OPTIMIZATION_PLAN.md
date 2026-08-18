# 前端架构优化计划

> 状态：阶段性完成。阶段 1 的渐进式架构门禁已于 2026-08-03 落地，阶段 2 的 `admin-accounts` 试点已于 2026-08-04 收口，阶段 3 的复杂管理域已全部完成；阶段 4 已完成 `auth` 与 `profile`，下一项为 `billing` 与 `subscriptions` 用户域迁移；请求方法与 CDN 缓存边界专项待实施。
>
> 基线日期：2026-08-03。后续迁移批次开始前必须重新统计代码和依赖，源码与测试始终是最终事实来源。

## 1. 背景

当前前端已经采用 `core`、`common` 和 `features/<domain>` 的 feature-first 垂直切片，主依赖方向为：

```text
main / core routes
  -> feature presentation
  -> feature data datasource
  -> core networks
  -> backend
```

现有方向是正确的，不需要推倒重建。但迁移尚未收口，仍存在以下维护问题：

- 运行时代码仍通过 `@/api`、`@/api/admin` 和 `@/stores` 兼容 barrel 间接访问 owner。
- 部分 datasource 同时包含大量查询、写操作、协议类型和兼容逻辑，职责过宽。
- `src/types/` 保存了不少实际只属于单一 feature 的请求、响应和业务类型。
- 少数 feature 直接依赖其他 feature 的私有 presentation 或 datasource 实现。
- 复杂页面的加载、取消、缓存、刷新、错误和写后刷新语义缺少统一的职责约定。
- 浏览器请求出口尚未通过类型和静态门禁限制 HTTP 方法；如果后续代码把 UI 输入、配置或服务端数据透传为请求方法，可能使用非预期方法绕过 CDN 缓存并直接放大源站流量。

历史上的大型页面已通过 page、widget、composable 和 datasource 拆分显著缩小，说明继续沿现有 feature owner 渐进治理比全量 Clean Architecture 重写更适合本项目。

## 2. 目标与非目标

### 2.1 目标

- 完成现有 feature-first 架构迁移，消除长期兼容 barrel。
- 让 API DTO、查询、写操作和复杂领域规则具有明确 owner。
- 限制跨 feature 私有依赖，形成可检查的依赖方向。
- 保持请求生命周期、鉴权、缓存、分包和页面行为兼容。
- 将前端业务请求收口到端点契约声明的方法，并与后端、CDN/WAF 的逐路由方法白名单形成纵深防护。
- 让后续二次开发可以从 feature 入口快速定位协议、规则、状态和 UI。

### 2.2 非目标

- 不一次性重写整个前端。
- 不要求每个 feature 都建立 Domain、Repository、UseCase 和 Mapper。
- 不在架构迁移中同时重新设计 UI 或修改后端 API。
- 不为了目录形式机械复制一对一 DTO、Entity 和 ViewModel。
- 不预先引入 TanStack Query 等新运行时框架；先解决 owner 和生命周期问题。
- 不通过全局 Store、动态组件或新 barrel 转移局部复杂度。

## 3. 基线

2026-08-03 的刷新扫描结果如下。数字用于确定优先级，后续批次开始前仍需刷新：

| 指标 | 当前基线 |
| --- | ---: |
| `features/<domain>` 数量 | 35 |
| feature 运行时与测试 TypeScript/Vue 文件 | 637 |
| feature datasource 文件 | 50 |
| feature presentation 文件 | 415 |
| 运行时兼容 barrel 引用 | 120 个文件 / 126 条 import |
| 跨 feature 私有 presentation 引用 | 46 个文件 / 74 条 import |
| 超过 500 有效行的 datasource | 4 |

修改前基线验证发现 `AccountsPage.vue` 为 1568 个物理行，超过专项测试的 1550 行过渡目标。本批已将排序偏好解析提取到同 feature 的 `accountSortState.ts`，恢复该专项门禁且未改变页面请求生命周期。

首批高收益对象：

| Feature | 主要问题 | 优先方向 |
| --- | --- | --- |
| `admin-accounts` | 协议面大，查询、ETag、授权、批量操作和导入导出混合 | DTO owner、Query/Action、纯表单策略 |
| `admin-settings` | datasource 和设置协议体量大，多个设置子域共享保存流程 | 按设置子域拆协议和保存 Action |
| `admin-ops` | 读取接口多，快照、日志、指标和错误详情混合 | 按查询资源拆分，不制造空 Action 层 |
| `admin-usage` | 直接组合其他管理 feature 的私有 UI 和数据实现 | 明确组合边界和共享契约 |
| `admin-orders` / `billing` | 共享支付类型和 presentation formatter | 提取稳定的 payment 契约与展示工具 owner |

## 4. 架构决策

### 4.1 保留 feature-first 主结构

默认结构保持简单：

```text
features/<domain>/
  data/
    datasources/
  presentation/
    pages/
    widgets/
    composables/
    stores/
```

只有满足后续条件时才增加更细层级。

### 4.2 Domain 层按需建立

满足下列任一条件时，可以建立 `domain/`：

- 同一业务规则被两个以上页面、widget 或 composable 使用。
- 存在值得独立单元测试的状态转换、策略、校验或计算。
- 后端 DTO 与编辑草稿、展示模型之间存在明显语义差异。
- 规则必须保持框架无关，不能依赖 Vue、Pinia、Router、Axios 或浏览器状态。

简单 CRUD、只读列表和一次性表单不建立空洞 Domain 层。

### 4.3 DTO 放回 API owner

- 请求和响应类型默认放在所属 feature 的 `data/`。
- DTO 保留后端传输字段和可选字段语义，包括 snake_case 与滚动升级兼容字段。
- 真正跨多个 feature 的稳定协议才保留在 `src/types/` 或明确的共享契约目录。
- 只有 API、domain、编辑草稿或 UI 模型形态不同时才建立 mapper。
- mapper 必须是显式纯函数，并覆盖空值、旧响应和敏感字段测试。

### 4.4 Query/Action 按职责拆分

以下情况需要物理拆分 Query 与 Action：

- 单个 datasource 超过约 500 行且同时包含大量读写接口。
- 查询具有分页、筛选、取消、ETag、缓存或轮询语义。
- 写操作具有幂等键、确认、批量执行、进度或写后失效语义。
- 同一 datasource 已难以确定某次修改影响哪些页面。

约定如下：

- Query 只负责读取，接收明确参数和可选 `AbortSignal`，不显示 toast、不跳转路由。
- Action 负责创建、更新、删除、批量操作和命令式探测，不直接操作 presentation state。
- 写后刷新、失效和用户反馈由拥有请求生命周期的 page、composable 或 feature Store 编排。
- 简单 feature 可以继续保留一个小型 datasource，不强制拆成多个单函数文件。

推荐的复杂 feature 结构：

```text
features/<domain>/
  domain/
    models.ts
    policies.ts
  data/
    <domain>Dto.ts
    <domain>Queries.ts
    <domain>Actions.ts
    <domain>Mapper.ts       # 仅在确有转换时存在
  presentation/
    pages/
    widgets/
    composables/
    stores/
```

### 4.5 请求方法白名单与 CDN 缓存边界

- 前端业务请求只允许使用端点契约中静态声明的 `GET`、`POST`、`PUT`、`PATCH` 和 `DELETE`；浏览器自动发起的 CORS `OPTIONS` 预检不属于业务代码可选方法。
- datasource 优先使用统一客户端的 `.get/.post/.put/.patch/.delete` 接口。SSE、流式上传下载、网关探测等必须直接使用 `fetch` 的例外，方法也必须是源码中的字面量，并登记路径、方法和不能复用统一客户端的原因。
- 禁止将表单字段、URL 参数、LocalStorage、远端响应或其他运行时字符串写入 `method`；禁止业务代码使用通用 `request({ method })` 逃逸类型约束。
- 禁止 `X-HTTP-Method-Override`、`X-Method-Override`、`X-HTTP-Method`、`_method` 等方法覆盖头或参数；禁止 `CONNECT`、`TRACE`、`TRACK`、`PURGE`、`BAN`、大小写变体和自定义 HTTP 动词。
- CDN 只允许静态资源和明确公开资源的 `GET/HEAD` 参与缓存；认证 API、管理 API、写请求、SSE 和 WebSocket 必须沿用其明确的旁路或 `private/no-store` 策略，不能靠更换方法隐式控制是否命中缓存。
- 前端白名单只防止产品代码误发请求，不是安全边界。后端路由必须拒绝不匹配的方法并返回 `405`，CDN/WAF 应在到达源站前按路径拒绝非白名单方法；CORS 只开放确有需要的方法和请求头。
- 方法门禁不得改变现有端点契约。若后端确需新增方法，必须先更新路由、CDN/WAF 规则、前端 datasource 类型和负向测试，再允许调用。

## 5. 实施阶段

### 阶段 0：刷新基线和保护现场

- [x] 重新统计 feature、旧 barrel 引用、跨 feature import 和大型 datasource。
- [x] 检查工作区已有修改，避免与正在进行的业务功能并行重写同一文件。
- [ ] 记录关键页面的请求数、请求时序、Abort、ETag、轮询和写后刷新行为。
- [x] 为本计划建立分 feature 验收清单，所有架构提交只覆盖一个 owner 或明确子域。

每个 feature 开始迁移前，验收清单至少记录：owner 与消费者、请求/响应 DTO、Query/Action 清单、Abort/ETag/轮询/幂等语义、写后刷新时机、路由 chunk、相邻测试、全量验证结果和独立回滚点。

完成条件：可以准确回答每个迁移对象的 owner、消费者、协议、测试和兼容行为。

### 阶段 1：建立渐进式架构门禁

- [x] 禁止新代码导入 `@/api`、`@/api/admin` 和 `@/stores`。
- [x] 为现有引用建立精确基线；每迁移一个 feature 就缩小基线，不扩大 allowlist。
- [x] 强化 `domain -> data -> presentation` 反向依赖限制。
- [x] 禁止新增跨 feature 的私有 presentation 导入。
- [x] 保持 1500 有效行硬门禁，并检查 datasource、composable 和普通 `.ts` 文件。
- [x] 更新 `frontend/README.md`、`frontend/AGENTS.md` 和 ESLint 规则说明。

实现入口是 `frontend/eslint/architecture-boundaries.cjs`，存量逐条基线是 `frontend/eslint/architecture-debt-baseline.cjs`。基线以“文件 + import source + 次数”为粒度；旧引用减少但未同步删除基线时，lint 同样失败。

完成条件：架构债务数量只能下降，不能被新代码继续扩大。

### 阶段 1A：收口请求方法和 CDN 边界

- [ ] 生成前端请求清单，记录路径、静态 HTTP 方法、调用 owner、是否携带凭据、是否允许 CDN 缓存以及直接 `fetch` 的必要性。
- [ ] 为统一客户端和直接 `fetch` 例外建立 HTTP 方法字面量门禁，拒绝动态 `method`、通用 `request({ method })`、方法覆盖头/参数和未登记的原生请求出口。
- [ ] 对静态资源、公开读取、认证/管理 API、SSE、WebSocket 和上传下载分别建立 CDN/WAF 方法矩阵；仅让明确的公开 `GET/HEAD` 进入缓存判定。
- [ ] 后端按已注册路由返回 `405` 并带准确的 `Allow`，边缘层在源站前拒绝 `CONNECT/TRACE/TRACK/PURGE/BAN`、自定义方法、非规范大小写和未授权的 `OPTIONS`。
- [ ] 增加负向验证：特殊方法、方法覆盖头/参数和动态方法均不能到达业务 handler；合法的 GET、写请求、SSE、WebSocket、上传下载及浏览器预检保持兼容。
- [ ] 使用 CDN 请求日志或隔离代理验证命中/旁路行为，确认非白名单方法不会形成缓存穿透或源站放大路径。

完成条件：前端运行时代码不存在动态 HTTP 方法和未登记请求出口，后端与边缘方法矩阵一致，特殊方法在业务 handler 之前被拒绝，现有合法请求路径无回归。

### 阶段 2：试点 `admin-accounts`

开始前必须先确认当前账号管理功能修改已经稳定，不能覆盖或回退已有改动。

- [x] 盘点账号列表、详情、统计、用量、授权、导入导出和批量接口。
- [x] 将账号专属 DTO 从 `src/types/gateway.ts` 迁回 feature data。
- [x] 拆分账号 Query：列表、ETag、详情、统计、用量和只读探测。
- [x] 拆分账号 Action：创建、更新、删除、授权、批量和导入导出。
- [x] 保留重复操作幂等键、AbortController、ETag 和额度刷新行为。
- [x] 将复杂表单校验、payload 构造和账号类型策略保留为纯函数。
- [x] 替换该 feature 对 `adminAPI.accounts` 的兼容调用。
- [x] 补充 DTO 兼容、Query 参数、Action payload 和页面请求生命周期测试。

已完成第一切片：列表/ETag、详情摘要、今日统计和上游费率快照迁入 Query owner；重复账号、上游计费探测和额度主动查询迁入 Action owner。旧 datasource 继续提供同名导出与 `accountsAPI`，今日统计和上游计费 composable 已移除统一 admin barrel 依赖，并同步缩小精确债务基线。

已完成第二切片：账号页使用的模型/探测设置查询、批量操作、导出和账号状态维护进入对应 owner；页面对账号、代理和分组的请求均改为直接 owner import，页面本身不再依赖 `adminAPI`，兼容 datasource 继续服务尚未迁移的 widgets/composables。

已完成第三切片：账号统计、用量和临时不可调度状态查询进入 Query owner；两个统计弹窗、用量单元格和临时状态弹窗改为直接依赖 Query/Action owner，兼容 datasource 保持同名导出与 `accountsAPI` 函数身份。

已完成第四切片：Grok 额度探测组件直接依赖平台 datasource；Ollama Cloud 状态读取与配置动作分别进入账号 Query/Action owner，编辑 widget 不再经过统一 admin barrel，兼容 datasource 继续提供原有方法。

已完成第五切片：账号连接测试的两个弹窗直接依赖账号 Query 的可用模型查询；SSE 测试请求、Authorization、Abort 和事件解析保持原有实现，兼容 datasource 继续提供原方法。

已完成第六切片：批量编辑对话框直接依赖账号 Action 的混合渠道风险检查和批量更新；筛选目标与所选账号两种 payload、409 确认回退和写后刷新时机保持不变，兼容 datasource 继续提供同名方法。

已完成第七切片：账号额度通知 composable 直接依赖 `admin-settings` 的设置查询 owner；全局开关异步加载、失败关闭和账号 extra 阈值写入语义保持不变，不再通过统一 admin barrel 访问其他 feature。

已完成第八切片：定时测试面板直接依赖 `scheduledTestsDatasource` 的计划与结果接口；打开时加载、创建后刷新、局部启停/编辑、删除和结果展开时序保持不变，不再通过统一 admin barrel 访问同域 owner。

已完成第九切片：通用账号和 OpenAI OAuth 请求进入 `adminAccountOAuthActions`，Gemini、Antigravity 和 Grok OAuth composable 直接依赖各自平台 datasource，创建账号的 OAuth 兑换编排也直接调用新 owner；授权 URL、state/session、代理参数、code exchange、cookie 授权、refresh token 和旧服务器 capabilities fallback 保持原语义。旧 `accountsAPI` 继续导出相同函数身份，5 个 OAuth composable 不再经过统一 admin barrel。

已完成第十切片：`ReAuthAccountDialog.vue` 与 `AdminReAuthAccountDialog.vue` 直接依赖账号 Action/OAuth Action owner；普通重新授权继续按 `update -> clear-error` 顺序写入，管理员流程继续使用 `apply-oauth-credentials` 增量合并 extra、清理错误并失效服务端 token cache。各平台 code exchange、cookie 授权、代理参数、凭据转换、成功事件与关闭时机保持不变，兼容 `accountsAPI` 继续导出相同函数身份。

已完成第十一切片：创建、更新、混合渠道预检、Codex session/PAT 导入、上游模型同步和 CPA 测试进入账号 Action owner，Antigravity 默认映射进入 Query owner；`CreateAccountDialog.vue`、`EditAccountDialog.vue` 及其创建 OAuth/编辑提交编排直接依赖明确 owner。Web Search 全局开关与 TLS 指纹 profile 继续由 `admin-settings` datasource 所有，表单 watcher、风险确认、敏感字段 payload、创建后主动计费探测、事件和关闭时机保持不变，兼容 `accountsAPI` 保持相同函数身份。

已完成第十二切片：CRS 预览进入账号 Query owner，数据导入和 CRS 同步进入账号 Action owner；`ImportDataDialog.vue` 与 `SyncFromCrsDialog.vue` 不再经过统一 admin barrel。多文件合并、逐文件头校验、导入结果统计、默认分组绑定字段、部分成功后的关闭刷新、CRS 自动选择/手动取消、代理同步选项、成功事件和 `180000ms` timeout 保持不变，`admin-accounts` 运行时 `@/api/admin` 引用归零。

已完成第十三切片：`ClaudeModel`、临时不可调度状态、账号创建/更新、混合渠道预检和 Codex session/PAT 导入 DTO 迁入 `admin-accounts/data/dtos/adminAccountDtos.ts`；Query、Action、账号页、创建/编辑编排和连接测试组件直接依赖 feature owner。`src/types/gateway.ts` 不再声明这些账号专属类型，`@/types` 保留兼容转发，未改变 API 字段或请求 payload。

停止条件：如果拆分产生大量一对一包装、测试替身显著膨胀或调用链更难追踪，退回 `presentation -> queries/actions -> network` 三层，不继续增加 Application/Repository 抽象。

### 阶段 3：迁移复杂管理域

- [x] `admin-settings` 按设置子域拆 DTO、加载 Query 和保存 Action。
- [x] `admin-ops` 按 snapshot、日志、错误详情、指标、告警、设置和 WebSocket 拆分明确 owner。
- [x] `admin-users` 迁移用户管理专属 DTO 和旧 admin barrel 调用。
- [x] `admin-groups` 迁移分组、组合路由和倍率协议 owner。
- [x] `admin-usage` 消除对 `admin-users`、`admin-ops` 私有 presentation 的直接依赖。
- [x] `admin-orders` 与 `billing` 提取稳定的 payment 共享契约和格式化能力。

已完成 `admin-settings` 第一切片：邮件模板与面板限流 DTO 迁入 `data/dtos/adminSettingsDtos.ts`，读取进入 `adminSettingsQueries.ts`，写入与预览进入 `adminSettingsActions.ts`；旧 `adminSettingsDatasource.ts` 和 `settingsAPI` 保留同名兼容导出及相同函数身份。邮件模板的路径分段编码、payload、预览和恢复时序，以及面板限流的旧响应归一化保持不变。

同一切片将 `admin-settings` 运行时代码中的 9 条 `@/api`、`@/api/admin` 和 `@/stores` 引用全部迁到明确 owner；管理员支付配置、错误透传、TLS 指纹、用户搜索、合规状态和通知 Store 仍调用原接口与原状态 owner。仓库精确 legacy barrel 基线从 100 条降至 91 条（87 个文件），该 feature 的运行时旧 barrel 引用归零。主设置 DTO、统一加载/保存和其余设置子域仍待后续切片，因此本阶段检查项保持未完成。

已完成 `admin-settings` 第二切片：管理员 API Key、529/429 冷却、全局临时不可调度、流超时、请求修正、Beta Policy、Web Search 和 SMTP 测试协议迁入同一 feature 的 DTO、Query、Action owner。相关 composable 直接调用明确 owner，ID 路径编码、各独立 URL、payload、失败降级和保存后回填时序保持不变；旧 `settingsAPI` 继续以相同函数身份提供兼容出口。

第二切片后，`admin-settings` presentation 仅有主设置的一次 `getSettings` 和一次 `updateSettings` 继续使用兼容 facade；独立设置子域不再通过 `settingsAPI` 调用。`SystemSettings`、`UpdateSettingsRequest`、注册/支付/微信兼容归一化和主设置 Query/Action 留给下一切片，避免把主设置加载/统一保存与独立端点迁移混在同一风险面。

已完成 `admin-settings` 第三切片：`SystemSettings`、`UpdateSettingsRequest`、认证来源默认值、平台限额、支付可见渠道和微信模式兼容归一化迁入 `data/dtos/systemSettingsDtos.ts`；统一主设置读取进入 Query owner，统一保存进入 Action owner。设置页、领域 Store、保存模块和账号创建/编辑相关调用均直接依赖明确 owner，单次主加载、统一保存、step-up 重试、保存后公开设置刷新和本地缓存回填顺序保持不变。

第三切片后，`adminSettingsDatasource.ts` 缩为 89 行的纯兼容 facade，不再拥有 `apiClient`、DTO 或归一化实现；运行时代码只有旧 `src/api/admin/index.ts` 仍导入该路径。结构测试锁定主 DTO 的无网络依赖、Query/Action 的请求所有权、presentation 对 facade 的零引用及 `settingsAPI` 函数身份，因此 `admin-settings` 阶段项完成。

已完成 `admin-ops` 第一切片：overview、`snapshot-v2`、吞吐/切换趋势、延迟分布和错误趋势/分布协议迁入 `opsDashboardDtos.ts`，对应请求进入 `opsDashboardQueries.ts`；图片生成、OpenAI Token、用户用量、并发、账号可用性和实时流量摘要协议与请求分别进入 `opsMetricsDtos.ts` 和 `opsMetricsQueries.ts`。

仪表盘页面、趋势组件、指标卡、并发卡和实时摘要 composable 已直接依赖明确 owner，`include_*` 查询裁剪、混合版本 fallback、AbortSignal、筛选参数、刷新触发和静态路由 chunk 保持不变。旧 `opsAPI` 继续以相同函数身份提供兼容出口，`adminOpsDatasource.ts` 从 1537 行降至 922 行；系统/请求日志、错误详情、告警和设置仍待后续切片，因此 `admin-ops` 阶段项保持未完成。

已完成 `admin-ops` 第二切片：请求明细、系统日志、运行时日志配置、清理 payload 与 sink health 协议迁入 `opsLogDtos.ts`；四个只读请求进入 `opsLogQueries.ts`，配置保存/重置和日志清理进入 `opsLogActions.ts`。请求明细和系统日志组件直接依赖新 owner，TTFT preset、分页/时间范围/筛选、`redis_only` 回填、保存/重置时序、显式 `clear_all` 与筛选清理语义保持不变。

第二切片后，7 个兼容 `opsAPI` 方法继续保持相同函数身份，`adminOpsDatasource.ts` 从 922 行降至 764 行。错误列表与详情、告警、设置和 WebSocket 订阅仍待后续切片，因此 `admin-ops` 阶段项继续保持未完成。

已完成 `admin-ops` 第三切片：统一错误与 request/upstream 拆分错误的列表、详情、关联上游详情协议迁入 `opsErrorDtos.ts`，七个只读请求进入 `opsErrorQueries.ts`，三个 resolved 操作进入 `opsErrorActions.ts`。错误列表和详情组件直接依赖新 owner；`admin-usage` 继续读取 legacy unified endpoint，但改为直接依赖公开 Error Query/DTO owner。

第三切片后，10 个兼容 `opsAPI` 方法继续保持相同函数身份，`view=errors|excluded|all`、分页与筛选参数、request/upstream 路径、关联查询的 `include_detail=1` 和 resolved payload 保持不变。`adminOpsDatasource.ts` 从 764 行降至 620 行；告警、设置和 WebSocket 订阅仍待后续切片，因此 `admin-ops` 阶段项继续保持未完成。

已完成 `admin-ops` 第四切片：告警规则/事件协议与请求进入 `opsAlertDtos.ts`、`opsAlertQueries.ts` 和 `opsAlertActions.ts`；通知、运行时、高级设置、统一快照和指标阈值进入 `opsSettingsDtos.ts`、`opsSettingsQueries.ts` 和 `opsSettingsActions.ts`；QPS WebSocket 的子协议鉴权、状态、陈旧检测和重连生命周期进入 `opsRealtimeSubscription.ts`。

告警、设置与仪表盘消费者已直接依赖新 owner，设置快照失败后的四请求 fallback、并行保存顺序、告警筛选与 cursor、事件状态/静默 payload、WebSocket token subprotocol、致命关闭码、陈旧检测、指数退避与离线恢复保持不变。有限 `maxReconnectAttempts` 修正了已上报第 N 次重连却在建连前被拦截的 off-by-one；默认无限重连路径不变。18 个兼容 `opsAPI` 方法继续保持相同函数身份；`adminOpsDatasource.ts` 从 620 行降至 151 行的纯兼容 facade。

同一切片为 `admin-settings` Store 增加稳定公开出口，`admin-ops` 的分组读取直接依赖 `admin-groups` datasource；该 feature 的 `@/api`、`@/api/admin`、`@/stores`、私有跨 feature presentation 和自身兼容 facade 运行时引用归零，仓库 legacy barrel 基线从 83 条降至 78 条。因此 `admin-ops` 阶段项完成。

已完成 `admin-users` 迁移：身份绑定、批量限制、余额历史和平台额度协议进入 `data/dtos/adminUserDtos.ts`，`@/types` 与旧管理员 API barrel 只保留兼容转发；`adminUsersDatasource.ts` 继续提供同名函数与 `usersAPI`，请求路径、参数和 payload 不变。

用户列表与 11 个请求型 widget 已直接依赖用户、属性、分组、仪表盘用量和 API Key 的明确 datasource owner，`admin-users` presentation 中 `@/api/admin` 与 `adminAPI` 引用归零，仓库 legacy barrel 基线从 78 条降至 66 条。AbortController、300ms 搜索防抖、50ms 二级数据延迟、按可见列批量加载、localStorage 偏好、step-up 包装和写后刷新顺序保持不变；结构测试锁定 DTO owner、明确依赖和旧 barrel 零引用，因此 `admin-users` 阶段项完成。

已完成 `admin-groups` 迁移：共享 `Group`、平台、订阅和定价协议进入 `src/types/group.ts`，管理员分组、创建/更新、组合路由、用户倍率/RPM、列表和汇总协议进入 `data/dtos/adminGroupDtos.ts`；读取与写入分别进入 `adminGroupQueries.ts` 和 `adminGroupActions.ts`。`src/types/gateway.ts` 与 74 行旧 datasource 仅保留兼容重导出和同函数身份 facade。

分组页面、编辑器与三个请求型弹窗已直接依赖 Query/Action owner，倍率和 RPM 弹窗的用户搜索直接依赖 `admin-users` datasource；300ms 防抖、列表 AbortSignal、服务端时区汇总、复制幂等键、组合路由请求时序和写后刷新保持不变。全仓库 feature 运行时的 `adminAPI.groups` 与 `adminGroupsDatasource` 引用归零，仅顶层管理员兼容 barrel 保留 facade；legacy barrel 基线从 66 条降至 62 条，因此 `admin-groups` 阶段项完成。

已完成 `admin-usage` 迁移：用量列表、统计、搜索、导出和清理任务直接依赖 `adminUsageDatasource.ts` 的命名函数；用户详情、账号搜索、模型统计与聚合快照分别依赖 `admin-users`、`admin-accounts` 和 `admin-dashboard` 的 datasource owner。页面与筛选器不再经过 `@/api/admin` 或 `adminAPI`，清理弹窗也不再依赖同域 `adminUsageAPI` 对象，兼容对象继续保持原函数身份。

余额历史弹窗通过 `admin-users/userBalanceHistoryDialog.ts` 暴露，错误表格与详情通过 `admin-ops/errorLogTable.ts`、`errorDetailDialog.ts` 暴露；三个文件只导出对应组件，不建立通用 UI barrel。`admin-usage` 对其他 feature 私有 `presentation/` 的三条直接依赖及其架构基线已删除，legacy barrel 基线从 62 条降至 60 条。列表 AbortSignal、精确导出分页、300ms 用户/API Key/账号搜索防抖、用户搜索防陈旧、路由用户回填、模型与快照请求序列、筛选/分页和清理轮询时序保持不变，因此 `admin-usage` 阶段项完成。

已完成 `admin-orders` 与 `billing` 支付共享契约收口：`paymentContracts.ts` 接管原 `src/types/payment.ts` 的支付、订单、套餐、Provider、checkout 和多币种仪表盘协议，旧路径只保留 type-only 兼容转发；`paymentDisplay.ts` 接管币种归一化/格式化、订单状态样式、可退款判定、日期展示和套餐有效期文案，旧 presentation formatter 只保留同函数身份转发。支付方式别名归一化进入 `paymentMethods.ts`，管理端设置和用户支付编排共享同一规则 owner。

管理员支付专属配置、订单筛选与退款协议进入 `adminPaymentDtos.ts`；读取和写入分别进入 `adminPaymentQueries.ts` 与 `adminPaymentActions.ts`。管理页面和设置消费者不再依赖 `adminPaymentAPI`，兼容 facade 保持全部函数身份和 Axios 返回形态。订单状态、订单表格、Provider 弹窗/列表及订阅卡片/Store 通过逐组件公开入口复用，不建立通用 UI barrel；所有 feature 对 `billing/presentation/` 的跨域引用以及 `billing` 对 auth/subscriptions 私有 presentation 的引用归零。跨 feature 私有 presentation 基线从 44 个文件/70 条降至 32 个文件/46 条，legacy barrel 基线保持 60 条，因此阶段 3 完成。

协议回归锁定全部 `/admin/payment/*` 路径、查询参数、payload 和 facade 函数身份；架构回归锁定 payment owner、管理端 presentation 零 facade、跨域私有 UI 零引用。共享订单状态组件使用真实 runtime message 覆盖 `/orders` 与 `/admin/orders` 的用户/管理员权限及中英文组合，并与全路由词表依赖闭包检查一同纳入关键测试集。

完成条件：管理端复杂域不再依赖统一 `adminAPI` 对象，跨域依赖具有明确公开 owner。

### 阶段 4：迁移用户域

- [x] 迁移 `auth` 与 `profile`，保持内存 access token 和 HttpOnly refresh cookie 不变量。
- [ ] 迁移 `billing` 与 `subscriptions`，保持支付 SDK 延迟加载和回调恢复行为。
- [ ] 迁移 `keys` 与 `usage`，保持筛选、分页、统计和路由查询语义。
- [ ] 迁移 `channels-user`、`model-plaza` 和 channel monitor 类型依赖。
- [ ] 对剩余简单 feature 仅执行 owner 收口，不机械添加 Domain/Mapper。

已完成 `auth` 与 `profile` 用户域收口：认证 DTO、会话/Token、只读查询、验证码/密码恢复和 OAuth 编排分别进入 `authDtos.ts`、`authSessionActions.ts`、`authQueries.ts`、`authVerificationActions.ts` 与 `authOAuthActions.ts`；762 行旧 datasource 收缩为 87 行纯兼容 facade。Auth Store 和所有页面直接依赖明确 owner，兼容 `authAPI` 保持原函数身份；Profile 与 TOTP 页面直接依赖 `profileDatasource.ts`、`totpDatasource.ts` 的命名函数，`userAPI`/`totpAPI` 仅供顶层兼容 API 转发。

全仓 feature 通过 `@/features/auth` 读取 Auth Store，通过 `auth/totpStepUpDialog.ts` 与 `passkeys/profilePasskeyCard.ts` 复用窄组件契约；对 `auth/presentation/` 和 Passkey 私有组件的跨域引用归零。`auth/profile` 的 `@/api`、`@/stores` 与 facade 对象引用归零，并一并迁移 `keys`、`billing`、`admin-audit` 中只为 auth/profile 所需的旧 API 导入。legacy barrel 基线从 60 条降至 35 条，跨 feature 私有 presentation 基线从 32 个文件/46 条降至 15 个文件/25 条。

会话不变量由源码与回归共同锁定：access token 仅进入 `tokenStore` 内存，持久 refresh credential 继续通过 HttpOnly cookie 与 `refreshBrowserSession()` 恢复；同标签页请求合并、跨标签页刷新轮换串行、初始路由等待恢复、真实 401 清理与瞬时 `/auth/me` 失败保留会话均保持不变。协议回归锁定 auth/public-settings、验证码、邀请码、密码恢复、2FA、注销与会话吊销路径及 facade 身份；真实 runtime message 矩阵覆盖 Profile 用户路由与管理端 step-up 的中英文词表 scope。

完成条件：用户域不再通过顶层兼容 barrel 访问 API 或 Store。

### 阶段 5：清理中央类型和兼容层

- [ ] 将 `src/types/` 中单一 feature 专属类型迁回 owner。
- [ ] 为真正共享的协议建立小而稳定的公共契约。
- [ ] 确认运行时代码对 `src/api/` 和 `src/stores/` 的引用为零。
- [ ] 删除 `src/api/index.ts`、`src/api/admin/index.ts` 和 `src/stores/index.ts`。
- [ ] 移除迁移基线并将架构规则提升为全量硬门禁。
- [ ] 更新前端索引、代码地图和请求链路文档。

完成条件：兼容层删除后，全量测试和生产构建仍通过。

## 6. 兼容性约束

架构迁移不得破坏以下行为：

- 后端 URL、HTTP 方法、JSON 字段、SSE 和错误格式保持不变。
- 请求方法门禁按当前路由契约生成，不以统一改写方法、方法覆盖或 GET/POST 互换破坏兼容性。
- 可选字段、旧响应和滚动升级兼容语义保持不变。
- access token 仅保存在内存，refresh credential 继续使用 HttpOnly cookie。
- 页面 loading、empty、error、分页、筛选和选择状态保持不变。
- AbortController、搜索防抖、ETag、轮询和写后刷新时机保持不变。
- 静态 import 和现有路由懒加载边界保持不变，不意外扩大首屏 chunk。
- 前端权限只负责体验，不能替代后端鉴权。
- 不修改 `backend/internal/transport/webassets/dist/` 生成产物。

## 7. 验证策略

单个 feature 迁移至少执行：

```sh
cd frontend
pnpm exec vitest run src/features/<domain>
pnpm exec eslint src/features/<domain> --ext .ts,.vue
pnpm run typecheck
```

每个迁移批次执行：

```sh
cd frontend
pnpm run lint:check
pnpm run test:run
pnpm run build
```

文档和最终收口执行：

```sh
make check-docs
make test-frontend
```

最终批次还应使用生产 Docker 构建验证嵌入式前端，并在实际浏览器检查桌面和移动端关键路径：登录、账号管理、设置保存、用量查询、订阅与支付。

共享组件的词表验证必须同时覆盖中文/英文与用户/管理员路由 scope。全路由静态依赖扫描应确认每个 `t()` key 在对应权限实际加载的词表中存在；发生过原始 key 泄漏的组件还要做 runtime message 渲染断言，并纳入 `make test-frontend` 的 CI 关键集。

请求方法专项还必须在后端和 CDN/WAF 等价测试环境执行正负矩阵：合法方法返回原有结果，非白名单方法和覆盖尝试在进入业务 handler 前被拒绝；分别核对 CDN 命中、旁路和源站请求计数。

## 8. 验收指标

| 指标 | 目标 |
| --- | ---: |
| 运行时 `@/api`、`@/api/admin` 引用 | 0 |
| 运行时 `@/stores` 引用 | 0 |
| 未声明的跨 feature 私有 presentation 依赖 | 0 |
| Domain 对 Vue、Pinia、Router、Axios 的依赖 | 0 |
| 超过 1500 有效行的运行时 TypeScript/Vue 模块 | 0 |
| 新增无归属 barrel | 0 |
| 动态 HTTP 方法、方法覆盖和未登记原生请求出口 | 0 |
| 非白名单方法到达业务 handler | 0 |
| 用户/管理员路由中原样显示的 locale key | 0 |
| 因架构迁移产生的后端协议变化 | 0 |
| 新增前端运行时依赖 | 默认 0，专项评审后例外 |
| lint、typecheck、相关测试和生产 build | 全部通过 |

不以目录数量、Domain 文件数量或 Mapper 数量作为成功指标。成功标准是 owner 清晰、依赖可检查、请求生命周期可追踪，并且行为没有回归。

## 9. 提交与回滚

- 一个提交只迁移一个 feature 或一个明确子域。
- 兼容 barrel 保留到消费者归零并完成回归验证后再删除。
- 不在同一提交中混入 UI 重设计、后端协议变化或无关格式化。
- 每批迁移前记录请求与交互基线，失败时可以按 feature 单独回滚。
- 不使用全仓库自动搬迁替代逐 owner 审查。
- 工作区存在其他修改时，必须与其协作，不得覆盖、还原或重新格式化无关文件。

## 10. 当前进度与接手位置

截至第十三切片，`admin-accounts` 的页面列表、统计、用量、模型、上游计费、批量编辑、额度通知、定时测试、OAuth、重新授权、创建/编辑、数据导入、CRS 预览/同步和账号专属 DTO 已经进入明确 owner。架构测试确认精确 legacy barrel 基线为 100 条，其中该 feature 的 `@/api/admin` 运行时入口已归零。

2026-08-03 收口验证记录：

- 定向测试通过，共 3 个测试文件、16 个测试；`admin-accounts` 回归通过，共 43 个测试文件、361 个测试。
- 宿主机全局 lint 和 typecheck 通过；Docker 全量前端 lint、typecheck 和测试通过，共 257 个测试文件、1652 个测试。
- 文档检查通过；生产镜像 `sub2api-frontend-arch-runtime:20260803` 构建成功，并在隔离 PostgreSQL、Redis 环境中完成迁移和自动初始化，`/health` 返回正常状态。
- 实际浏览器完成首页、登录页、管理员登录、仪表盘和账号管理页冒烟验证；账号管理页正常显示筛选、操作栏与空数据表格，新的已认证页面没有控制台错误。

2026-08-04 OAuth Action 切片验证记录：

- OAuth owner 定向验证通过，共 5 个测试文件、20 个测试；`admin-accounts` 回归通过，共 44 个测试文件、365 个测试。
- 宿主机全局 lint 和 typecheck 通过；Docker 全量前端 lint、typecheck、测试和生产 build 通过，共 261 个测试文件、1672 个测试。
- 正式多阶段镜像 `sub2api-frontend-arch-runtime:20260804-oauth` 构建成功，确认生产前端可嵌入 Go 后端二进制。

2026-08-04 重新授权切片验证记录：

- 定向验证通过，共 6 个测试文件、26 个测试；`admin-accounts` 回归通过，共 44 个测试文件、367 个测试。
- 宿主机 lint 和 typecheck 通过；Docker 全量前端 lint、typecheck、测试和生产 build 通过，共 261 个测试文件、1674 个测试。
- Docker 隔离验证镜像为 `sub2api-frontend-reauth-test:20260804-final`；正式多阶段运行时镜像 `sub2api-frontend-arch-runtime:20260804-reauth` 构建成功（`linux/arm64`，manifest `sha256:00981ff088f191239a358f87ee359af7bf2aa62c5b885fc8b7de5377f31aa20e`）。

2026-08-04 创建/编辑切片验证记录：

- 定向验证通过，共 9 个测试文件、115 个测试；`admin-accounts` 回归通过，共 44 个测试文件、371 个测试。
- 宿主机全局 lint 和 typecheck 通过；Docker 全量验证和正式多阶段运行时镜像已在本切片最终收口后补充。
- Docker 全量验证镜像为 `sub2api-frontend-create-edit-test:20260804`（261 个文件、1678 项测试、lint/typecheck/build 通过）；正式多阶段运行时镜像为 `sub2api-frontend-arch-runtime:20260804-create-edit`（`linux/arm64`，约 41.2 MB，manifest `sha256:1f8d8107b16864d15722d5814441186f30433c169f084784a905c71c23910dec`）。

2026-08-04 导入/CRS 同步切片验证记录：

- `adminAccountQueries.ts` 新增 CRS 预览 owner；`adminAccountActions.ts` 收口数据导入与 CRS 同步，保留默认分组绑定字段、结果统计、预览选择和 `180000ms` 同步 timeout。
- `ImportDataDialog.vue` 和 `SyncFromCrsDialog.vue` 不再依赖 `@/api/admin`；兼容 `accountsAPI` 继续保持原函数身份。
- 定向验证通过，共 5 个测试文件、38 个测试；`admin-accounts` 回归通过，共 45 个测试文件、377 个测试；宿主机全局 lint 和 typecheck 通过。
- 当前精确 legacy barrel 基线为 100 条，`admin-accounts` 的 `@/api/admin` 运行时入口为 0 条。
- Docker 隔离验证镜像 `sub2api-frontend-import-sync-test-suite:20260804` 内的全局 lint、typecheck、262 个测试文件/1684 项测试和 production build 全部通过（`linux/arm64`，manifest `sha256:11ae3358c91f117bbbeb2a29b17b5de3fd34cfaeea057c20ece66a5c76a4835a`）。
- 正式多阶段运行时镜像 `sub2api-frontend-arch-runtime:20260804-import-sync` 构建成功（`linux/arm64`，约 41.2 MB，manifest `sha256:404d3ec65cfa9d848827e90541bdfea7f7a6ab81e7f9244d3e4f9867850e8f32`）。

2026-08-04 账号 DTO 切片验证记录：

- `adminAccountDtos.ts` 收口 14 个账号专属协议类型；`gateway.ts` 删除对应声明，`@/types` 保留兼容转发，架构测试锁定 owner 和声明边界。
- 定向验证通过，共 3 个测试文件、30 个测试；`admin-accounts` 回归通过，共 45 个测试文件、378 个测试；宿主机全局 lint、typecheck、全量测试（262 个测试文件/1685 项）和 production build 全部通过。
- Docker 隔离验证镜像 `sub2api-frontend-dto-test-suite:20260804` 内的全局 lint、typecheck、262 个测试文件/1685 项测试和 production build 全部通过（`linux/arm64`，manifest `sha256:7291373296e2eb92aa7a5cc67348a98ce30462b0763f472749dbc9523def2c4f`）。
- 正式多阶段运行时镜像 `sub2api-frontend-arch-runtime:20260804-dto` 构建成功（`linux/arm64`，40,262,958 bytes，manifest `sha256:c08b9edf650e65616a51f7e8eb7d60b021911ec1c1875a195ace04541bdbd9e2`）；容器内 `sub2api --version` 正常返回 `Sub2API 0.1.173`。

2026-08-09 `admin-settings` 第一切片记录：

- 邮件模板和面板限流已建立 DTO、Query、Action owner；兼容 `settingsAPI` 继续保留相同函数身份。
- `admin-settings` 的运行时旧 barrel 引用由 9 条降至 0，仓库精确基线由 100 条降至 91 条。
- 定向 owner、组件和设置页验证通过，共 4 个测试文件、55 项测试；`admin-settings` feature 回归通过，共 12 个测试文件、91 项测试。
- 宿主机全局 lint、typecheck、全量测试（268 个测试文件/1745 项）和 production build 全部通过；`make test-frontend` 的 6 个关键测试文件/115 项测试及 `make check-docs` 通过。
- 正式多阶段运行时镜像 `sub2api-frontend-arch-runtime:20260809-settings` 构建成功（`linux/arm64`，40,398,265 bytes，manifest `sha256:c220c6e6669f770bdd2af19942a71f76f2aff86f56562da2b91732a16185219b`）；容器内 `sub2api --version` 正常返回 `Sub2API 0.1.178`。

2026-08-09 `admin-settings` 第二切片记录：

- 管理员 API Key、独立网关策略、Web Search 和 SMTP 测试协议进入 DTO、Query、Action owner；兼容 `settingsAPI` 保持相同函数身份。
- `adminSettingsDatasource.ts` 从 1631 行降至 1224 行；presentation 仅剩主设置 `getSettings/updateSettings` 继续通过兼容 facade。
- 定向 owner 和设置页验证通过，共 2 个测试文件、51 项测试；`admin-settings` feature 回归通过，共 12 个测试文件、96 项测试。
- 宿主机全局 lint、typecheck、全量测试（268 个测试文件/1750 项）和 production build 全部通过；`make test-frontend` 的 6 个关键测试文件/115 项测试及 `make check-docs` 通过。
- 正式多阶段运行时镜像 `sub2api-frontend-arch-runtime:20260809-settings2` 构建成功（`linux/arm64`，40,398,463 bytes，manifest `sha256:0e746f336684b2c3df1d25ce55e8d717ade2926d2728048d3a83e910b63bca25`）；容器内 `sub2api --version` 正常返回 `Sub2API 0.1.178`。

2026-08-09 `admin-settings` 第三切片记录：

- `systemSettingsDtos.ts` 接管主设置协议以及注册、认证来源、平台额度、支付方式和微信模式兼容归一化；`adminSettingsQueries.ts` 与 `adminSettingsActions.ts` 分别接管主设置统一加载和保存。
- `adminSettingsDatasource.ts` 收缩为 89 行的兼容 facade，不再声明 DTO、实现归一化或直接访问 `apiClient`；`settingsAPI` 的函数身份保持不变，运行时仅 `src/api/admin/index.ts` 继续使用该兼容出口。
- 主设置定向验证通过，共 6 个测试文件、78 项测试；`admin-settings` 回归通过，共 13 个测试文件、101 项测试；受影响的 `admin-accounts` 回归通过，共 45 个测试文件、382 项测试。
- 宿主机全局 lint、typecheck、全量测试（269 个测试文件/1755 项）和 production build 全部通过；`make test-frontend` 的 6 个关键测试文件/115 项测试及 `make check-docs` 通过。
- 正式多阶段运行时镜像 `sub2api-frontend-arch-runtime:20260809-settings3` 构建成功（`linux/arm64`，40,398,129 bytes，manifest `sha256:9445bdde0dda477d826a4a59b1a7d16ed6fe27cc2c1f4da353ed34b73adc510d`）；容器内 `sub2api --version` 正常返回 `Sub2API 0.1.178`。Docker Desktop 无法解析 Docker Hub 的 Dockerfile frontend，本次使用仅移除 `# syntax=docker/dockerfile:1.7` 的临时 Dockerfile 和本地已有基础镜像完成等价构建，仓库 Dockerfile 未修改。

2026-08-09 `admin-ops` snapshot/metrics 第一切片记录：

- `opsDashboardDtos.ts` 与 `opsDashboardQueries.ts` 接管 overview、`snapshot-v2`、吞吐/切换趋势、延迟分布和错误趋势/分布；`opsMetricsDtos.ts` 与 `opsMetricsQueries.ts` 接管图片、Token、用户用量、并发、可用性和实时流量摘要。
- 页面、趋势组件、指标卡、并发卡和实时摘要 composable 已直接依赖 Query/DTO owner；15 个兼容 `opsAPI` 方法保持原函数身份，`adminOpsDatasource.ts` 从 1537 行降至 922 行，legacy barrel 基线仍为 91 条。
- 定向 owner、结构和受影响组件验证通过，共 6 个测试文件、24 项测试；`admin-ops` feature 回归通过，共 14 个测试文件、64 项测试。
- 宿主机全局 lint、typecheck、全量测试（271 个测试文件/1762 项）和 production build 全部通过；`make test-frontend` 的 6 个关键测试文件/115 项测试通过。
- 正式多阶段运行时镜像 `sub2api-frontend-arch-runtime:20260809-ops-dashboard` 构建成功（`linux/arm64`，40,398,365 bytes，manifest `sha256:5b2343c2d012a4dced9fbad18568deed6983dc9f41c0dcb9fa179c1fc7a0f22b`）；容器内 `sub2api --version` 正常返回 `Sub2API 0.1.178`。沿用仅移除 Dockerfile frontend 声明的临时等价 Dockerfile 和本地基础镜像，仓库 Dockerfile 未修改。

2026-08-09 `admin-ops` 日志/请求明细第二切片记录：

- `opsLogDtos.ts` 接管请求明细、系统日志、运行时日志配置、清理 payload 和 sink health 协议；`opsLogQueries.ts` 接管四个只读端点，`opsLogActions.ts` 接管配置保存/重置和日志清理。
- 请求明细和系统日志组件直接依赖新 owner；7 个兼容 `opsAPI` 方法保持原函数身份，`adminOpsDatasource.ts` 从 922 行降至 764 行，legacy barrel 基线仍为 91 条。
- 定向 owner、结构和组件验证通过，共 4 个测试文件、12 项测试；`admin-ops` feature 回归通过，共 15 个测试文件、67 项测试。
- 宿主机全局 lint、typecheck、全量测试（272 个测试文件/1765 项）和 production build 全部通过；`make test-frontend` 的 6 个关键测试文件/115 项测试通过。
- 正式多阶段运行时镜像 `sub2api-frontend-arch-runtime:20260809-ops-logs` 构建成功（`linux/arm64`，40,398,364 bytes，manifest `sha256:3dcaae87fa3d423938067703f80f2fbf5f6e3311275cbc003f20c61774593c54`）；容器内 `sub2api --version` 正常返回 `Sub2API 0.1.178`。沿用仅移除 Dockerfile frontend 声明的临时等价 Dockerfile 和本地基础镜像，仓库 Dockerfile 未修改。

2026-08-17 `admin-ops` 错误列表/详情第三切片与跨权限词表守卫记录：

- `opsErrorDtos.ts` 接管 unified 与 request/upstream split 错误协议，`opsErrorQueries.ts` 接管七个只读端点，`opsErrorActions.ts` 接管三个 resolved 操作；错误组件和 `admin-usage` 已直接依赖新 owner。
- 10 个兼容 `opsAPI` 方法保持原函数身份；`view=errors|excluded|all`、分页筛选、`include_detail=1` 和 resolved payload 保持不变，`adminOpsDatasource.ts` 从 764 行降至 620 行。
- `routeLocaleCoverage.spec.ts` 已纳入关键集，按每条路由实际加载的 scope 检查中英文静态词表依赖；`GroupOptionItem.spec.ts` 使用真实 runtime message 覆盖用户/管理员与中英文四种组合，验证普通用户不加载管理员词表且任何组合都不显示原始 locale key。
- 定向 `admin-ops` 与受影响的 `admin-usage` 验证通过，共 18 个测试文件、87 项测试；宿主机 lint、typecheck、`make test-frontend`（8 个测试文件/126 项）和 `make check-docs` 通过。
- Docker 隔离验证镜像 `sub2api-frontend-ops-errors-i18n-test:20260817` 内的全局 lint、typecheck、全量测试（292 个测试文件/1882 项）和 production build 全部通过（`linux/arm64`，manifest `sha256:bf65d0b0007aa7a2e872117caebda912a782e63c3ffda09817d08bc1a7d79eec`）。
- 仓库原始 `Dockerfile` 构建的正式多阶段运行时镜像 `sub2api-frontend-arch-runtime:20260817-ops-errors-i18n` 验证通过（`linux/arm64`，41,377,481 bytes，manifest `sha256:c611fa09e7de73531872c588f747eaf3a731e073788516501bde48b394cba0f3`）；容器经正式 entrypoint 执行 `sub2api --version` 正常返回 `Sub2API 0.1.182`。

2026-08-17 `admin-ops` 告警/设置/WebSocket 第四切片记录：

- 告警、设置和 WebSocket 分别迁入明确 DTO、Query、Action 与 subscription owner；18 个兼容 `opsAPI` 方法保持原函数身份，`adminOpsDatasource.ts` 从 620 行降至 151 行的纯兼容 facade。
- `admin-settings` Store 增加 feature 级稳定公开出口，告警规则的分组查询直接依赖 `admin-groups` datasource；`admin-ops` presentation 中兼容 facade、旧 API/Store barrel 和私有跨 feature presentation 引用全部归零，仓库 legacy barrel 基线从 83 条降至 78 条。
- HTTP 回归锁定告警/设置全部路径、查询参数和 payload；WebSocket 回归覆盖 token subprotocol、消息、致命关闭、陈旧检测、指数退避与离线恢复，并修正有限重连上限的 off-by-one。
- `admin-ops` feature 回归共 19 个测试文件、84 项测试；宿主机全局 lint、typecheck 和全量测试（294 个测试文件/1891 项）通过，`make check-docs` 通过。
- Docker 隔离验证镜像 `sub2api-frontend-admin-ops-complete-test:20260817` 内的冻结安装、全局 lint、typecheck、全量测试（294 个测试文件/1891 项）和 production build 全部通过（`linux/arm64`，148,583,964 bytes，manifest `sha256:27ecc653078a330133aae20f80281957f23bd5493df2e62d354af62cdf6c23f5`）。
- 仓库原始 `Dockerfile` 构建的正式多阶段运行时镜像 `sub2api-frontend-arch-runtime:20260817-admin-ops-complete` 验证通过（`linux/arm64`，42,076,276 bytes，manifest `sha256:ab17945f6dfd1b477c0fe8fadfa1ab41b4f5995098f72dbbee213467104f5bea`）；容器经正式 entrypoint 执行 `sub2api --version` 正常返回 `Sub2API 0.1.182`。

2026-08-17 `admin-users` DTO 与旧 admin barrel 收口记录：

- `adminUserDtos.ts` 接管身份绑定、批量限制、余额历史和平台额度协议；旧 datasource、`@/types` 与管理员 API barrel 保持兼容转发，新增契约回归覆盖列表 AbortSignal/属性筛选、余额历史、平台额度 batch/update/reset payload 和 facade 函数身份。
- 用户列表和 11 个请求型 widget 已直接依赖 5 个明确 datasource owner，`admin-users` presentation 中 `@/api/admin` 与 `adminAPI` 引用归零；精确 legacy barrel 基线从 78 条降至 66 条。定向回归共 8 个测试文件、43 项测试，同时覆盖用户/管理员与中英文四种共享组件词表组合。
- 宿主机全局 lint、typecheck、全量测试（294 个测试文件/1897 项）、production build、`make test-frontend`（8 个测试文件/126 项）和 `make check-docs` 全部通过。
- Docker 隔离验证镜像 `sub2api-frontend-admin-users-complete-test:20260817` 内的冻结安装、全局 lint、typecheck、全量测试（294 个测试文件/1897 项）和 production build 全部通过（`linux/arm64`，146,506,835 bytes，manifest `sha256:eb2a9defd0d60b6892ce08fde7876c3997314337959de2a1e3cc37a372d74561`）。
- 仓库原始 `Dockerfile` 构建的正式多阶段运行时镜像 `sub2api-frontend-arch-runtime:20260817-admin-users-complete` 验证通过（`linux/arm64`，41,378,843 bytes，manifest `sha256:d665258fb06b9a89d57ac3c8ed626b1afade836c5e8f0701b6ee9916c79050c5`）；容器经正式 entrypoint 执行 `sub2api --version` 正常返回 `Sub2API 0.1.182`。

2026-08-17 `admin-groups` 协议 owner 与跨权限词表收口记录：

- 分组共享类型和管理员专属 DTO 已分别进入 `src/types/group.ts` 与 `adminGroupDtos.ts`，读取和写入进入 `adminGroupQueries.ts`、`adminGroupActions.ts`；74 行兼容 facade 保持函数身份，feature 运行时旧分组出口归零，legacy barrel 基线从 66 条降至 62 条。
- 契约回归锁定分组、组合路由、倍率、RPM 与汇总路径、payload 和字段映射，并覆盖 AbortSignal、300ms 搜索防抖、服务端时区、复制幂等键、异步防陈旧及写后刷新顺序；`admin-groups` 共 19 个测试文件/81 项测试通过。
- `GroupOptionItem` 改用所有消费路由均加载的 `groups.rateLabel`；runtime message 矩阵覆盖 `/keys`、`/admin/users`、`/admin/subscriptions`、`/admin/redeem`、`/admin/settings` 的用户/管理员权限和中英文组合。全路由依赖闭包扫描会按真实 locale scope 拒绝缺失 key，相关 3 个测试文件/24 项测试及 `make test-frontend` 的 8 个文件/132 项测试通过。
- Docker 隔离验证镜像 `sub2api-frontend-admin-groups-complete-test:20260817` 内的冻结安装、全局 lint、typecheck、全量测试（296 个测试文件/1911 项）和 production build 全部通过（`linux/arm64`，148,035,598 bytes，manifest `sha256:ae522297c60219ba7af1810d5800f7fdaeca5b83b65fcf26a90628f0699290ba`）。
- 仓库原始 `Dockerfile` 构建的正式多阶段运行时镜像 `sub2api-frontend-arch-runtime:20260817-admin-groups-complete` 验证通过（`linux/arm64`，41,379,249 bytes，manifest `sha256:0093098df17b13784e8c5df2b8b7be9ab974e8d48b2a426d3c580d3295256c60`）；容器经正式 entrypoint 执行 `sub2api --version` 正常返回 `Sub2API 0.1.182`。

2026-08-17 `admin-usage` 协议 owner 与跨 feature presentation 收口记录：

- 用量列表、统计、用户/API Key 搜索与清理任务已进入明确 datasource/Query/Action owner；列表 AbortSignal、导出精确分页、300ms 搜索防抖、异步防陈旧和路由用户回填行为保持不变，兼容 `adminUsageAPI` 保持原函数身份。
- `admin-usage` presentation 中 `@/api/admin`、`adminAPI`、`adminUsageAPI` 与跨 feature 私有 `presentation/` 引用全部归零；余额历史与错误列表/详情改由三个窄公开组件入口复用。legacy barrel 基线从 62 条降至 60 条，跨 feature 私有 presentation 基线为 44 个文件/70 条引用。
- 定向 owner/组件验证共 4 个测试文件/24 项测试，`admin-usage` feature 回归共 8 个测试文件/51 项测试，权限词表与架构组合共 6 个测试文件/43 项测试；宿主机全局 lint、typecheck、全量测试（298 个测试文件/1919 项）、production build、`make test-frontend`（8 个测试文件/132 项）和 `make check-docs` 全部通过。
- Docker 隔离验证镜像 `sub2api-frontend-admin-usage-complete-test:20260817` 内的冻结安装、全局 lint、typecheck、全量测试（298 个测试文件/1919 项）和 production build 全部通过（`linux/arm64`，148,039,272 bytes，manifest `sha256:9a44ebec9c3c9c035a2415ffa892c7b6f7be6ca371d6a9c042d98a3dbcf53bd2`）。
- 仓库原始 `Dockerfile` 构建的正式多阶段运行时镜像 `sub2api-frontend-arch-runtime:20260817-admin-usage-complete` 验证通过（`linux/arm64`，41,381,812 bytes，manifest `sha256:cf663f523969b074e1d718fe4fb604362064323137db1d577770c41f825af161`）；容器经正式 entrypoint 执行 `sub2api --version` 正常返回 `Sub2API 0.1.182`。

2026-08-17 `admin-orders` / `billing` 支付共享契约与跨权限词表收口记录：

- `paymentContracts.ts` 接管共享支付协议，`paymentDisplay.ts` 接管币种、订单状态、退款、日期和套餐有效期规则，`paymentMethods.ts` 接管支付方式别名归一化；旧 `@/types/payment` 与 presentation formatter 保留 type/function-compatible 转发。
- 管理支付 DTO、读取和写入分别进入 `adminPaymentDtos.ts`、`adminPaymentQueries.ts` 和 `adminPaymentActions.ts`；页面与设置域不再依赖 `adminPaymentAPI`，兼容 facade 的全部函数身份、URL、查询参数、payload 和 Axios 返回形态保持不变。
- 订单状态/表格、Provider 弹窗/列表、订阅卡片/Store 使用逐组件公开入口；feature 运行时对 `billing/presentation/` 的跨域引用及 billing 对 auth/subscriptions 私有 presentation 的引用归零。跨 feature 私有 presentation 基线从 44 个文件/70 条降至 32 个文件/46 条，legacy barrel 基线保持 60 条。
- 共享订单状态组件的真实 runtime message 矩阵覆盖 `/orders`、`/admin/orders` 的用户/管理员与中英文组合，并与全路由词表依赖闭包一同进入关键集。Docker 首轮全量测试发现两个全仓扫描在并发负载下逼近/超过旧 10s/30s 超时，已统一改为 60s；扫描范围、断言和失败条件不变，最终容器内分别约 10s/18s 通过。
- 受影响的 `admin-orders`、`billing`、`subscriptions`、`admin-settings` 与 `affiliate` 回归共 42 个测试文件/275 项测试；宿主机全局 lint、typecheck、全量测试（301 个测试文件/1932 项）、production build、`make test-frontend`（9 个测试文件/136 项）和 `make check-docs` 全部通过。
- Docker 隔离验证镜像 `sub2api-frontend-admin-orders-billing-test:20260817` 内的冻结安装、全局 lint、typecheck、全量测试（301 个测试文件/1932 项）和 production build 全部通过（`linux/arm64`，148,043,301 bytes，manifest `sha256:398fb7ffb8ea395910b7c03c72e0874fd0b644f944f5fade86749709e3e1cc10`）。
- 仓库原始 `Dockerfile` 构建的正式多阶段运行时镜像 `sub2api-frontend-arch-runtime:20260817-admin-orders-billing-complete` 验证通过（`linux/arm64`，41,377,434 bytes，manifest `sha256:529b34d0141b52ff9661339f67bfbc140326eb06e9a69bf28b3617ea49601ebf`）；容器经正式 entrypoint 执行 `sub2api --version` 正常返回 `Sub2API 0.1.182`。

2026-08-17 `auth` / `profile` owner、会话不变量与跨权限词表收口记录：

- Auth DTO、会话/Token、查询、验证和 OAuth 分别进入 `authDtos.ts`、`authSessionActions.ts`、`authQueries.ts`、`authVerificationActions.ts` 与 `authOAuthActions.ts`；旧 `authDatasource.ts` 从 762 行收缩为 87 行纯兼容 facade，`authAPI` 保持全部函数身份。
- Auth Store、认证页面与 Profile/TOTP 组件直接依赖明确 owner；全仓 feature 通过 `@/features/auth`、`auth/totpStepUpDialog.ts` 与 `passkeys/profilePasskeyCard.ts` 使用稳定入口，对 auth/passkey 私有 presentation 的跨域引用归零。`auth/profile` 的旧 API/Store 与 facade 对象引用归零，并同步迁移只为这些 owner 所需的 keys、billing、admin-audit 旧 API 导入。
- access token 与 legacy OAuth refresh 响应只进入 `tokenStore` 内存，长期 refresh credential 继续由 HttpOnly cookie 和 `refreshBrowserSession()` 恢复；同标签页合并、Web Locks/storage lease 跨标签页串行、初始路由等待、真实 401 清理和瞬时 `/auth/me` 失败保留已恢复会话均由回归覆盖。
- 协议回归锁定 `/auth/me`、公开设置、验证码、邀请码、密码恢复、2FA、注销、会话吊销和 facade 身份；架构回归锁定零旧入口/零私有依赖。真实 runtime message 矩阵覆盖 `/profile` 用户权限与 `/admin/accounts` step-up 的中英文组合，并与全路由词表闭包一同进入关键集。
- legacy barrel 基线从 60 条降至 35 条，跨 feature 私有 presentation 基线从 32 个文件/46 条降至 15 个文件/25 条。`auth/profile`、session refresh 与 route guard 定向回归共 33 个测试文件/260 项测试；宿主机全局 lint、typecheck、全量测试（304 个测试文件/1945 项）、production build、`make test-frontend`（10 个测试文件/140 项）和 `make check-docs` 全部通过。
- Docker 隔离验证镜像 `sub2api-frontend-auth-profile-complete-test:20260817` 内的冻结安装、全局 lint、typecheck、全量测试（304 个测试文件/1945 项）和 production build 全部通过（`linux/arm64`，148,049,079 bytes，manifest `sha256:d00959d041962ee8a2dcd19092e2f702fcb47f4bd982f34bb448a648e2ede1c7`）。
- 仓库原始 `Dockerfile` 构建的正式多阶段运行时镜像 `sub2api-frontend-arch-runtime:20260817-auth-profile-complete` 验证通过（`linux/arm64`，41,380,071 bytes，manifest `sha256:185f37988d75856928b7b2d33433eb22b017e8f098cc979f21540e8bcc79df34`）；容器经正式 entrypoint 执行 `sub2api --version` 正常返回 `Sub2API 0.1.182`。

下一步迁移 `billing` 与 `subscriptions` 用户域，保持支付 SDK 延迟加载与回调恢复行为。
