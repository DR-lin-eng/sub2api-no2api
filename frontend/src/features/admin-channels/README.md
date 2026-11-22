# Admin Channels

本 feature 管理渠道列表、渠道表单、平台模型映射和渠道级定价规则。

从渠道管理点击「创建渠道」，先搜索或按类别选择厂商，也可选择稍后配置。选定厂商后，再在「基础与分组」「模型与映射」「计费与统计」中配置。一个渠道可添加多个厂商。更换厂商会保留本次打开期间的草稿，保存时只提交已启用厂商的分组、映射和定价。编辑已有渠道直接进入配置，组合分组仍按各平台展开。

渠道模型定价支持 Token、按次、图片和按秒视频。新增模型时，默认价仅回填到仍未修改的同一条 Token 定价草稿。按 Token 结算时，区间下界不含、上界包含；未命中区间使用显式配置的默认价，默认字段留空则沿用已有基础价。分时倍率作用于本次费用。

## 代码边界

- `presentation/pages/ChannelsPage.vue`：路由页状态、列表请求、对话框编排、校验、账号搜索和生命周期。
- `presentation/channelProviders.ts`：渠道定价目录和厂商专属开关。账号与渠道共用 `core/constants/modelProviders.ts` 的厂商目录和分类。新增厂商时同步补齐中英文介绍和平台显示名称。
- `presentation/channelFormCodec.ts`：渠道表单与 API 结构的纯双向转换，以及账号统计定价规则的平台分发。
- `presentation/adminChannelSignals.ts`：价格单位、区间和模型模式等可复用信号处理。
- `presentation/widgets/`：厂商选择器、分组和专属开关、模型映射与定价控件。
- `data/datasources/adminChannelsDatasource.ts`：渠道 HTTP 协议和请求入口。

`channelFormCodec.ts` 使用静态 feature-local import，继续归入管理渠道路由 chunk。它不拥有响应式状态、HTTP 请求、watcher、timer 或生命周期；转换时必须保持 API 字段、数组顺序、旧 `features_config` 字段兼容和既有读取语义。

## 验证

从 `frontend/` 执行：

```sh
pnpm exec vitest run src/features/admin-channels/__tests__
pnpm exec eslint src/features/admin-channels
pnpm run typecheck
```

Web Search 的 Anthropic/OpenAI 平台开关由厂商目录决定；保存时写显式 true/false，未编辑和未来平台维度保留。后端只对该键按维度合并；其它特性仍遵守原整块替换语义。

Bedrock 兼容开关按后台读取的渠道级 boolean 保存，关闭时显式写 false；其余未知 feature 维度继续保留。
