# Codex CLI 语音接入与分钟计费

本接入对照 OpenAI Codex 最新稳定版本 `0.160.1`（`rust-v0.160.1`，commit `d27764b82f7118f674371e6d6e76271d9d606edb`）。CLI `/voice` 选择声音，F8 开始或结束语音；TUI 使用 WebRTC 和 V3 Frameless Sideband，默认语音模型是 `gpt-live-1-codex`。

## 配置

管理员在 OpenAI 分组开启“允许访问 Live”，并绑定可调度的普通 OpenAI OAuth 账号。此能力默认关闭，不支持 PAT、Agent Identity 或上游 API Key 账号。分组模型白名单开启时，加入 `gpt-live-1-codex`。

在客户端 `~/.codex/config.toml` 的顶层设置 Sideband 地址。保留原有任务模型，新增或合并以下 provider 配置（`HOST` 是网关地址）：

```toml
model_provider = "sub2api"
experimental_realtime_ws_base_url = "https://HOST/v1"

[model_providers.sub2api]
name = "Sub2API"
base_url = "https://HOST/v1"
env_key = "SUB2API_API_KEY"
wire_api = "responses"
```

通过环境变量 `SUB2API_API_KEY` 提供本项目 API Key。Sideband 地址需要显式设置：官方 CLI 的 WebRTC Sideband 默认使用 OpenAI 的地址，不会自动跟随自定义 provider。语音音频由客户端与上游 WebRTC 直接交换，网关转发创建和控制协议；网络仍须允许客户端连接上游媒体服务。

## 协议

- `POST /v1/live`：CLI V3 multipart `sdp` 与 `session`。`GET /v1/live/:call_id`：Sideband WebSocket。
- `POST /live` 与 `GET /live/:call_id`：无 `/v1` 的兼容入口。
- `POST /backend-api/codex/realtime/calls`：Codex OAuth JSON 创建；`GET /backend-api/codex/:call_id`：对应 Sideband。
- `POST /v1/realtime/calls` 和 `POST /realtime/calls`：兼容创建别名。

上游普通 OpenAI OAuth 账号使用 ChatGPT Codex 创建与 Sideband 路径。`session` 的声音、instructions、delegation、initial_items 等字段保持不变。创建与 Sideband 使用同一服务端会话身份，客户端设备证明不会被转移到另一个 OAuth 账号。

官方 CLI 对语音请求的 attestation 是可选项；`gpt-live-1-codex` 不强制要求服务端安装 macOS ChatGPT 的 DeviceCheck 模块。旧版 Desktop Live 会话继续使用原有设备证明链路。是否实际允许请求仍由 OpenAI 的账号权限、地区和上游风控决定。

## 费用与恢复

[OpenAI 官方说明](https://learn.chatgpt.com/docs/pricing#how-much-does-voice-cost)规定语音为 `$0.05/分钟`，执行任务的模型另外按其 token 价格收费。本项目以服务端通话时长按毫秒折算分钟：30 秒 `$0.025`，60 秒 `$0.05`，90 秒 `$0.075`。实际扣款应用用户专属或分组倍率，0 倍率保持免费，不叠加 token 高峰倍率。

通话创建时冻结倍率与配额策略。终止时 Redis 原子冻结计费时长，最多计算到配置的会话到期时间；余额、订阅、API Key 额度/限额及用户平台配额通过现有幂等账务仓库结算。语音用量为 `request_type=live`、`billing_mode=live`，token 数为 0，任务请求的 token 账单独立产生。

未结算记录保留在 Redis，固定批量恢复器处理断连、数据库写入失败和进程重启后的到期会话。账务与用量都持久化后才移除待结算标记；重试使用相同请求 ID、时长和账务指纹。成功记录保留 24 小时。Redis 需使用现有持久化及备份配置；账务仓库已有的持久队列行为保持不变。

回退代码前应先停止接收新语音、结束活动会话并排空 `live:billing:pending`。源代码回退不撤销已经完成的账务记录，也不替代对 Redis 和数据库的备份恢复。

## 验证入口

从 `backend/` 运行 `go test ./internal/application/service ./internal/infrastructure/repository ./internal/transport/http/handler ./internal/transport/http/server/routes`。专项覆盖 CLI payload、稳定 Sideband 身份、旧 Desktop 证明要求、分钟价格、订阅/余额、结算重试和跨实例时长冻结。

源码依据：官方仓库 `codex-rs/core/src/realtime_conversation.rs`、`core/src/client.rs`、`codex-api/src/endpoint/realtime_call.rs`、`codex-api/src/endpoint/realtime_websocket/methods.rs` 和 `tui/src/chatwidget/realtime.rs`。测试使用本地协议与存储夹具；真实 OAuth 语音和客户端媒体连通性须在部署环境另行验收。
