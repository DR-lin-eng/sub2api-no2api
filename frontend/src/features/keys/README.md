# API Keys

API Key feature 负责密钥列表、创建编辑、用量刷新和客户端导入。

- `data/datasources/`: API Key CRUD、用量与待结算查询协议。
- `presentation/pages/`: 列表请求、用量小批次有界并发与失败批次的单 Key 降级、5s/60s 刷新调度、visibility 和弹窗编排。
- `presentation/widgets/`: 表格、创建编辑器、端点说明和使用说明。
- `presentation/resolvers/`: 客户端配置的纯序列化逻辑与 feature-private 静态模型目录。
- `presentation/keysPageContext.ts`: 表格与编辑器消费的有界类型契约。

`KeysPage.vue` 是请求、AbortController、localStorage、轮询和弹窗状态 owner。静态 widget 只消费 typed context，不自行加载数据。扩展字段时同步检查创建/编辑 payload、列偏好版本和 pending usage 刷新条件。

验证入口：

```sh
pnpm exec vitest run src/features/keys
pnpm run typecheck
```
