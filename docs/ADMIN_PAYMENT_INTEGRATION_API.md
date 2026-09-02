# ADMIN_PAYMENT_INTEGRATION_API

> 单文件中英双语文档 / Single-file bilingual documentation (Chinese + English)

---

## 中文

### 目标
本文档用于对接外部支付系统（如 `sub2apipay`）与 Sub2API 的 Admin API，覆盖：
- 支付成功后充值
- 用户查询
- 人工余额修正
- 自定义 iframe 上下文与显式令牌委托

### 基础地址
- 生产：`https://<your-domain>`
- Beta：`http://<your-server-ip>:8084`

### 认证
推荐使用：
- `x-api-key: admin-<64hex>`
- `Content-Type: application/json`
- 幂等接口额外传：`Idempotency-Key`

说明：管理员 JWT 也可访问 admin 路由，但服务间调用建议使用 Admin API Key。

Admin API Key 现在采用细粒度 scope。本文涉及的 Key 至少需要 `admin.users.read`、`admin.users.write` 和 `admin.write`。创建、轮换和撤销说明见 [Admin API 调用文档](./ADMIN_API.md)。

### 1) 一步完成创建并兑换
`POST /api/v1/admin/redeem-codes/create-and-redeem`

用途：原子完成“创建兑换码 + 兑换到指定用户”。

请求头：
- `x-api-key`
- `Idempotency-Key`

请求体示例：
```json
{
  "code": "s2p_cm1234567890",
  "type": "balance",
  "value": 100.0,
  "user_id": 123,
  "notes": "sub2apipay order: cm1234567890"
}
```

幂等语义：
- 同 `code` 且 `used_by` 一致：`200`
- 同 `code` 但 `used_by` 不一致：`409`
- 缺少 `Idempotency-Key`：`400`（`IDEMPOTENCY_KEY_REQUIRED`）

curl 示例：
```bash
curl -X POST "${BASE}/api/v1/admin/redeem-codes/create-and-redeem" \
  -H "x-api-key: ${KEY}" \
  -H "Idempotency-Key: pay-cm1234567890-success" \
  -H "Content-Type: application/json" \
  -d '{
    "code":"s2p_cm1234567890",
    "type":"balance",
    "value":100.00,
    "user_id":123,
    "notes":"sub2apipay order: cm1234567890"
  }'
```

### 2) 查询用户（可选前置校验）
`GET /api/v1/admin/users/:id`

```bash
curl -s "${BASE}/api/v1/admin/users/123" \
  -H "x-api-key: ${KEY}"
```

### 3) 余额调整（已有接口）
`POST /api/v1/admin/users/:id/balance`

用途：人工补偿 / 扣减，支持 `set` / `add` / `subtract`。

请求体示例（扣减）：
```json
{
  "balance": 100.0,
  "operation": "subtract",
  "notes": "manual correction"
}
```

```bash
curl -X POST "${BASE}/api/v1/admin/users/123/balance" \
  -H "x-api-key: ${KEY}" \
  -H "Idempotency-Key: balance-subtract-cm1234567890" \
  -H "Content-Type: application/json" \
  -d '{
    "balance":100.00,
    "operation":"subtract",
    "notes":"manual correction"
  }'
```

### 4) 自定义页面嵌入上下文
当 Sub2API 打开用户侧自定义页面 iframe URL 时，会追加以下非敏感上下文：
- `user_id`
- `theme`（`light` / `dark`）
- `lang`（例如 `zh` / `en`，用于向嵌入页传递当前界面语言）
- `ui_mode`（固定 `embedded`）
- `src_host`（Sub2API 来源 origin）
- `src_url`（只包含 origin + pathname，不包含 query/hash）

示例：
```text
https://help.example.com/page?user_id=123&theme=light&lang=zh&ui_mode=embedded
```

