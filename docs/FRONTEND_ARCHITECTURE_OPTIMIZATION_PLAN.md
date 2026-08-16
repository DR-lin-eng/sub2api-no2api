# 前端架构优化计划

> 状态：阶段性完成。阶段 1 的渐进式架构门禁已于 2026-08-03 落地，阶段 2 的 `admin-accounts` 试点已于 2026-08-04 收口；阶段 3 已于 2026-08-09 启动，`admin-settings` 已完成三切片迁移；请求方法与 CDN 缓存边界专项待实施。
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
- [ ] `admin-ops` 按 snapshot、日志、错误详情和指标拆只读查询。
- [ ] `admin-users` 迁移用户管理专属 DTO 和旧 admin barrel 调用。
- [ ] `admin-groups` 迁移分组、组合路由和倍率协议 owner。
- [ ] `admin-usage` 消除对 `admin-users`、`admin-ops` 私有 presentation 的直接依赖。
- [ ] `admin-orders` 与 `billing` 提取稳定的 payment 共享契约和格式化能力。

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

完成条件：管理端复杂域不再依赖统一 `adminAPI` 对象，跨域依赖具有明确公开 owner。

### 阶段 4：迁移用户域

- [ ] 迁移 `auth` 与 `profile`，保持内存 access token 和 HttpOnly refresh cookie 不变量。
- [ ] 迁移 `billing` 与 `subscriptions`，保持支付 SDK 延迟加载和回调恢复行为。
- [ ] 迁移 `keys` 与 `usage`，保持筛选、分页、统计和路由查询语义。
- [ ] 迁移 `channels-user`、`model-plaza` 和 channel monitor 类型依赖。
- [ ] 对剩余简单 feature 仅执行 owner 收口，不机械添加 Domain/Mapper。

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

下一步迁移 `admin-ops` 的错误列表、详情和 resolved 操作，保持 legacy unified 与 request/upstream split 端点、`view=errors|excluded|all`、分页筛选、`include_detail` 以及 `admin-usage` 跨 feature 读取行为不变。
