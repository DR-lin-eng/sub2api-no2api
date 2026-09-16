# OAuth2 对外授权服务

Sub2API 可以作为 OAuth2 授权服务器，让管理员登记的第三方应用在用户明确同意后读取有限的用户身份信息。该能力默认关闭，配置入口位于“系统设置 -> 安全与认证 -> OAuth2 对外授权服务”。

## 权限边界

- 只有完整管理员 JWT 会话可以启停服务、修改 issuer、创建或删除客户端、编辑回调地址和 scope、轮换客户端密钥；Admin API Key 始终不能执行这些写操作。站点启用 step-up 后，这些操作还要求近期 TOTP 验证。
- 自定义员工权限组和 Admin API Key 不能建立或修改 OAuth2 信任关系。
- 用户必须先登录 Sub2API，并在 `/oauth/authorize` 页面明确允许或拒绝每次授权请求。
- OAuth2 access token 只用于 `/oauth/userinfo`，不能作为站内 JWT、Admin API Key 或模型网关 API Key。模型请求仍执行原有 API Key、分组、余额、订阅、并发和计费校验。
- 管理员关闭全局开关、停用/删除客户端或移除其 scope 后，相关 access token 在下一次资源请求时立即失效。用户停用或 TokenVersion 变化也会立即使 token 失效。

## 支持的协议

当前支持 OAuth 2.0 Authorization Code Grant：

- 所有客户端强制使用 PKCE `S256`。
- 回调地址必须与管理员登记值逐字匹配；远程地址必须使用 HTTPS，HTTP 仅允许 loopback 开发地址。
- `state` 必填，authorization code 有效期为 5 分钟且只能消费一次。
- access token 是随机不透明凭据，服务端只在 Redis 保存 SHA-256 摘要与授权元数据；默认有效期 3600 秒，管理员可设置 60–86400 秒。
- 机密客户端支持 `client_secret_basic` 和 `client_secret_post`；公共客户端不签发 secret。
- 浏览器内公共客户端还需由部署管理员把其 origin 加入 `cors.allowed_origins`；服务端或原生客户端不受浏览器 CORS 限制。
- 不签发 refresh token，也不把 OAuth2 access token 写入 URL、日志或数据库。

支持的 scope：

| Scope | `/oauth/userinfo` 字段 |
| --- | --- |
| `profile` | 每客户端隔离的 `sub`、`preferred_username`、`name` |
| `email` | `email` |

## 端点

| 方法与路径 | 鉴权 | 用途 |
| --- | --- | --- |
| `GET /.well-known/oauth-authorization-server` | 公开，仅服务启用时可用 | 授权服务器元数据 |
| `GET /oauth/authorize?...` | Sub2API 用户登录 | 浏览器授权页 |
| `POST /oauth/token` | 客户端认证或公共客户端 ID | 用 authorization code + verifier 换取 access token |
| `GET /oauth/userinfo` | `Authorization: Bearer <access-token>` | 读取获准的用户字段 |
| `POST /oauth/revoke` | 客户端认证或公共客户端 ID | 撤销本客户端签发的 access token |

管理端接口统一位于 `/api/v1/admin/oauth2-provider`：

- `GET /api/v1/admin/oauth2-provider`
- `PUT /api/v1/admin/oauth2-provider`
- `POST /api/v1/admin/oauth2-provider/clients`
- `PUT /api/v1/admin/oauth2-provider/clients/:client_id`
- `POST /api/v1/admin/oauth2-provider/clients/:client_id/rotate-secret`
- `DELETE /api/v1/admin/oauth2-provider/clients/:client_id`

客户端 secret 仅在创建或轮换成功响应中显示一次，持久设置只保存其 SHA-256 摘要。

## PKCE 调用示例

下面示例使用占位符。`CODE_VERIFIER` 必须是 43–128 字符的 RFC 7636 unreserved 字符串，`CODE_CHALLENGE` 为其 SHA-256 后的 base64url（无 padding）结果。

```text
GET https://<issuer>/oauth/authorize
  ?response_type=code
  &client_id=<client-id>
  &redirect_uri=https%3A%2F%2Fclient.example.com%2Foauth%2Fcallback
  &scope=profile%20email
  &state=<opaque-state>
  &code_challenge=<s256-challenge>
  &code_challenge_method=S256
```

授权成功后，第三方应用后端使用回调中的 code 换取 token：

```bash
curl -u '<client-id>:<client-secret>' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'grant_type=authorization_code' \
  -d 'code=<authorization-code>' \
  -d 'redirect_uri=https://client.example.com/oauth/callback' \
  -d 'code_verifier=<code-verifier>' \
  'https://<issuer>/oauth/token'
```

读取用户信息：

```bash
curl -H 'Authorization: Bearer <access-token>' 'https://<issuer>/oauth/userinfo'
```

协议端点返回 RFC 6749 风格的 `error` / `error_description` JSON，并带 `Cache-Control: no-store`。管理端接口继续使用 Sub2API 标准 JSON envelope。