Sub2API 登录令牌不会写入 iframe URL、新窗口 URL、referrer、`src_url` 或浏览器消息。如果管理员对单个自定义菜单显式开启“向嵌入页提供权限验证令牌”，后端会签发 90 秒有效、绑定菜单与目标 origin 的能力令牌，并仅在 iframe 加载完成后通过限定 `targetOrigin` 的 `postMessage` 发送：

```json
{
  "type": "sub2api:embedded-auth",
  "version": 2,
  "credential_type": "embedded_capability",
  "token": "<short-lived-capability>",
  "expires_at": "2026-08-30T12:01:30Z",
  "user_id": 123
}
```

嵌入页应在安装监听器后发送 ready 消息；宿主会校验 `event.origin` 和 `event.source` 后重发当前能力令牌：

```json
{"type":"sub2api:embedded-auth-ready","version":2}
```

嵌入页不能把能力令牌当作 Sub2API Bearer Token。它只能调用权限验证接口；浏览器调用时，请求 `Origin` 还必须与令牌受众一致：

```http
POST /api/v1/auth/embedded-capability/verify
Content-Type: application/json

{"token":"<short-lived-capability>","audience":"https://help.example.com"}
```

验证成功返回 `valid`、`user_id`、`role`、`menu_id`、`audience`、`permissions` 和 `expires_at`，不会创建会话、Cookie、刷新令牌或登录令牌。能力令牌使用与登录 JWT 不同的派生签名密钥，无法反向登录 Sub2API；菜单开关关闭、URL/角色变化、用户停用、密码导致 TokenVersion 变化或令牌过期后，验证立即失败。非本机目标必须使用 HTTPS；HTTP 只允许 loopback 开发地址。嵌入页若从浏览器直接调用验证接口，还须把自身 origin 加入 Sub2API 的 `cors.allowed_origins`；也可以由嵌入页后端代为验证。新窗口始终不会接收令牌。

### 5) 失败处理建议
- 支付成功与充值成功分状态落库
- 回调验签成功后立即标记“支付成功”
- 支付成功但充值失败的订单允许后续重试
- 重试保持相同 `code`，并使用新的 `Idempotency-Key`

### 6) `doc_url` 配置建议
- 查看链接：`https://github.com/DR-lin-eng/sub2api-no2api/blob/main/docs/ADMIN_PAYMENT_INTEGRATION_API.md`
- 下载链接：`https://raw.githubusercontent.com/DR-lin-eng/sub2api-no2api/main/docs/ADMIN_PAYMENT_INTEGRATION_API.md`

---

## English

### Purpose
This document describes the minimal Sub2API Admin API surface for external payment integrations (for example, `sub2apipay`), including:
- Recharge after payment success
- User lookup
- Manual balance correction
- Custom iframe context and explicit permission-capability delegation

### Base URL
- Production: `https://<your-domain>`
- Beta: `http://<your-server-ip>:8084`

### Authentication
Recommended headers:
- `x-api-key: admin-<64hex>`
- `Content-Type: application/json`
- `Idempotency-Key` for idempotent endpoints

Note: Admin JWT can also access admin routes, but Admin API Key is recommended for server-to-server integration.

Admin API Keys now use fine-grained scopes. The integration in this document requires at least `admin.users.read`, `admin.users.write`, and `admin.write`. See [Admin API documentation](./ADMIN_API.md) for key creation, rotation, and revocation.

### 1) Create and Redeem in one step
`POST /api/v1/admin/redeem-codes/create-and-redeem`

Use case: atomically create a redeem code and redeem it to a target user.

Headers:
- `x-api-key`
- `Idempotency-Key`

Request body:
```json
{
  "code": "s2p_cm1234567890",
  "type": "balance",
  "value": 100.0,
  "user_id": 123,
  "notes": "sub2apipay order: cm1234567890"
}
```

Idempotency behavior:
- Same `code` and same `used_by`: `200`
- Same `code` but different `used_by`: `409`
- Missing `Idempotency-Key`: `400` (`IDEMPOTENCY_KEY_REQUIRED`)

