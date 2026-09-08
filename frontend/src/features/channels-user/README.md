# User Channels And Custom Pages

本 feature 组合 `/available-channels` 可用渠道、`/monitor` 登录监控和 `/custom/:id` 自定义页面。

- [data/datasources/channelsUserDatasource.ts](data/datasources/channelsUserDatasource.ts)：当前用户可见渠道。
- [data/datasources/embeddedCapabilityDatasource.ts](data/datasources/embeddedCapabilityDatasource.ts)：自定义嵌入页面的专用能力凭据请求。
- [presentation/pages](presentation/pages/)：渠道列表、监控组件装配和自定义内容宿主。
- [presentation/customPageHtml.ts](presentation/customPageHtml.ts)：自定义 HTML 处理边界。

渠道展示不授予新分组权限。自定义 HTML/iframe 必须保留现有清洗、sandbox、URL 与消息来源校验；专用能力凭据不等于浏览器 JWT，不能把登录 token 或通用管理员凭据发给嵌入页面。外部支付嵌入流程见 [支付集成 API](../../../../docs/ADMIN_PAYMENT_INTEGRATION_API.md)。

从 `frontend/` 执行 `pnpm exec vitest run src/features/channels-user src/features/channel-monitor-user`。HTML 渲染变化还需共享动态 HTML 门禁；跨域问题参阅 [CORS 部署](../../../../docs/FRONTEND_CORS_DEPLOYMENT_CN.md)。
