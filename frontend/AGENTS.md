# Frontend Agent Guide

本文件补充根目录 `AGENTS.md`，适用于 `frontend/` 子树。开始修改前先读 `README.md`，路由或 Store 任务再读对应子目录 README。

## 运行事实

- 依赖管理只使用 pnpm，脚本以 `package.json` 为准。
- 应用入口是 `src/main.ts`；路由事实源是 `src/core/routes/index.ts`。
- 共享 HTTP 与 session 行为集中在 `src/core/networks/`。
- i18n 事实源是 `src/core/i18n/`；全局主题入口是 `src/core/themes/style.css`。
- access token 仅存内存，refresh credential 由后端 HttpOnly cookie 管理。
- 生产构建输出到 `../backend/internal/transport/webassets/dist/`，该目录不手改。

## 代码归属

```text
main / core routes
  -> feature presentation
  -> feature data datasource
  -> core networks
  -> backend
```

- 业务默认归属 `src/features/<domain>/`。页面、领域组件、交互和 Store 分别进入 `presentation/pages/`, `widgets/`, `composables/`, `stores/`。
- 领域 HTTP 调用进入所属 feature 的 `data/datasources/`，并统一使用 `src/core/networks/client.ts`。
- 跨业务复用 UI、页面、composable 和 UI 类型进入 `src/common/`；不要把领域 API 或领域 Store 放入 `common`。
- 应用级 Router、HTTP/session、i18n、主题、全局 Store、服务、常量和工具进入 `src/core/`。
- 只有真正跨 feature 的协议类型和全局声明保留在 `src/types/`。
- 用户可见文案进入 `src/core/i18n/locales/`，同步所有受支持语言。
- 新增 `v-html` 或非空 `innerHTML` 前必须先建立专用清洗 owner，并同步更新动态 HTML 安全门禁；不得直接渲染 API、设置、URL、storage 或消息返回的原始 HTML。

`src/api/index.ts`、`src/api/admin/index.ts` 和 `src/stores/index.ts` 是旧导入的过渡兼容 barrel。新代码直接导入 owner 路径；除维持既有导入兼容外，不要把新功能继续注册到这些 barrel，也不要为旧目录再创建平行实现。旧导入全部迁移并通过回归验证前不得移除兼容导出。

存量旧 barrel 和跨 feature 私有 `presentation` 导入逐条登记在 `eslint/architecture-debt-baseline.cjs`。ESLint 会拒绝新增债务，也会在旧引用迁移后要求删除对应基线项；不要为通过检查而扩大该文件。`domain`、`data` 的相对导入同样受反向依赖检查，不能用相对路径绕过 owner 边界。

不要在页面中创建新的 Axios client、token 刷新队列或权限事实源。前端 guard 和菜单隐藏只负责体验，后端必须执行真实权限校验。

## 修改检查

- 新页面：feature owner、route、lazy import、meta、导航、datasource/type、i18n、权限和 page test。
- 新 API 字段：后端 DTO 兼容、所属 feature 的 datasource/type/mapper、空值和旧响应测试。
- Store：并发加载去重、失败恢复、invalidate、logout/user switch 清理。
- 表格/筛选：loading、empty、error、分页、查询取消和 URL/偏好持久化。
- 支付/认证：不要提前加载第三方 SDK；验证回调、刷新竞争和失败跳转。
- 大页面：page 只负责路由级编排和请求生命周期；领域表单、表格、tab/panel 放入同 feature 的 widget，复用交互和纯转换放入 composable/resolver。源码拆分默认使用静态 import，不能为了缩短文件擅自改变路由 chunk 或请求时序。
- 所有运行时 TypeScript/Vue 模块必须保持在 1500 有效行以内，ESLint 会忽略空行和注释后执行硬门禁。接近上限时按业务职责继续拆分，不能通过关闭规则、压缩排版、搬到 composable/datasource 或把整页状态塞进无边界 Store 绕过。

## 验证

以下命令从 `frontend/` 执行：

```sh
pnpm run lint:check
pnpm run typecheck
pnpm run test:run
pnpm exec vitest run src/path/to/example.spec.ts
pnpm run build
```

单页面改动先跑相邻 spec 和 typecheck。Router、API client、认证、共享 Store、i18n 或构建配置属于共享面，需要运行所有相关 spec、lint 和 typecheck。视觉/响应式改动还要在实际浏览器验证桌面和移动宽度。

## 文档同步

- 顶层目录或通用约定变化：更新 `README.md`。
- Router meta/guard 变化：更新 `src/core/routes/README.md`。
- Store owner/生命周期变化：更新 `src/core/stores/README.md`。
- 跨前后端调用链变化：更新 `../docs/CODE_MAP.md` 或 `REQUEST_LIFECYCLES.md`。
