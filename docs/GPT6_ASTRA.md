# GPT-6 Astra 接入记录

## OpenAI 官方资料

- 模型页：[GPT-6 Astra](https://developers.openai.com/api/docs/models/gpt-6-astra)
- 目录：[Models](https://developers.openai.com/api/docs/models)
- 价格：[Pricing](https://developers.openai.com/api/docs/pricing)
- Model ID：`gpt-6-astra`
- 类型：reasoning；推理强度 `low`、`medium`、`high`、`xhigh`、`max`
- 上下文窗口：1,050,000；最大输入：922,000；最大输出：128,000
- 输入：text、image；输出：text
- 端点：Responses、Chat Completions、Batch
- 能力：streaming、structured outputs、function calling、file search、image input、web search、prompt caching

本次只新增可选模型，不把账号测试或已有分组的默认模型从 `gpt-5.6-sol` 自动切换到 Astra；未获早期访问资格的账号因此不会在升级后被动命中 Astra。

## 本项目价格映射（USD / 1M tokens）

| 档位 | 输入 | 缓存读取 | 缓存写入 | 输出 |
| --- | ---: | ---: | ---: | ---: |
| Standard（272K 以下） | 10.00 | 1.00 | 12.50 | 50.00 |
| Standard（超过 272K） | 20.00 | 2.00 | 25.00 | 75.00 |
| Fast / priority | Standard 对应价格 × 2 | × 2 | × 2 | × 2 |
| Flex | Standard 对应价格 × 0.5 | × 0.5 | × 0.5 | × 0.5 |
| Batch | Standard 对应价格 × 0.5 | × 0.5 | × 0.5 | × 0.5 |

缓存写入按输入标准价的 1.25 倍计费；长上下文对整次请求的输入/缓存应用 2 倍、输出应用 1.5 倍。

OpenAI 已把 Priority processing 更名为 Fast；请求中的 `service_tier: "fast"` 和
`service_tier: "priority"` 在本项目中使用同一 Fast 价格。Astra 不支持 `none` 或
`minimal` 推理强度，兼容客户端传入这两档时定向降为最低可用的 `low`，其他模型的
既有归一化保持不变。Chat Completions 转 Responses 的兼容路径会按 reasoning 模型
规则丢弃 Astra 不支持的自定义 `temperature` 和 `top_p`，直通协议仍保持原请求语义。

内置价格是 OpenAI 直接 API 的基准价。EU data-residency Astra 不支持 Fast；符合条件
的 regional processing 端点另有 10% 加价，本项目不从上游 URL 猜测地域，部署者应在
对应渠道倍率中显式配置该加价。

## 实现位置

- `backend/internal/shared/openai/constants.go`：默认模型目录
- `backend/internal/application/service/openai_model_alias.go`：模型规范化与未知模型拒绝
- `backend/internal/application/service/openai_codex_transform.go`：Codex 模型映射
- `backend/internal/application/service/openai_codex_models_service.go`：图片输入、Fast 能力清单补全
- `backend/internal/application/service/billing_service.go`：统一计费与静态兜底价
- `backend/internal/application/service/pricing_service.go`：远端价格缺失时的静态兜底价
- `backend/resources/model-pricing/model_prices_and_context_window.json`：内置价格目录
- `frontend/src/features/admin-accounts/presentation/composables/useModelWhitelist.ts`：管理端白名单
- `frontend/src/features/keys/presentation/resolvers/openCodeModelCatalogs.ts`：OpenCode 模型描述

显式上游价格目录优先于静态兜底价；静态兜底仅在远端/本地动态目录没有可用 GPT-6 Astra 条目时生效。
