# Common Pages And Widgets

本目录持有不属于单一业务 feature 的页面、布局、通用组件和可复用交互。公共页面入口包括 `/home`、`/legal/:documentId` 和未匹配路由的 404 页面；业务功能仍由 `src/features/<domain>/` owner 持有。

## 页面与边界

- [pages/HomePage.vue](pages/HomePage.vue)：读取公开设置，展示默认首页、紧凑首页或受控自定义首页内容，并提供登录/仪表盘与模型广场入口。
- [pages/LegalDocumentPage.vue](pages/LegalDocumentPage.vue)：展示登录协议、管理员合规文档和公开法律文档；Markdown 转 HTML 后经过清洗。
- [pages/NotFoundPage.vue](pages/NotFoundPage.vue)：统一未知路由反馈，不承载业务状态。
- [widgets/](widgets/)：跨用户端、管理端或多个 feature 复用的布局、上传、公告和展示组件。

首页的 `home_content` 只有受控 HTML 或绝对 HTTP(S) URL 可进入相应展示路径。URL iframe 使用无同源权限的 sandbox、`no-referrer` 和懒加载；HTML、Markdown、SVG 和公告内容分别使用各自 sanitizer。公共页面不能把浏览器 token、管理权限或未清洗的 API 内容写入 DOM。

法律文档的 `admin-compliance` 内容由仓库内置双语 Markdown 提供，其他登录协议来自公开设置；文档不存在、为空或设置加载失败时分别展示对应状态。公共页面读取设置用于展示，不替代后端鉴权或合规守卫。

## 依赖与验证

Common 可以依赖 `core` 和稳定的 feature public entry，但不应持有领域 API、领域 Store 或管理员权限事实。公告组件通过 `features/announcements` 的公开 Store 工作，认证和网络能力继续由 `core`/`features/auth` 提供。

从 `frontend/` 执行：

```sh
pnpm exec vitest run src/common/pages src/common/widgets/data/__tests__
pnpm exec vitest run src/__tests__/dynamicHtmlSecurity.spec.ts src/core/utils/__tests__/homeContent.spec.ts
```

修改公共 HTML、iframe、Markdown 或 SVG 展示时，必须同时检查动态 HTML 安全门禁和所有消费路由的 locale scope。
