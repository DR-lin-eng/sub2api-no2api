# GPT-6 模型目录与价格

## 官方资料

- [Models](https://developers.openai.com/api/docs/models)
- [GPT-6.2 Astra](https://developers.openai.com/api/docs/models/gpt-6.2-astra)
- [GPT-6.2 Sol](https://developers.openai.com/api/docs/models/gpt-6.2-sol)
- [GPT-6.2 迁移指南](https://developers.openai.com/api/docs/guides/latest-model)
- [GPT-6.1 Sol](https://developers.openai.com/api/docs/models/gpt-6.1-sol)
- [GPT-6.1 Terra](https://developers.openai.com/api/docs/models/gpt-6.1-terra)
- [GPT-6 Sol](https://developers.openai.com/api/docs/models/gpt-6-sol)
- [GPT-6 Luna](https://developers.openai.com/api/docs/models/gpt-6-luna)
- [Pricing](https://developers.openai.com/api/docs/pricing)

截至 2026-11-18，内置 OpenAI 目录包含 `gpt-6.2-astra`、`gpt-6.2-sol`、`gpt-6-astra`、`gpt-6.1-sol`、
`gpt-6.1-terra`、`gpt-6-sol` 和 `gpt-6-luna`。本次更新不改变账号测试默认值或已有
分组的默认映射。

## 能力

| 模型 | Model ID | 推理强度 | 上下文窗口 | 最大输入 | 最大输出 |
| --- | --- | --- | ---: | ---: | ---: |
| GPT-6.2 Astra | `gpt-6.2-astra` | low、medium、high、xhigh、max | 1,050,000 | 922,000 | 128,000 |
| GPT-6.2 Sol | `gpt-6.2-sol` | low、medium、high、xhigh、max | 1,050,000 | 922,000 | 128,000 |
| GPT-6 Astra | `gpt-6-astra` | low、medium、high、xhigh、max | 1,050,000 | 922,000 | 128,000 |
| GPT-6.1 Sol | `gpt-6.1-sol` | low、medium、high、xhigh、max | 1,050,000 | 922,000 | 128,000 |
| GPT-6.1 Terra | `gpt-6.1-terra` | none、low、medium、high、xhigh、max | 1,050,000 | 922,000 | 128,000 |
| GPT-6 Sol | `gpt-6-sol` | none、low、medium、high、xhigh、max | 1,050,000 | 922,000 | 128,000 |
| GPT-6 Luna | `gpt-6-luna` | none、low、medium、high、xhigh、max | 1,050,000 | 922,000 | 128,000 |

七个模型均支持 text/image 输入、text 输出、Responses、Chat Completions、Batch、流式、
structured outputs、web search 和 prompt caching。GPT-6.2 Astra/Sol 和 GPT-6.1 Sol/Terra 的工具调用必须使用
Responses API；仅支持 Chat Completions 的账号在最终模型映射后拒绝这些工具请求。
GPT-6 Sol/Luna/Terra 的官方模型页明确列出 `none`；Astra、GPT-6.1 Sol 和两个 GPT-6.2 模型对不支持的 `none`/`minimal` 按兼容逻辑
降为 `low`。

GPT-6.2 的原生 Responses 路径移除 `temperature`、`top_p`、`top_logprobs` 和
`include` 中的 `message.output_text.logprobs`；Chat Completions 路径同时移除 `logprobs`。
保留其它 `include` 项、加密推理内容和 `max` 强度。模型发现补齐图片输入、工具和上下文元数据；
管理端白名单、身份映射预设及 OpenCode 配置包含这两个模型。显式账号映射和分组白名单仍决定实际可用范围。

## 标准价格

单位为 USD / 1M tokens。下表缓存写入采用 30 分钟保留期，为输入标准价的 1.25 倍；超过 272K 输入 token 的
请求，输入和缓存价格乘 2，输出价格乘 1.5；Fast（兼容 `priority`）乘 2，
Batch 乘 0.5。GPT-6.2 的官方价卡未列出 Flex，不从旧模型推断其可用性。

| 模型 | 输入 | 缓存读取 | 缓存写入 | 输出 |
| --- | ---: | ---: | ---: | ---: |
| GPT-6.2 Astra | 8.00 | 0.40 | 10.00 | 40.00 |
| GPT-6.2 Sol | 2.00 | 0.10 | 2.50 | 10.00 |
| GPT-6 Astra | 10.00 | 1.00 | 12.50 | 50.00 |
| GPT-6.1 Sol | 2.00 | 0.10 | 2.50 | 10.00 |
| GPT-6.1 Terra | 1.00 | 0.05 | 1.25 | 5.00 |
| GPT-6 Sol | 2.00 | 0.20 | 2.50 | 10.00 |
| GPT-6 Luna | 0.10 | 0.01 | 0.125 | 0.50 |

官方 Ultrafast 价卡覆盖 GPT-6 Astra、GPT-6.1 Sol 和两个 GPT-6.2 模型。
旧模型为 Standard 的 6 倍；GPT-6.2 在 2027-02-28（Pacific time）结束前为 3 倍，
自 2027-03-01 00:00 Pacific（08:00 UTC，北京时间 16:00）起自动切换为 6 倍。
切换使用既有计费时间参数，不依赖进程启动时间或价格目录刷新；叠加长上下文倍率，
并应用于输入、输出、缓存读取和缓存写入。
请求及用量记录保留 `service_tier=ultrafast`；模型专属计费策略也应用于渠道的 Standard
自定义价格，且不修改共享价格快照。未列出 Ultrafast 官方价卡的模型不推断此倍率。
JSON 或终止 SSE 事件附带实际 `service_tier` 时，以该值结算并记录用量；未返回时
沿用请求档位。请求 Ultrafast 后上游降为 Standard 的响应因此按 Standard 收费。

官方同时列出 12 小时缓存写入价格：GPT-6.2 Astra 为 20.00、GPT-6.2 Sol 为 5.00、Terra 为 2.50 USD / 1M tokens。
当前本项目的 OpenAI 缓存写入计费仍沿用聚合用量的 30 分钟价格，尚未新增按
`prompt_cache_options.ttl` 区分 12 小时写入的结算路径。
官方新增的 2/8 小时 background completion window 折扣也未接入当前同步计费。
Batch 价卡为模型元数据，不代表本次新增 Batch 提交、异步轮询或多代理协议入口。

内置 `backend/resources/model-pricing/model_prices_and_context_window.json` 与
`BillingService`/`PricingService` 的静态兜底都使用上述价格；远端价格目录存在时仍以
远端精确条目优先。

## 实现位置

- `backend/internal/shared/openai/constants.go`：`/models` 默认目录
- `backend/internal/application/service/openai_model_alias.go`：GPT-6 别名归一化和未知模型拒绝
- `backend/internal/application/service/openai_codex_transform.go`：Codex 模型映射
- `backend/internal/application/service/openai_gpt62_request.go`：保留对话前缀的 GPT-6.2 参数补丁
- `backend/internal/application/service/openai_compat_tool_model.go`：Responses 工具调用限制
- `backend/internal/application/service/openai_codex_models_service.go`：图片输入/Fast 能力补全
- `backend/internal/application/service/billing_service.go`：统一计费静态兜底
- `backend/internal/application/service/billing_openai_ultrafast.go`：模型专属 Ultrafast 倍率
- `backend/internal/application/service/pricing_service.go`：动态目录缺失时的静态兜底
- `frontend/src/features/admin-accounts/presentation/composables/useModelWhitelist.ts`：管理端白名单
- `frontend/src/features/keys/presentation/resolvers/openCodeModelCatalogs.ts`：OpenCode 配置目录
