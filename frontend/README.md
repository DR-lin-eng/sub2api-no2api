# Frontend

本目录是 Sub2API 的 Vue 3 管理与用户界面，使用 TypeScript、Vite、Pinia、Vue Router、Tailwind CSS 和 Vitest。依赖统一由 pnpm 管理。

## 启动与构建

```sh
cd frontend
pnpm install --frozen-lockfile
pnpm run dev
```

Vite 默认监听 `3000`，后端代理默认指向 `http://localhost:8080`。可通过 `VITE_DEV_PORT` 和 `VITE_DEV_PROXY_TARGET` 覆盖。

```sh
pnpm run lint:check
pnpm run typecheck
pnpm run test:run
pnpm run build
```

生产构建输出到 `../backend/internal/transport/webassets/dist/`，由后端嵌入。不要直接编辑该生成目录。`vite.config.ts` 是唯一的 Vite 配置事实源；package scripts 会显式选择它，忽略 TypeScript 增量编译可能留下的 `vite.config.js`。

## 源码索引

| 路径 | 作用 |
| --- | --- |
| `src/main.ts` | 应用启动、Pinia、公开设置、i18n 和 Router 初始化 |
| `src/App.vue` | 全局壳层和应用生命周期 |
| `src/core/` | 应用级基础能力：Router、HTTP/session、i18n、全局 Store、主题、服务、常量和工具 |
| `src/common/` | 跨功能复用的页面、组件、composable 和 UI 类型 |
| `src/features/<domain>/` | 业务垂直切片；包含 datasource、页面、组件、composable、领域 Store 和相邻测试 |
| `src/features/<domain>/data/datasources/` | 对应功能的请求协议、响应类型和 API 调用 |
| `src/features/<domain>/presentation/` | 对应功能的页面、组件、交互逻辑和 Store |
| `src/types/` | 仍被多个功能共享的协议类型和全局类型声明 |
| `src/api/`, `src/stores/` | 迁移期兼容 barrel；只转发旧导入，不是新代码的归属位置 |
| `src/assets/`, `src/core/themes/` | 静态资源、全局样式和专题样式 |

真实的应用级入口如下：

- Router：`src/core/routes/index.ts`
- HTTP client 与 session：`src/core/networks/client.ts`, `tokenStore.ts`, `sessionRefresh.ts`
- 全局 Store：`src/core/stores/`；领域 Store：对应 `src/features/<domain>/presentation/stores/`
- i18n：`src/core/i18n/index.ts` 与 `src/core/i18n/locales/`
- 主题：`src/core/themes/style.css` 及同目录专题样式

## 首屏与按需加载

- i18n 启动时只加载 `base` 文案；Router 在进入页面前按路由加载 `user`、`batchImage`、`supportChat` 或 `admin` scope。新增路由或把文案移入独立语言文件时，同步更新 `src/core/i18n/index.ts` 的路由映射和 scope 测试。
- 公告、管理员合规等非必现的全局对话框使用 `defineAsyncComponent` 并只在状态需要时挂载。不要把低频弹窗或它们的重依赖静态导入应用壳层。
- `vite.config.ts` 将 Markdown、二维码、表格虚拟化、引导、支付和导出等低频依赖拆为独立 chunk。新增重依赖后检查生产 `index.html` 的 `modulepreload`，避免它重新进入匿名用户首屏。
- `index.html` 中的 `#app-logo-preload` 是登录页 Logo 的高优先级预加载入口。修改其结构时同步检查 Vite 开发态 branding 注入和后端 `internal/transport/webassets` 的生产 HTML 注入，确保自定义 Logo 使用同一 URL。

## 模块分层

推荐依赖方向：

```text
main / core routes
  -> feature presentation (page / widget / composable / store)
  -> feature data datasource
  -> core networks
  -> backend

feature presentation -> common widgets/composables + core services/stores/utils
```

- `features/<domain>` 是业务代码的默认 owner。页面放 `presentation/pages/`，领域组件放 `presentation/widgets/`，领域交互放 `presentation/composables/`，API 放 `data/datasources/`。
- `common` 只保存不属于单一业务的复用能力。业务协议、管理端 API 和领域 Store 不应下沉到 `common`。
- `core` 保存应用级运行能力和组合入口。统一认证头、401 刷新、URL 处理和错误拦截集中在 `core/networks`。
- Store 只保存跨页面共享、需要缓存或具备明确启动/停止生命周期的状态；领域 Store 跟随所属 feature。
- 跨 feature 协作应导入明确的 owner 文件，避免新增无归属的顶层聚合模块。
- `src/api/index.ts`、`src/api/admin/index.ts` 和 `src/stores/index.ts` 仅用于平滑迁移旧调用。新代码直接导入 `@/features/...` 或 `@/core/...`，不向兼容 barrel 添加新的业务边界；在旧导入全部迁移并通过回归验证前不要移除这些 barrel。

