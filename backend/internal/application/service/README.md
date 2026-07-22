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

## 拆分约定

单个功能按 `types/plan/request/forward/response/billing/runtime` 职责拆文件，不按“公共 helper”堆积。新增功能若不需要访问本包大量私有状态，应建立 `modules/<domain>` 并通过接口接入。

本包禁止导入 `internal/infrastructure/repository`；例外只能在 lint 配置中显式记录并附迁移原因。
