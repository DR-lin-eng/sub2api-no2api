# Admin Channels

本 feature 管理渠道列表、渠道表单、平台模型映射和渠道级定价规则。

## 代码边界

- `presentation/pages/ChannelsPage.vue`：路由页状态、列表请求、对话框编排、校验、账号搜索和生命周期。
- `presentation/channelFormCodec.ts`：渠道表单与 API 结构的纯双向转换，以及账号统计定价规则的平台分发。
- `presentation/adminChannelSignals.ts`：价格单位、区间和模型模式等可复用信号处理。
- `presentation/widgets/`：渠道表单中的定价、模型和区间展示控件。
- `data/datasources/adminChannelsDatasource.ts`：渠道 HTTP 协议和请求入口。

`channelFormCodec.ts` 使用静态 feature-local import，继续归入管理渠道路由 chunk。它不拥有响应式状态、HTTP 请求、watcher、timer 或生命周期；转换时必须保持 API 字段、数组顺序、旧 `features_config` 字段兼容和既有读取语义。

## 验证

从 `frontend/` 执行：

```sh
pnpm exec vitest run src/features/admin-channels/__tests__
pnpm exec eslint src/features/admin-channels
pnpm run typecheck
```

Web Search 的 Anthropic/OpenAI 平台开关由 codec 的同一集合决定；保存时写显式 true/false，未编辑和未来平台维度保留。后端只对该键按维度合并；其它特性仍遵守原整块替换语义。
