# Application Services

本包是现有业务兼容层，集中保存应用端口、核心用例和跨模块编排。package 名仍为 `service`，以保持现有构造器和调用方稳定。

## 文件索引

| 前缀 | 职责 |
| --- | --- |
| `account*`, `admin_account*`, `admin_group*`, `admin_user*` | 账号与管理用例 |
| `auth*`, `oauth*`, `token*`, `totp*` | 身份、会话和凭据刷新 |
| `gateway*` | Anthropic/Claude 通用网关请求、调度、转发和计费 |
| `openai*` | OpenAI/Codex/Responses/Images/WS 管线 |
| `gemini*`, `grok*`, `antigravity*`, `bedrock*` | 各上游协议适配与重试 |
| `scheduler*`, `channel*`, `ratelimit*`, `concurrency*` | 调度、通道和并发控制 |
| `billing*`, `usage*`, `pricing*`, `subscription*` | 计费、用量和订阅 |
| `payment*`, `batch_image*` | 支付应用编排与批量图片任务 |
| `ops*`, `audit*`, `content_moderation*` | 运维、审计和内容策略 |
| `setting*`, `notification*`, `backup*` | 配置、通知和维护用例 |
| `cluster*` | 稳定节点身份、心跳清单、任务租约、就绪门禁与串行版本发布 |
| `wire.go` | application provider 集合 |

### 核心拆分索引

| 文件组 | 职责 |
| --- | --- |
| `content_moderation.go` | 内容审核常量、传输模型、端口和服务状态 |
| `content_moderation_config_api.go`, `content_moderation_config_rules.go`, `content_moderation_validation.go` | 配置读写、默认规则和校验 |
| `content_moderation_check.go`, `content_moderation_queue.go` | 同步审核决策与异步任务处理 |
| `content_moderation_runtime.go`, `content_moderation_cleanup.go`, `content_moderation_admin.go` | 运行快照、清理和管理查询 |
| `content_moderation_client.go`, `content_moderation_key_health.go` | 上游审核 API 和密钥健康状态 |
| `content_moderation_side_effects.go`, `content_moderation_cyber_policy.go` | 命中后的账户、通知和网络安全事件处理 |
| `content_moderation_test_input.go` | 管理端测试输入和确定性评分辅助 |
| `setting_parse.go`, `setting_parse_core.go` | 持久设置默认值、解析编排与基础站点设置 |
| `setting_parse_identity.go`, `setting_parse_oidc.go` | LinuxDo、DingTalk、OIDC、OAuth 与微信身份源设置 |
| `setting_parse_features.go`, `setting_parse_gateway.go`, `setting_parse_notifications.go` | 功能开关、网关调度与通知展示设置 |
| `setting_update.go`, `setting_update_prepare.go` | 持久设置更新编排、首错顺序与跨域预处理 |
| `setting_update_core.go`, `setting_update_identity.go`, `setting_update_product.go` | 注册访问、身份源与产品默认设置写入 |
| `setting_update_gateway.go`, `setting_update_notifications.go` | 网关调度、通知与平台额度设置写入 |
| `setting_codex_simulation.go` | Codex A/B 数据库覆盖、强制关闭、身份密钥生成与后台同步的无 DB 热路径快照 |
| `api_key_group_routing.go` | API Key 有序分组候选、倍率保护过滤和请求内实际分组切换 |
| `openai_codex_identity_plan.go`, `openai_codex_simulation_state.go` | Codex request root、per-principal 身份计划与短状态 fallback |
| `openai_codex_continuation.go` | Codex continuation 分类、owner 策略、跨主体 sanitizer 与成功回写 |
| `openai_gateway_forward.go`, `openai_gateway_request_build.go` | OpenAI 转发编排与 HTTP 上游请求构造 |
| `invalid_auth_abuse_limiter.go`, `cloudflare_ingress_settings.go` | 无效 API Key 来源计数、本地临时封禁，以及 Access Rule/WAF 双模式的 Cloudflare 加密持久设置与边缘端口 |

## 拆分约定

单个功能按 `types/plan/request/forward/response/billing/runtime` 职责拆文件，不按“公共 helper”堆积。新增功能若不需要访问本包大量私有状态，应建立 `modules/<domain>` 并通过接口接入。

本包禁止导入 `internal/infrastructure/repository`；例外只能在 lint 配置中显式记录并附迁移原因。
