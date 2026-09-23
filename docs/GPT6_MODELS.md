# GPT-6 模型目录与价格

## 官方资料

- [Models](https://developers.openai.com/api/docs/models)
- [GPT-6 Sol](https://developers.openai.com/api/docs/models/gpt-6-sol)
- [GPT-6 Luna](https://developers.openai.com/api/docs/models/gpt-6-luna)
- [Pricing](https://developers.openai.com/api/docs/pricing)

当前内置 OpenAI 目录包含 `gpt-6-astra`、`gpt-6-sol` 和 `gpt-6-luna`。本次新增
Sol/Luna，不改变账号测试默认值或已有分组的默认映射。

## 能力

| 模型 | Model ID | 推理强度 | 上下文窗口 | 最大输入 | 最大输出 |
| --- | --- | --- | ---: | ---: | ---: |
| GPT-6 Astra | `gpt-6-astra` | low、medium、high、xhigh、max | 1,050,000 | 922,000 | 128,000 |
| GPT-6 Sol | `gpt-6-sol` | none、low、medium、high、xhigh、max | 1,050,000 | 922,000 | 128,000 |
| GPT-6 Luna | `gpt-6-luna` | none、low、medium、high、xhigh、max | 1,050,000 | 922,000 | 128,000 |

三个模型均支持 text/image 输入、text 输出、Responses、Chat Completions、Batch、流式、
function calling、structured outputs、web search 和 prompt caching。Sol/Luna 的
官方模型页明确列出 `none`，Astra 仍按现有兼容逻辑把不支持的 `none`/`minimal` 降为
`low`。

## 标准价格

单位为 USD / 1M tokens。缓存写入为输入标准价的 1.25 倍；超过 272K 输入 token 的
请求，输入和缓存价格乘 2，输出价格乘 1.5；Fast（兼容 `priority`）乘 2，Flex 和
Batch 乘 0.5。

| 模型 | 输入 | 缓存读取 | 缓存写入 | 输出 |
| --- | ---: | ---: | ---: | ---: |
| GPT-6 Astra | 10.00 | 1.00 | 12.50 | 50.00 |
| GPT-6 Sol | 2.00 | 0.20 | 2.50 | 10.00 |
| GPT-6 Luna | 0.10 | 0.01 | 0.125 | 0.50 |

内置 `backend/resources/model-pricing/model_prices_and_context_window.json` 与
`BillingService`/`PricingService` 的静态兜底都使用上述价格；远端价格目录存在时仍以
远端精确条目优先。

## 实现位置

- `backend/internal/shared/openai/constants.go`：`/models` 默认目录
- `backend/internal/application/service/openai_model_alias.go`：GPT-6 别名归一化和未知模型拒绝
- `backend/internal/application/service/openai_codex_transform.go`：Codex 模型映射
- `backend/internal/application/service/openai_codex_models_service.go`：图片输入/Fast 能力补全
- `backend/internal/application/service/billing_service.go`：统一计费静态兜底
- `backend/internal/application/service/pricing_service.go`：动态目录缺失时的静态兜底
- `frontend/src/features/admin-accounts/presentation/composables/useModelWhitelist.ts`：管理端白名单
- `frontend/src/features/keys/presentation/resolvers/openCodeModelCatalogs.ts`：OpenCode 配置目录
