# 管理端 OpenAI SIWC 授权

管理员在“账号管理 → 创建账号 → OpenAI → OAuth”的授权步骤中选择 **Sign in with ChatGPT（SIWC）**，生成链接后在浏览器登录并授权。浏览器跳转到 `http://127.0.0.1:1455/auth/callback` 后，把地址栏的**完整回调地址**粘贴回管理端，再点击“完成授权”。即使该本地页面无法打开，也可以复制地址栏；不需要在部署服务器上开放 1455 端口。

完整回调包含 `code`、`state` 和已签发的 `client_id`。只粘贴授权码会缺少 SIWC 动态客户端信息，服务端会拒绝兑换。重新授权 SIWC 账号会继续使用相同认证协议。

## 协议与实现位置

本实现参考 [CLIProxyAPI v8.0.57](https://github.com/router-for-me/CLIProxyAPI/tree/c2f402b) 的 `internal/auth/codex/siwc_auth.go`，保留本项目账号、代理和刷新架构。该参考版本的 HEAD 是 `c2f402b`；上游协议变化时应重新核对源码。

| 环节 | SIWC 行为 |
| --- | --- |
| 授权 URL | `https://auth.openai.com/api/accounts/authorize`，使用 `dynamic_agent_client`、Sub2API 标识、设备 UUID、state、nonce、PKCE S256 |
| 请求 scope | `openid profile email offline_access resource.invoke chatgpt.tokens.use.direct` |
| token 兑换与刷新 | `https://auth.openai.com/api/accounts/oauth/token`，发送已签发 `client_id` 和 `resource=https://api.openai.com/v1` |
| 账号凭据 | 继续保存为 OpenAI OAuth 账号，`credentials.auth_mode=siwc`，保留 `client_id`、scope、设备 UUID、access token、refresh token 与过期时间 |
| 模型请求 | 默认通过 `https://api.openai.com/v1/responses`；WebSocket 使用相同源的 `wss` 地址；账号自定义上游继续使用已有地址校验策略 |
| 请求适配 | HTTP/SSE、非流式收集、compact 和 WS 使用对应协议字段；不压缩 SIWC 请求，不发送 ChatGPT workspace、Codex 模拟与桌面证明头 |
| 用量与重置 | 管理端 OpenAI 额度卡对 SIWC 账号查询 WHAM 5 小时/7 天等官方窗口、reset credit 数量/过期时间，并调用 reset-credit consume；请求使用 SIWC agent UA、Bearer token 和账号 principal |

`POST /api/v1/admin/openai/generate-auth-url` 新增可选字段 `flow=siwc`，省略时沿用 Codex OAuth。`exchange-code` 和 `create-from-oauth` 支持完整回调 URL 作为 `code`，或显式的 `code`、`state`、`client_id` 字段。接口继续经过管理端鉴权与权限校验。

服务端会话持有 flow、回调地址、nonce、设备与 PKCE；回调不能更换 token 端点或 redirect URI。兑换时校验 state、回调地址和重复参数，并检查 ID token 的 nonce、token 完整性与 direct scope。凭据刷新沿用现有账号出口、锁和持久化流程。SIWC token 不用于 ChatGPT 后台账号信息和训练隐私设置请求。

SIWC 账号用于直接模型请求。图片生成/编辑使用标准 `/v1/images/generations`、`/v1/images/edits`，
与 API Key 账号共用原生参数与响应路径；账号画图测试遵循同一规则。SIWC 不使用普通 OAuth 的
ChatGPT App conversation、Sentinel 或全局 Codex 中转覆盖。Live、Excel/BPS 与 Spark 影子账号功能不适用于此认证模式；额度查询和 reset credit 是独立的 WHAM 管理路径，不依赖这些功能。

主要源码：

- 协议与 token 校验：`backend/internal/shared/openai/oauth.go`、`siwc.go`。
- 认证编排与刷新：`backend/internal/application/service/openai_oauth_service.go`、`openai_siwc.go`。
- 上游 HTTP 客户端：`backend/internal/infrastructure/repository/openai_siwc.go`。
- 模型请求适配：`backend/internal/application/service/openai_siwc_gateway.go` 与现有 OAuth 端点、HTTP/WS 出站边界。
- 管理端入口：`frontend/src/features/admin-accounts/presentation/widgets/OAuthAuthorizationFlow.vue`、同域 OAuth composable 和 datasource。

## 验证边界

相邻 SIWC 测试使用本地 mock token 服务，覆盖授权参数、完整回调、state/nonce、scope、客户端 ID、刷新轮换、旧 OAuth 隔离及 HTTP/WS 请求契约。实际 OpenAI 登录和真实模型调用需要管理员使用自己的账号完成授权后验收；mock 测试不能证明上游账号资格、额度或未来协议兼容性。

2026-11-06 的本地验证结果：

- Docker / Go 1.26.10：以 `-tags=unit -p 1` 运行 SIWC、OpenAI OAuth/token、Codex models、OpenAI WS、Excel/BPS 与 shadow 相关回归，shared/openai、application/service、repository 和 admin handler 四个包通过，routes 包编译通过。SIWC 转发专项分别验证普通/透传、流式/非流式及 input/output 用量；额度专项 mock WHAM 验证 5 小时/7 天窗口、reset credit 详情和 consume 重置请求。
- Docker / Node 24、pnpm 11.17.0：完整 lint、typecheck 通过；账号管理、OAuth composable、路由词表和动态翻译门禁共 62 个文件、519 个用例通过。
- 仓库根 Dockerfile 构建 Linux ARM64 完整镜像 `sub2api:siwc-20261106` 成功；独立 PostgreSQL 18 / Redis 8 实例中健康与就绪检查通过，匿名管理端访问和原有凭据/token 版本门禁继续生效。
- 浏览器验证隔离实例登录成功后遇到原有合规承诺确认，未替用户确认；SIWC 组件另在独立模拟授权预览中检查桌面和 390px 移动布局，无横向溢出。
- `git diff --check` 通过。仓库原有门禁仍失败：`make check-docs` 的 `PR169_MEDIA_STUDIO_REVIEW.md` 链接目标缺失；`make -C backend check-layout` 的 `payment_fulfillment_test.go`（1201 行）与 `pricing_service.go`（1218 行）超限。这些问题在本次修改前已存在。

多实例部署应先完成全部节点升级，再添加 SIWC 账号。旧节点没有 SIWC 的刷新与模型选路能力。
