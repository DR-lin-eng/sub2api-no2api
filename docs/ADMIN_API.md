# Admin API 调用文档

本文档说明如何使用管理员 JWT 或 scoped Admin API Key 调用 Sub2API 管理接口，并介绍 Admin API Key 的创建、权限范围、轮换和撤销方式。

## 基础信息

- 基础地址：`https://<your-domain>`
- Admin API 前缀：`/api/v1/admin`
- JSON 请求头：`Content-Type: application/json`
- 机器调用认证头：`x-api-key: <admin-api-key>`
- 管理员页面调用：`Authorization: Bearer <admin-jwt>`

Admin API Key 适合服务间调用。管理员 JWT 适合浏览器管理页面和需要交互式 TOTP step-up 的操作。

## 创建 Admin API Key

首次创建建议在管理页面完成：

1. 使用管理员账号登录。
2. 进入“系统设置 -> 安全”。
3. 输入 Key 名称和有效期。
4. 勾选需要的权限范围。
5. 创建后立即保存完整 Key。

完整 Key 只会在创建或轮换时显示一次。服务端仅保存 SHA-256 摘要、前缀、后四位和权限元数据。

也可以使用管理员 JWT 调用创建接口：

```bash
BASE="https://<your-domain>"
ADMIN_JWT="<admin-jwt>"

curl -X POST "${BASE}/api/v1/admin/settings/admin-api-keys" \
  -H "Authorization: Bearer ${ADMIN_JWT}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "只读运维",
    "scopes": ["admin.read", "admin.ops.read"],
    "expires_at": "2026-12-31T23:59:59Z"
  }'
```

创建成功返回 `201`：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "key": "admin-<仅显示一次的完整密钥>",
    "metadata": {
      "id": "<key-id>",
      "name": "只读运维",
      "key_prefix": "admin-abcd",
      "last_four": "1234",
      "scopes": ["admin.read", "admin.ops.read"],
      "status": "active",
      "expires_at": "2026-12-31T23:59:59Z"
    }
  }
}
```

不要把返回的完整 Key 写入日志、URL、前端代码或 Git 仓库。

## 权限范围

| Scope | 允许的操作 |
| --- | --- |
| `admin.read` | 所有非敏感 Admin GET/HEAD/OPTIONS 请求 |
| `admin.write` | 未划分到专属资源的 Admin POST/PUT/PATCH/DELETE 请求 |
| `admin.users.read` | 用户管理读取接口 |
| `admin.users.write` | 用户管理写入接口 |
| `admin.accounts.read` | 上游账号管理读取接口 |
| `admin.accounts.write` | 上游账号管理写入接口 |
| `admin.settings.read` | 系统设置读取和 Admin API Key 列表 |
| `admin.settings.write` | 系统设置修改、Key 创建、编辑、轮换和撤销 |
| `admin.backups.read` | 备份元数据读取，不包含下载链接 |
| `admin.backups.write` | 备份创建、恢复和删除 |
| `admin.system.read` | 系统状态和版本读取 |
| `admin.system.write` | 系统生命周期操作 |
| `admin.audit.read` | 审计日志读取 |
| `admin.audit.write` | 审计日志管理操作 |
| `admin.ops.read` | 运维监控读取 |
| `admin.ops.write` | 运维规则、告警和配置修改 |

读权限不会自动授予写权限，写权限也不会自动授予读权限。需要完整访问时必须同时授予对应的 read 和 write scope。

以下敏感读取无论授予什么 scope 都不允许 Admin API Key 调用：

- `GET /api/v1/admin/accounts/data`
- `GET /api/v1/admin/proxies/data`
- `GET /api/v1/admin/backups/:id/download-url`

这些操作必须使用管理员 JWT，并通过接口要求的 TOTP step-up。

旧版单一 Admin API Key 仅保留只读兼容能力，不能修改设置、轮换自身或执行其他写操作。建议重新创建 scoped Key 后撤销旧 Key。

## 使用 scoped Key 调用 Admin API

```bash
BASE="https://<your-domain>"
ADMIN_API_KEY="admin-<your-key>"
```

读取用户，需要 `admin.users.read` 或 `admin.read`：

```bash
curl -sS "${BASE}/api/v1/admin/users/123" \
  -H "x-api-key: ${ADMIN_API_KEY}"
