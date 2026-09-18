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
| `proxy_auto_assignment*`, `proxy_health*` | 代理池强制均衡分配、定期测活和失活迁移 |
| `oauth_model_sync_service.go` | OpenAI OAuth 实时模型能力快照与定时同步 |
| `oauth2_provider.go` | 管理员控制的 OAuth2 客户端、PKCE 授权码、token 校验与 userinfo |
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
| `setting_codex_simulation.go`, `setting_codex_turn_state_replay.go`, `openai_codex_turn_state_auto_replay.go`, `openai_codex_turn_state_probe.go` | Codex A/B/C 数据库覆盖、强制关闭、Turn State 手工随机池/质量同步、关注模型的可配置字符长度自动池、首次无效立即探测与 45 分钟刷新、32 目标乘 4 代理的有界并发和 5 秒无限轮次恢复，以及身份密钥生成与后台同步的无 DB 热路径快照 |
| `wire_openai_gateway.go` | OpenAI gateway provider、代理库与跨实例探测锁装配及后台探测启动 |
| `api_key_group_routing.go` | API Key 有序分组候选、倍率保护过滤和请求内实际分组切换 |
| `openai_codex_identity_plan.go`, `openai_codex_simulation_state.go` | Codex request root、per-principal 身份计划与短状态 fallback |
| `openai_codex_continuation.go` | Codex continuation 分类、owner 策略、跨主体 sanitizer 与成功回写 |
| `openai_oauth_gateway_rate_limit.go` | 统一配置、按账号分桶且跨实例共享的 OpenAI OAuth RPM/burst 准入、逻辑请求去重与协议错误投影 |
| `openai_request_integrity_observe.go` | OpenAI OAuth 兼容转换前后语义字段的只观察差异诊断 |
| `openai_quota_account_transport.go` | OpenAI quota/reset 通过账号级 HTTP/TLS upstream 的辅助请求路径 |
| `account_quality_runtime_snapshot.go` | 质量巡检私有 artifact 的非敏感请求运行画像快照 |
| `ratelimit_oauth_401_delete.go` | 网关 OAuth 401 自动删除准入、凭据 CAS 结果和运行态清理 |
| `openai_codex_remote_control_store.go` | Codex Remote Control enrollment token 的账号级加密持久化适配 |
| `openai_gateway_forward.go`, `openai_gateway_request_build.go` | OpenAI 转发编排与 HTTP 上游请求构造 |
| `invalid_auth_abuse_limiter.go`, `cloudflare_ingress_settings.go` | 无效 API Key 来源计数、本地临时封禁，以及 Access Rule/WAF 双模式的 Cloudflare 加密持久设置与边缘端口 |

## 拆分约定

单个功能按 `types/plan/request/forward/response/billing/runtime` 职责拆文件，不按“公共 helper”堆积。新增功能若不需要访问本包大量私有状态，应建立 `modules/<domain>` 并通过接口接入。

本包禁止导入 `internal/infrastructure/repository`；例外只能在 lint 配置中显式记录并附迁移原因。