curl example:
```bash
curl -X POST "${BASE}/api/v1/admin/redeem-codes/create-and-redeem" \
  -H "x-api-key: ${KEY}" \
  -H "Idempotency-Key: pay-cm1234567890-success" \
  -H "Content-Type: application/json" \
  -d '{
    "code":"s2p_cm1234567890",
    "type":"balance",
    "value":100.00,
    "user_id":123,
    "notes":"sub2apipay order: cm1234567890"
  }'
```

### 2) Query User (optional pre-check)
`GET /api/v1/admin/users/:id`

```bash
curl -s "${BASE}/api/v1/admin/users/123" \
  -H "x-api-key: ${KEY}"
```

### 3) Balance Adjustment (existing API)
`POST /api/v1/admin/users/:id/balance`

Use case: manual correction with `set` / `add` / `subtract`.

Request body example (`subtract`):
```json
{
  "balance": 100.0,
  "operation": "subtract",
  "notes": "manual correction"
}
```

```bash
curl -X POST "${BASE}/api/v1/admin/users/123/balance" \
  -H "x-api-key: ${KEY}" \
  -H "Idempotency-Key: balance-subtract-cm1234567890" \
  -H "Content-Type: application/json" \
  -d '{
    "balance":100.00,
    "operation":"subtract",
    "notes":"manual correction"
  }'
```

### 4) Custom page embed context
When Sub2API opens a user-facing custom page iframe URL, it appends the following non-sensitive context:
- `user_id`
- `theme` (`light` / `dark`)
- `lang` (for example `zh` / `en`, used to pass the current UI language to the embedded page)
- `ui_mode` (fixed: `embedded`)
- `src_host` (the Sub2API origin)
- `src_url` (origin + pathname only; query and hash are excluded)

Example:
```text
https://help.example.com/page?user_id=123&theme=light&lang=en&ui_mode=embedded
```

Sub2API login tokens are never placed in iframe URLs, new-window URLs, referrers, `src_url`, or browser messages. If an administrator explicitly enables "Provide embedded permission proof" for a custom menu item, the backend issues a 90-second capability bound to that menu and target origin. Only that capability is sent after the iframe loads through a `postMessage` call locked to the exact target origin:

```json
{
  "type": "sub2api:embedded-auth",
  "version": 2,
  "credential_type": "embedded_capability",
  "token": "<short-lived-capability>",
  "expires_at": "2026-08-30T12:01:30Z",
  "user_id": 123
}
```

After installing its listener, the embedded page can send `{"type":"sub2api:embedded-auth-ready","version":2}`. The host validates both `event.origin` and `event.source` before resending the current capability.

The capability is not a Sub2API Bearer token. It can only be introspected with `POST /api/v1/auth/embedded-capability/verify` and a JSON body containing `token` and the exact `audience` origin. Browser requests must also have a matching `Origin` header. Successful verification returns identity and menu permissions without creating a session, cookie, refresh token, or login token. A separately derived signing key prevents the capability from authenticating to normal Sub2API endpoints. Verification fails after expiry, opt-out, menu URL or role changes, user deactivation, or TokenVersion changes. Non-loopback targets must use HTTPS. Direct browser introspection also requires the embedded origin in Sub2API's `cors.allowed_origins`; a backend-to-backend verification call is supported instead. New windows never receive a capability.

### 5) Failure handling recommendations
- Persist payment success and recharge success as separate states
- Mark payment as successful immediately after verified callback
- Allow retry for orders with payment success but recharge failure
- Keep the same `code` for retry, and use a new `Idempotency-Key`

### 6) Recommended `doc_url`
- View URL: `https://github.com/DR-lin-eng/sub2api-no2api/blob/main/docs/ADMIN_PAYMENT_INTEGRATION_API.md`
- Download URL: `https://raw.githubusercontent.com/DR-lin-eng/sub2api-no2api/main/docs/ADMIN_PAYMENT_INTEGRATION_API.md`