```

调整余额，需要 `admin.users.write`：

```bash
curl -X POST "${BASE}/api/v1/admin/users/123/balance" \
  -H "x-api-key: ${ADMIN_API_KEY}" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: balance-adjust-order-123" \
  -d '{
    "balance": 10,
    "operation": "add",
    "notes": "order 123"
  }'
```

读取运维状态，需要 `admin.ops.read` 或 `admin.read`：

```bash
curl -sS "${BASE}/api/v1/admin/ops/concurrency" \
  -H "x-api-key: ${ADMIN_API_KEY}"
```

## Key 管理接口

### 查询列表

`GET /api/v1/admin/settings/admin-api-keys`

需要管理员 JWT，或具有 `admin.settings.read` / `admin.read` 的 Admin API Key。

```bash
curl -sS "${BASE}/api/v1/admin/settings/admin-api-keys" \
  -H "Authorization: Bearer ${ADMIN_JWT}"
```

返回值只包含脱敏信息，不包含完整 Key 或摘要。

### 编辑名称、权限和有效期

`PUT /api/v1/admin/settings/admin-api-keys/:id`

```bash
curl -X PUT "${BASE}/api/v1/admin/settings/admin-api-keys/<key-id>" \
  -H "Authorization: Bearer ${ADMIN_JWT}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "用户只读同步",
    "scopes": ["admin.users.read"],
    "expires_at": "2027-01-31T00:00:00Z"
  }'
```

将 `expires_at` 设置为 `null` 可以取消有效期限制。

### 轮换 Key

`POST /api/v1/admin/settings/admin-api-keys/:id/rotate`

```bash
curl -X POST "${BASE}/api/v1/admin/settings/admin-api-keys/<key-id>/rotate" \
  -H "Authorization: Bearer ${ADMIN_JWT}"
```

旧 Key 会立即失效，新 Key 只在本次响应的 `data.key` 中返回。

推荐轮换顺序：

1. 调用轮换接口获取新 Key。
2. 将新 Key 写入密钥管理系统。
3. 更新调用方配置并重启或热加载。
4. 使用只读接口验证新 Key。

### 撤销 Key

`DELETE /api/v1/admin/settings/admin-api-keys/:id`

```bash
curl -X DELETE "${BASE}/api/v1/admin/settings/admin-api-keys/<key-id>" \
  -H "Authorization: Bearer ${ADMIN_JWT}"
```

撤销后，使用该 Key 的后续请求返回 `401 INVALID_ADMIN_KEY`。

## 状态码与错误

| HTTP 状态码 | 错误码 | 说明 |
| --- | --- | --- |
| `401` | `UNAUTHORIZED` | 未提供认证信息 |
| `401` | `INVALID_ADMIN_KEY` | Key 不存在、已撤销或已过期 |
| `403` | `ADMIN_API_KEY_SCOPE_REQUIRED` | Key 缺少当前接口需要的 scope，或接口禁止机器 Key 调用 |
| `403` | `FORBIDDEN` | JWT 用户不是管理员 |
| `423` | `ADMIN_COMPLIANCE_ACK_REQUIRED` | 管理员尚未确认当前版本的部署与运营合规承诺 |

错误响应示例：

```json
{
  "code": "ADMIN_API_KEY_SCOPE_REQUIRED",
  "message": "Admin API key does not have permission for this operation"
}
```

## 安全建议

- 每个外部系统创建独立 Key，不要多人或多服务共享。
- 只授予实际需要的 scope，并设置合理的 `expires_at`。
- 将 Key 保存在 Vault、KMS 或部署平台的 Secret 中。
- 不要通过 query string 传递 Key。
- 不要在错误日志、审计 payload 或监控标签中记录完整 Key。
- 定期检查 `last_used_at`，撤销长期未使用的 Key。
- 对余额、兑换、支付回调等接口使用稳定的 `Idempotency-Key`。
- 对系统更新、备份恢复、凭证导出等高影响操作使用管理员 JWT 和 TOTP step-up。

支付集成的具体接口请参阅 [ADMIN_PAYMENT_INTEGRATION_API.md](./ADMIN_PAYMENT_INTEGRATION_API.md)。
