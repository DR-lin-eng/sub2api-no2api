# Admin Risk Control

风险控制 feature 负责审核配置、入口与边缘防护运行状态、命中记录和管理员解禁操作。

- `data/datasources/`: 风控配置、状态、日志和管理动作协议。
- `presentation/pages/`: 路由级加载、保存、日志刷新和轮询生命周期。
- `presentation/widgets/`: 运行概览、记录表格和输入详情展示。
- `presentation/composables/`: 轮询、表单归一化和选项解析。

页面是请求和状态 owner。记录 widget 只展示并上抛筛选、分页和解禁动作；resolver 必须保持旧配置缺失字段的默认值。入口页同时管理 Cloudflare 持久配置和运行健康：Token 输入只上送、保存后清空，后端只回传 `api_token_configured`。Cloudflare 可选择逐条 Access Rule 或按多个主机名维护多个 WAF Rule 分片；WAF 区展示后端缓存的 24 小时请求与拦截合计及逐主机明细，不在页面刷新时直连 Cloudflare。已配置且启用时默认只显示配置摘要，主配置和高级参数分两级按需展开。健康字段仍必须兼容旧后端缺失字段。扩展配置时同步检查 load/save 双向转换，并保持唯一的 15 秒状态轮询及卸载清理。

验证入口：

```sh
pnpm exec vitest run src/features/admin-risk-control
pnpm run typecheck
```