复杂页面采用稳定的三段式边界：page 负责路由级加载、保存和对话框编排；feature-private widget 负责表单、表格和 tab/panel；composable/resolver 负责可测试的交互状态与纯转换。源码组织拆分使用静态 import，使子模块继续归入原路由 chunk；只有现有路由懒加载和明确的按需重依赖可以使用动态 import。

所有运行时 TypeScript/Vue 模块受 ESLint 的 1500 有效行硬门禁约束，空行和注释不计入。接近上限时按领域职责拆分，不以新增全局 Store、无归属 barrel、一对一 DTO/反射层或把代码搬进 composable/datasource 来转移复杂度。

ESLint 还对迁移期架构债务执行“只减不增”门禁：禁止新增 `@/api`、`@/api/admin`、`@/stores` 引用，禁止新增跨 feature 的私有 `presentation` 导入，并阻止相对路径绕过 `domain -> data -> presentation` 依赖方向。现有引用逐条记录在 `eslint/architecture-debt-baseline.cjs`；迁移一条引用时必须在同一改动中删除对应基线项，不得为新代码扩充基线。

## 认证和权限

短期 access token 保存在内存中，刷新凭据由后端 HttpOnly cookie 管理。`src/core/networks/client.ts` 会合并并发 401 刷新并重试请求。

Router guard 提供页面跳转和功能开关体验，但不是安全边界。管理员、step-up、用户身份和功能权限必须由后端再次验证。

修改登录/刷新流程时同时检查：

- `src/features/auth/presentation/stores/authStore.ts`
- `src/features/auth/data/datasources/authDatasource.ts`
- `src/core/networks/tokenStore.ts`, `sessionRefresh.ts`, `client.ts`
- `src/core/routes/index.ts`
- `src/features/auth/presentation/pages/`

## 添加页面

1. 选择或创建 `src/features/<domain>/`，页面放入 `presentation/pages/`，复杂 UI 拆入 `presentation/widgets/`。
2. 在 `src/core/routes/index.ts` 添加懒加载路由和准确 meta。
3. 在该 feature 的 `data/datasources/` 增加请求；只有真正跨 feature 的协议类型才放 `src/types/`。
4. 新增用户可见文案到 `src/core/i18n/locales/` 的所有受支持语言。
5. 更新导航可见性和后端权限。
6. 添加相邻 Vitest，并运行 lint/typecheck。

路由约定见 [src/core/routes/README.md](src/core/routes/README.md)。

## 状态选择

使用以下判断避免 store 膨胀：

| 状态范围 | 放置位置 |
| --- | --- |
| 单个组件内部 | 组件 `ref` / `computed` |
| 同一 feature 内多个组件 | `presentation/composables/` 或页面级状态 |
| 同一 feature 多页面共享、需要缓存 | `features/<domain>/presentation/stores/` |
| 全应用共享的运行状态 | `core/stores/` |
| URL 可表达的筛选/分页 | route query |
| 后端持久事实 | API + 后端数据库，不以 localStorage 作为事实源 |

Store 索引见 [src/core/stores/README.md](src/core/stores/README.md)。

## 测试

测试与源码相邻或放在同域 `__tests__/`。共享模块的风险高于单页面，应扩大测试范围。

```sh
# 单文件
pnpm exec vitest run src/features/admin-settings/__tests__/SettingsPage.spec.ts

# 全部前端测试
pnpm run test:run

# 静态检查
pnpm run lint:check
pnpm run typecheck
```

涉及路由、认证、API client、共享 store 或构建分包时，至少运行相关 spec、lint 和 typecheck。支付、图片、表格等交互页面还应验证 loading、empty、error 和权限受限状态。

## 维护规则

- 使用 `@/` 别名导入 `src/` 内容。
- 不混用 npm/yarn，不提交 `node_modules/` 和构建缓存。
- 不在页面中创建第二套 Axios client 或 token 刷新逻辑。
- 不在新代码中继续依赖或扩充 `@/api`、`@/api/admin`、`@/stores` 兼容 barrel。
- 不为新增旧 barrel 或跨 feature 私有 presentation 依赖扩充 `eslint/architecture-debt-baseline.cjs`；该文件只随迁移缩小。
- 不把后端实体直接当作 UI 状态；通过类型和映射隔离可选字段与兼容字段。
- 大页面在所属 feature 内按 page 编排、widget 展示、composable 交互和 datasource 协议拆分；保持原路由 chunk、请求时序与局部状态 owner。
- 目录或公共约定变化时更新本 README 和最近的子目录 README。
